package salesforce

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSalesforceID(t *testing.T) {

	for ii, tt := range []struct {
		input          string
		expectedOutput *salesforceID
	}{
		{"0015A00002CrA9PQAV", new(salesforceID("0015A00002CrA9PQAV"))},
		{"In Valid", nil},
		{"0015A00002CrA9PQAVx", nil}, // too long
		{"0015A00002CrA9", nil},      // too short
		{"0015A00002-rA9PQAV", nil},  // invalid character
	} {

		t.Run(fmt.Sprintf("%d_test", ii), func(t *testing.T) {
			got := newSalesforceID(tt.input)
			want := tt.expectedOutput
			if got == nil {
				if want == nil {
					return
				}
				t.Errorf("expected %q got nil", *want)
				return
			}
			if want == nil {
				t.Errorf("unexpected %q expected nil", *got)
				return
			}
			if *got != *want {
				t.Errorf("got %s want %s", *got, *want)
			}
			if got, want := got.String(), tt.input; got != want {
				t.Errorf("string mismatch %s want %s", got, want)
			}
		})
	}
}

func TestSalesforceIDsValid(t *testing.T) {

	for ii, tt := range []struct {
		input       []string
		expectedErr error
	}{
		{[]string{"0015A00002CrA9PQAV"}, nil},
		{[]string{"0015A00002CrA9PQAV", "0055A000006vN9PQAU"}, nil},
		{[]string{"0015-A0002vvvvvvvv", "0055A000006vN9PQAU"}, errors.New(`"0015-A0002vvvvvvvv" is an invalid Salesforce ID`)},
		{nil, errors.New("no ids provided")},
	} {
		t.Run(fmt.Sprintf("%d_test", ii), func(t *testing.T) {
			err := IDsValid(tt.input...)
			if err != nil {
				if tt.expectedErr == nil {
					t.Fatalf("got unexpected err: %v", err)
				}
				if got, want := err.Error(), tt.expectedErr.Error(); !strings.Contains(got, want) {
					t.Errorf("expected error %q to contain %q", got, want)
				}
			}
			if err == nil && tt.expectedErr != nil {
				t.Fatalf("expected err %q, got nil", tt.expectedErr.Error())
			}
		})
	}
}
