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
		d.idRefs[counter] = salesforce.IDRef{strconv.Itoa(counter), "hi"}
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

type pok struct{}

func (p *pok) Headers() []string { return []string{"ID", "Ref"} }
func (p *pok) Rows() iter.Seq[[]string] {
	return func(yield func([]string) bool) {
		for _, r := range [][]string{{"0033000000abcde", "1"}, {"0063000000abcde", "2"}} {
			if !yield(r) {
				return
			}
		}
	}
}

type punlink struct{}

func (p *punlink) Headers() []string { return []string{"ID", "Ref"} }
func (p *punlink) Rows() iter.Seq[[]string] {
	return func(yield func([]string) bool) {
		for _, r := range [][]string{{"0033000000abcde", " "}, {"0063000000abcde", ""}} {
			if !yield(r) {
				return
			}
		}
	}
}

type pbad struct{ headersBad bool }

func (p *pbad) Headers() []string {
	if !p.headersBad {
		return []string{"ID", "Ref"} // ok headers
	} else {
		return []string{"invalid", "Ref"} // invalid headers
	}
}
func (p *pbad) Rows() iter.Seq[[]string] {
	return func(yield func([]string) bool) {
		for _, r := range [][]string{{"tooshort", "1"}} {
			if !yield(r) {
				return
			}
		}
	}
}

func TestDataNew(t *testing.T) {

	tests := []struct {
		parser Parser
		action string
		isErr  bool
		errMsg string
	}{
		{&pok{}, "link", false, ""},
		{&pok{}, "unlink", true, "reference not empty for unlink"},
		{&pok{}, "invalid", true, "must be 'link' or 'unlink'"},
		{&punlink{}, "unlink", false, ""},
		{&pbad{false}, "link", true, "row 1 (1 indexed) error: \"tooshort\" is an invalid Salesforce ID"},
		{&pbad{true}, "link", true, "header \"invalid\" found -- expected ID"},
	}
	for ii, tt := range tests {
		t.Run(fmt.Sprintf("test_%d", ii), func(t *testing.T) {

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
