package db

import (
	"log/slog"
	"testing"
	"time"

	mounts "reconciler/internal/mounts"
)

func ptrTime(ti time.Time) *time.Time { return &ti }

func ptrStr(s string) *string { return &s }

// func ptrBool(b bool) *bool { return &b }

func ptrFloat64(f float64) *float64 { return &f }

// setupTestDB sets up a test database connection.
func setupTestDB(t *testing.T) (*DB, func()) {
	t.Helper()

	accountCodes := "^(53|55|57)"

	// mount the sql fs either using the embedded fs or via the provided path.
	// The path is likely to need to be relative to "here" as ".." type paths are not
	// accepted by fs mounting.
	sqlFS, err := mounts.NewFileMount("sql", SQLEmbeddedFS, "sql")
	if err != nil {
		t.Fatalf("mount error: %v", err)
	}

	testDB, err := NewConnectionInTestMode("file::memory:?cache=shared", sqlFS, accountCodes, nil)
	if err != nil {
		t.Fatalf("in-memory test database opening error: %v", err)
	}

	// set log level to Info/Debug
	testDB.SetLogLevel(slog.LevelInfo)

	// closeDBFunc is a closure for running by the function consumer.
	closeDBFunc := func() {
		err := testDB.Close()
		if err != nil {
			t.Fatalf("unexpected db close error: %v", err)
		}
	}

	return testDB, closeDBFunc
}
