package db

import (
	"log/slog"
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

	testingMode = true
	t.Cleanup(func() {
		testingMode = false
	})

	accountCodes := "^(53|55|57)"
	sqlDir := "sql"

	var err error
	testDB, err := NewConnectionInTestMode("file::memory:?cache=shared", sqlDir, accountCodes, nil)
	if err != nil {
		t.Fatalf("in-memory test database opening error: %v", err)
	}

	// set log level to Debu
	testDB.SetLogLevel(slog.LevelWarn)

	// closeDBFunc is a closure for running by the function consumer.
	closeDBFunc := func() {
		err := testDB.Close()
		if err != nil {
			t.Fatalf("unexpected db close error: %v", err)
		}
	}

	return testDB, closeDBFunc
}
