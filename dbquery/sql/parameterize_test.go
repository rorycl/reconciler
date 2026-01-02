package main

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParameterize(t *testing.T) {

	tests := []struct {
		input        string
		expectedArgs []string
		expectedBody string
		isErr        bool
	}{
		{
			input:        `date('2026-03-31') AS DateFrom   /* @param */`,
			expectedArgs: []string{"DateFrom"},
			expectedBody: `? AS DateFrom`,
		},
		{
			input: `nothing`,
			isErr: true,
		},
		{
			input: `
WITH concrete AS (
	date('2025-04-01') AS DateFrom   /* @param */
	,date('2026-03-31') AS DateTo    /* @param */
	,'^(53|55|57).*' AS AccountCodes /* @param */
	-- All | Reconciled | NotReconciled
	,'All' AS ReconciliationStatus   /* @param */
	,null AS NullExample             /* @param */
	,-34.5 AS FloatExample           /* @param */
	,'raw string' AS RawString
)
`,
			expectedArgs: []string{
				"DateFrom", "DateTo", "AccountCodes", "ReconciliationStatus",
				"NullExample", "FloatExample"},
			expectedBody: `
WITH concrete AS (
	? AS DateFrom
	,? AS DateTo
	,? AS AccountCodes
	-- All | Reconciled | NotReconciled
	,? AS ReconciliationStatus
	,? AS NullExample
	,? AS FloatExample
	,'raw string' AS RawString
)
`,
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("test_%d", ii), func(t *testing.T) {
			result, err := Parameterize([]byte(tt.input))
			if err != nil {
				if tt.isErr {
					return
				}
				t.Fatal(err)
			}
			if got, want := len(result.Parameters), len(tt.expectedArgs); got != want {
				t.Errorf("got %d parameters, want %d", got, want)
			}
			if diff := cmp.Diff(tt.expectedArgs, result.Parameters); diff != "" {
				t.Errorf("Parameters mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(string(result.Body), tt.expectedBody); diff != "" {
				t.Error(diff)
			}
		})
	}
}
