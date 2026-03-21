package filewatcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFiles(t *testing.T, dir1, dir2 string, flushDuration time.Duration) {
	t.Helper()
	for _, dirFile := range [][]string{
		{dir1, ".newfile.html"}, // not counted
		{dir1, "abc.html"},      // counted
		{dir1, "abc.HTML"},      // counted
		{dir1, ".hidden.HTML"},  // not counted
		{dir2, "abctxt"},        // not counted
		{dir2, "ABC.txt"},       // counted
	} {
		dir, file := dirFile[0], dirFile[1]
		fmt.Println(dir, file)
		o, err := os.Create(filepath.Join(dir, file))
		if err != nil {
			t.Fatal(err)
		}
		_, err = fmt.Fprint(o, "hi")
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(flushDuration) // accommodate flush interval
	}
}

func TestFileChangeCanonical(t *testing.T) {
	dir1, dir2 := t.TempDir(), t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
	}()

	fcn, err := NewFileChangeNotifier(
		ctx,
		map[string][]string{
			dir1: {".html"},
			dir2: {"txt"},
		},
	)
	if err != nil {
		t.Fatalf("error initialising fcn: %v", err)
	}

	// override default flushDuration for testing
	fcn.flushDuration = 2 * time.Millisecond

	writeFiles(t, dir1, dir2, fcn.flushDuration)

	timeOut := time.After(2 * fcn.flushDuration)

	select {
	case <-timeOut:
		t.Fatal("test timed out")
	case err := <-fcn.Update():
		if err != nil {
			t.Errorf("unexpected error from update: %v", err)
		}
		break
	}

}

func TestFileChangeNotifier(t *testing.T) {

	dir1, dir2 := t.TempDir(), t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())

	// Register notifer.
	fcn, err := NewFileChangeNotifier(
		ctx,
		map[string][]string{
			dir1: {".html"},
			dir2: {"txt"},
		},
	)
	if err != nil {
		t.Fatalf("error initialising fcn: %v", err)
	}

	// override default flushDuration for testing
	fcn.flushDuration = 2 * time.Millisecond

	// 2. wait for Update events
	counter := 0
	go func() {
		for range fcn.Update() {
			counter++
		}
	}()

	// 3. cancel after a few moments.
	go func() {
		time.Sleep(5 * fcn.flushDuration)
		cancel()
	}()

	// Write files.
	writeFiles(t, dir1, dir2, fcn.flushDuration)

	// Expect 3 valid events.
	if got, want := counter, 3; got != want {
		t.Errorf("counter got %d want %d", got, want)
	}
}
