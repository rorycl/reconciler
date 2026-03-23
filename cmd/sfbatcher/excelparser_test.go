package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestExcelParser(t *testing.T) {

	testFile := "testdata/simple.xlsx"

	parser, err := NewParser(testFile)
	if err != nil {
		t.Fatal(err)
	}

	headers := parser.Headers()
	got, want := headers, []string{"ID", "Ref", "Other"}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("headers mismatch:\n%s", diff)
	}

	rows := [][]string{}
	for r := range parser.Rows() {
		rows = append(rows, r)
	}
	if got, want := len(rows), 3; got != want {
		t.Errorf("got %d want %d rows", got, want)
	}
}

func TestExcelParserFail(t *testing.T) {

	tests := []struct {
		filename string
		err      error
	}{
		{"", errors.New("newparser received an empty file")},
		{"testdata/doesnotexist.xlsx", errors.New("doesnotexist.xlsx\" error")},
		{"testdata/twosheets.xlsx", errors.New("with only one sheet supported")},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("test_%d", ii), func(t *testing.T) {
			_, err := NewParser(tt.filename)
			if err == nil {
				t.Fatal("expected error")
			}
			if got, want := err.Error(), tt.err.Error(); !strings.Contains(got, want) {
				t.Errorf("error %q did not contain %q", got, want)
			}
		})
	}
}
