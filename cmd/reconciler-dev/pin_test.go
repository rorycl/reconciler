package main

import (
	"io"
	"os"
	"regexp"
	"testing"
	"time"
)

func TestPin(t *testing.T) {

	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	pinTimeout = 1 * time.Millisecond
	err := getPin()
	if err == nil {
		t.Error("unexpected nil error")
	}

	os.Stdout = orig
	_ = w.Close()

	contents, err := io.ReadAll(r)
	if err != nil {
		t.Error(err)
	}

	regexpPin := regexp.MustCompile("\n[0-9]{6}\n")
	if !regexpPin.Match(contents) {
		t.Errorf("expected pin in %s", string(contents))
	}
}
