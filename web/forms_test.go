package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func newRequest(t *testing.T, urlString string) *http.Request {
	t.Helper()
	r, err := http.NewRequest("GET", urlString, nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestSearchForm tests the SearchForm behaviour
func TestSearchForm(t *testing.T) {

	defaultDateFrom, defaultDateTo := defaultDateToAndFrom()

	tests := []struct {
		name           string
		inputURL       string
		searchForm     *SearchForm
		err            error      // top level errors
		validationErrs *Validator // validation errors
	}{
		{
			name:     "default",
			inputURL: "http://127.0.0.1:8080/invoices/",
			searchForm: &SearchForm{
				ReconciliationStatus: "NotReconciled",
				DateFrom:             defaultDateFrom,
				DateTo:               defaultDateTo,
				Page:                 1, // 1-based pagination.
			},
			err: nil,
			validationErrs: &Validator{
				Errors: map[string]string{},
			},
		},
		{
			name:     "defaults with page 2",
			inputURL: "http://127.0.0.1:8080/invoices/?date-from=2025-06-01&date-to=2025-05-01&search=search string&page=2",
			searchForm: &SearchForm{
				ReconciliationStatus: "NotReconciled",
				DateFrom:             time.Date(2025, time.June, 1, 0, 0, 0, 0, time.UTC),
				DateTo:               time.Date(2025, time.May, 1, 0, 0, 0, 0, time.UTC),
				SearchString:         "search string",
				Page:                 2,
			},
			err: nil,
			validationErrs: &Validator{
				Errors: map[string]string{
					"date-to": "End date cannot be before the start date.",
				},
			},
		},
		{
			name:     "all fields specified",
			inputURL: "http://127.0.0.1:8080/invoices/?status=NotReconciled&date-from=2025-06-01&date-to=2025-07-01&search=search string",
			searchForm: &SearchForm{
				ReconciliationStatus: "NotReconciled",
				DateFrom:             time.Date(2025, time.June, 1, 0, 0, 0, 0, time.UTC),
				DateTo:               time.Date(2025, time.July, 1, 0, 0, 0, 0, time.UTC),
				SearchString:         "search string",
				Page:                 1, // 1-based pagination.
			},
			err: nil,
			validationErrs: &Validator{
				Errors: map[string]string{},
			},
		},
		{
			name:     "default status",
			inputURL: "http://127.0.0.1:8080/invoices/?date-from=2025-06-01&date-to=2025-05-01&search=search string",
			searchForm: &SearchForm{
				ReconciliationStatus: "NotReconciled",
				DateFrom:             time.Date(2025, time.June, 1, 0, 0, 0, 0, time.UTC),
				DateTo:               time.Date(2025, time.May, 1, 0, 0, 0, 0, time.UTC),
				SearchString:         "search string",
				Page:                 1, // 1-based pagination.
			},
			err: nil,
			validationErrs: &Validator{
				Errors: map[string]string{
					"date-to": "End date cannot be before the start date.",
				},
			},
		},
		{
			name:     "invalid dateto",
			inputURL: "http://127.0.0.1:8080/invoices/?status=Reconciled&date-from=2025-06-01&date-to=2025-05-01&search=search string",
			searchForm: &SearchForm{
				ReconciliationStatus: "Reconciled",
				DateFrom:             time.Date(2025, time.June, 1, 0, 0, 0, 0, time.UTC),
				DateTo:               time.Date(2025, time.May, 1, 0, 0, 0, 0, time.UTC),
				SearchString:         "search string",
				Page:                 1, // 1-based pagination.
			},
			err: nil,
			validationErrs: &Validator{
				Errors: map[string]string{
					"date-to": "End date cannot be before the start date.",
				},
			},
		},
		{
			name:     "invalid datefrom",
			inputURL: "http://127.0.0.1:8080/invoices/?status=Reconciled&date-from=INVALID-06-01&date-to=2026-05-01&search=search string",
			searchForm: &SearchForm{
				ReconciliationStatus: "Reconciled",
				DateFrom:             time.Time{},
				DateTo:               time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC),
				SearchString:         "search string",
				Page:                 1, // 1-based pagination.
			},
			err: nil,
			validationErrs: &Validator{
				Errors: map[string]string{
					"date-from": "From date must be provided.",
				},
			},
		},
		{
			name:     "invalid status",
			inputURL: "http://127.0.0.1:8080/invoices/?status=XXXXX&date-from=2025-06-01&date-to=2025-07-01&search=search string",
			searchForm: &SearchForm{
				ReconciliationStatus: "XXXXX",
				DateFrom:             time.Date(2025, time.June, 1, 0, 0, 0, 0, time.UTC),
				DateTo:               time.Date(2025, time.July, 1, 0, 0, 0, 0, time.UTC),
				SearchString:         "search string",
				Page:                 1, // 1-based pagination.
			},
			err: nil,
			validationErrs: &Validator{
				Errors: map[string]string{
					"status": "Invalid status value provided.",
				},
			},
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("%d_%s", ii, tt.name), func(t *testing.T) {
			simulatedRequest := newRequest(t, tt.inputURL)
			form := NewSearchForm()
			if err := DecodeURLParams(simulatedRequest, form); err != nil {
				if tt.err != err {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}

			validator := NewValidator()
			form.Validate(validator)

			if diff := cmp.Diff(form, tt.searchForm); diff != "" {
				t.Errorf("unexpected searchform diff %s", diff)
			}

			if diff := cmp.Diff(validator, tt.validationErrs); diff != "" {
				t.Errorf("unexpected validation diff %s", diff)
			}
		})
	}
}

// TestURLParse tests the validQuery function.
func TestURLParse(t *testing.T) {

	tests := []struct {
		name   string
		url    string
		keys   []string
		vals   url.Values
		hasErr error
	}{
		{
			name: "two parameters ok",
			url:  "http://test.com?hi=there&ok=fine",
			keys: []string{"hi", "ok"},
			vals: url.Values{
				"hi": []string{"there"},
				"ok": []string{"fine"},
			},
			hasErr: nil,
		},
		{
			name: "ok extraneous argument",
			url:  "http://test.com?hi=there&ok=fine&not=needed",
			keys: []string{"hi", "ok"},
			vals: url.Values{
				"hi":  []string{"there"},
				"not": []string{"needed"},
				"ok":  []string{"fine"},
			},
			hasErr: nil,
		},
		{
			name:   "two parameters ok",
			url:    "http://test.com?hi=there",
			keys:   []string{"hi", "ok"},
			hasErr: errors.New(`"ok" not in url`),
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("%d_%s", ii, tt.name), func(t *testing.T) {
			u, err := url.Parse(tt.url)
			if err != nil {
				t.Fatal(err)
			}
			vq, err := validQuery(u, tt.keys...)
			if err != nil {
				if tt.hasErr == nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got, want := tt.hasErr.Error(), err.Error(); got != want {
					t.Errorf("got error %q want %q", got, want)
				}
				return
			}
			if diff := cmp.Diff(tt.vals, vq); diff != "" {
				t.Errorf("unexpected diff %v", diff)
			}

		})
	}
}

// TestValidMuxVars tests the validMuxVars function.
func TestValidMuxVars(t *testing.T) {

	tests := []struct {
		name   string
		vars   map[string]string
		keys   []string
		hasErr error
	}{
		{
			name:   "hi ok fine",
			vars:   map[string]string{"hi": "there", "ok": "fine"},
			keys:   []string{"hi", "ok"},
			hasErr: nil,
		},
		{
			name:   "ok missing",
			vars:   map[string]string{"hi": "there", "not": "here"},
			keys:   []string{"hi", "ok"},
			hasErr: errors.New(`parameter "ok" missing`),
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("%d_%s", ii, tt.name), func(t *testing.T) {

			_, err := validMuxVars(tt.vars, tt.keys...)
			if err != nil {
				if tt.hasErr == nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got, want := tt.hasErr.Error(), err.Error(); got != want {
					t.Errorf("got error %q want %q", got, want)
				}
				return
			}
		})
	}
}

// TestFormLinkOrUnlink tests the LinkOrUnlinkForm.
func TestFormLinkOrUnlink(t *testing.T) {
	tests := []struct {
		name        string
		formData    map[string][]string
		routeParams map[string]string
		isErr       bool
	}{
		{
			name: "form ok",
			formData: map[string][]string{
				"donation-ids": []string{"1", "3", "5"},
			},
			routeParams: map[string]string{
				"type":   "invoice",
				"id":     "e181a801-0829-11f1-b473-7404f143aa1c",
				"action": "link",
			},
			isErr: false,
		},
		{
			name: "form error with empty donation ids",
			formData: map[string][]string{
				"donation-ids": []string{},
			},
			routeParams: map[string]string{
				"type":   "invoice",
				"id":     "e181a801-0829-11f1-b473-7404f143aa1c",
				"action": "link",
			},
			isErr: true,
		},
		{
			name: "form error with incorrect action",
			formData: map[string][]string{
				"donation-ids": []string{"1", "3", "5"},
			},
			routeParams: map[string]string{
				"type":   "invoice",
				"id":     "e181a801-0829-11f1-b473-7404f143aa1c",
				"action": "some-action",
			},
			isErr: true,
		},
	}
	for ii, tt := range tests {
		t.Run(fmt.Sprintf("%d_%s", ii, tt.name), func(t *testing.T) {
			form, err := CheckLinkOrUnlinkForm(tt.formData, tt.routeParams)
			if err != nil {
				t.Fatal(err)
			}
			validator := NewValidator()
			form.Validate(validator)
			if !validator.Valid() {
				if tt.isErr == false {
					t.Errorf("unexpected validation errors: %v", validator.Errors)
				}
				return
			}
			if tt.isErr {
				t.Error("expected validation error")
			}

			// t.Logf("form:\n%#v\n", form)
		})
	}
}
