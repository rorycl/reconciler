package db

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite" // pure go sqlite driver
)

// TestRegexpFunctionFail tests the failure of a REGEXP call without first registering
// the custom function.
func TestRegexpFunctionFail(t *testing.T) {
	t.Skip()
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open error: %v", err)
	}
	if err := testDB.Ping(); err != nil {
		t.Fatalf("ping error: %v", err)
	}
	t.Cleanup(func() {
		_ = testDB.Close()
	})

	_, err = testDB.Exec("select 'ABC' REGEXP '^[A-Z]'")
	if err == nil {
		t.Error("unexpected regexp success without registration")
		return
	}
	if got, want := err.Error(), "SQL logic error: no such function: REGEXP"; !strings.Contains(got, want) {
		t.Errorf("err %q does not contain %q", got, want)
	}
}

// TestRegexpFunctionSucceed shows the success of a REGEXP call after registration of
// the custom function.
func TestRegexpFunctionSucceed(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open error: %v", err)
	}

	// Register the regexp function.
	RegisterFunctions()

	if err := testDB.Ping(); err != nil {
		t.Fatalf("ping error: %v", err)
	}
	t.Cleanup(func() {
		_ = testDB.Close()
	})

	_, err = testDB.Exec("select 'ABC' REGEXP '^[A-Z]'")
	if err != nil {
		t.Errorf("unexpected regexp error after registration: %v", err)
	}
}
