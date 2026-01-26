package db

import (
	"io/fs"
	"os"
	"testing"
	"time"
)

func ptrTime(ti time.Time) *time.Time { return &ti }

func ptrStr(s string) *string { return &s }

func ptrBool(b bool) *bool { return &b }

func ptrFloat64(f float64) *float64 { return &f }

// setupTestDB sets up a test database connection.
func setupTestDB(t *testing.T) (*DB, func()) {
	t.Helper()

	prepareNamedStatementsOnStartup = false
	defer func() {
		prepareNamedStatementsOnStartup = true
	}()

	accountCodes := "^(53|55|57)"
	sqlDir := os.DirFS("sql")

	var err error
	testDB, err := NewConnection("file::memory:?cache=shared", sqlDir, accountCodes)
	if err != nil {
		t.Fatalf("in-memory test database opening error: %v", err)
	}

	// Load the schema definitions.
	if err := testDB.InitSchema(sqlDir, "schema.sql"); err != nil {
		_ = testDB.Close()
		t.Fatalf("Failed to initialize schema for test database: %v", err)
	}

	// Load the test data.
	data, err := fs.ReadFile(sqlDir, "load_data.sql")
	if err != nil {
		t.Fatalf("Failed to read file for loading data for test DB: %v", err)
	}
	_, err = testDB.Exec(string(data))
	if err != nil {
		_ = testDB.Close()
		t.Fatalf("Failed to load data for test database: %v", err)
	}

	// Prepare the functions and named statements.
	err = testDB.prepareNamedStatements()
	if err != nil {
		t.Fatalf("could not prepare named statements: %v", err)
	}

	// closeDBFunc is a closure for running by the function consumer.
	closeDBFunc := func() {
		err := testDB.Close()
		if err != nil {
			t.Fatalf("unexpected db close error: %v", err)
		}
	}

	return testDB, closeDBFunc
}
