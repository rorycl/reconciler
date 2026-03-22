package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPin(t *testing.T) {

	testPin := newPin()

	if !testPin.verify(string(*testPin)) {
		t.Errorf("unexpected error %s", string(*testPin))
	}

	// error test.
	pinTimeout = 1 * time.Millisecond
	err := testPin.check()
	if err == nil {
		t.Error("unexpected nil error")
	}

	// success test.
	fileName := filepath.Join(t.TempDir(), "t1")
	_ = os.WriteFile(fileName, []byte(*testPin), 0600)
	f, err := os.Open(fileName)
	if err != nil {
		t.Fatal(err)
	}
	stdin = f

	err = testPin.check()
	if err != nil {
		t.Errorf("unexpected error %v", err)
	}

}
