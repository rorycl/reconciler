package main

import (
	"fmt"
	"iter"
	"strconv"
	"strings"
	"testing"

	"github.com/rorycl/reconciler/apiclients/salesforce"
)

func generate(d *Data, no int, t *testing.T) {
	t.Helper()
	counter := 0
	d.idRefs = make([]salesforce.IDRef, no)
	for range no {
		d.idRefs[counter] = salesforce.IDRef{ID: strconv.Itoa(counter), Ref: "hi"}
		counter++
	}
	d.Records = no
}

func TestDataBatch(t *testing.T) {
	for ii, tt := range []struct {
		inputNo      int
		expectedRows int
	}{
		{7, 4},
		{6, 3},
		{0, 0},
	} {
		t.Run(fmt.Sprintf("test_%d", ii), func(t *testing.T) {
			d := &Data{}
			generate(d, tt.inputNo, t)
			rows := 0
			for range d.Batch(2) {
				rows++
			}
			if got, want := rows, tt.expectedRows; got != want {

				t.Errorf("got %d want %d rows", got, want)
			}
		})
	}
}

// p is a test Parser.
type p struct {
	headers []string
	data    [][]string
}

func (p *p) Headers() []string { return p.headers }
func (p *p) Rows() iter.Seq[[]string] {
	return func(yield func([]string) bool) {
		for _, r := range p.data {
			if !yield(r) {
				return
			}
		}
	}
}

func TestDataNew(t *testing.T) {

	tests := []struct {
		name   string
		parser Parser
		action string
		isErr  bool
		errMsg string
	}{
		{
			name: "link ok",
			parser: &p{
				headers: []string{"ID", "Ref"},
				data:    [][]string{{"0033000000abcde", "1"}, {"0063000000abcde", "2"}},
			},
			action: "link",
			isErr:  false,
			errMsg: "",
		},
		{
			name: "unlink error ref not empty",
			parser: &p{
				headers: []string{"ID", "Ref"},
				data:    [][]string{{"0033000000abcde", "1"}, {"0063000000abcde", "2"}},
			},
			action: "unlink",
			isErr:  true,
			errMsg: "reference not empty for unlink",
		},
		{
			name: "invalid action",
			parser: &p{
				headers: []string{"ID", "Ref"},
				data:    [][]string{{"0033000000abcde", "1"}, {"0063000000abcde", "2"}},
			},
			action: "invalid",
			isErr:  true,
			errMsg: "must be 'link' or 'unlink'",
		},
		{
			name: "unlink ok",
			parser: &p{
				headers: []string{"ID", "Ref"},
				data:    [][]string{{"0033000000abcde", " "}, {"0063000000abcde", ""}},
			},
			action: "unlink",
			isErr:  false,
			errMsg: "",
		},
		{
			name: "link id too short",
			parser: &p{
				headers: []string{"ID", "Ref"},
				data:    [][]string{{"tooshort", "1"}},
			},
			action: "link",
			isErr:  true,
			errMsg: "row 1 (1 indexed) error: \"tooshort\" is an invalid Salesforce ID",
		},
		{
			name: "link invalid headername",
			parser: &p{
				headers: []string{"invalid", "Ref"},
				data:    [][]string{{"tooshort", "1"}},
			},
			action: "link",
			isErr:  true,
			errMsg: "header \"invalid\" found -- expected ID",
		},
	}
	for ii, tt := range tests {
		t.Run(fmt.Sprintf("%d_%s", ii, tt.name), func(t *testing.T) {

			_, err := NewData(tt.parser, tt.action)

			if err != nil && !tt.isErr {
				t.Fatalf("unexpected error: %v", err)
			}
			if err == nil && tt.isErr {
				t.Fatal("expected an error")
			}
			if err == nil {
				return
			}
			if got, want := err.Error(), tt.errMsg; !strings.Contains(got, want) {
				t.Errorf("got error %q did not contain %q", got, want)
			}

		})
	}
}
