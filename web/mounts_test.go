package main

import (
	"fmt"
	"testing"
)

func TestMounts(t *testing.T) {
	fm, err := makeMounts(true)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%#v\n", fm)

	fm, err = makeMounts(false)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%#v\n", fm)
}
