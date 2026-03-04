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

	defaultDateFrom, defaultDateTo := defaultDateToAndFrom(
		new(time.Date(2025, time.April, 1, 0, 0, 0, 0, time.UTC)),
		nil,
	)

	tests := []struct {
		name              string
		inputURL          string
		searchForm        *SearchForm
		err               error      // top level errors
		validationErrs    *Validator // validation errors
		fieldNameForError string
		fieldNameIsError  bool
		offSetPageLen     int
		offSetResult      int
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
			offSetPageLen: 20,
			offSetResult:  0, // database offset
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
			offSetPageLen: 9,
			offSetResult:  9, // database offset
		},
		{
			name:     "defaults with page 3",
			inputURL: "http://127.0.0.1:8080/invoices/?date-from=2025-06-01&date-to=2025-05-01&search=search string&page=3",
			searchForm: &SearchForm{
				ReconciliationStatus: "NotReconciled",
				DateFrom:             time.Date(2025, time.June, 1, 0, 0, 0, 0, time.UTC),
				DateTo:               time.Date(2025, time.May, 1, 0, 0, 0, 0, time.UTC),
				SearchString:         "search string",
				Page:                 3,
			},
			err: nil,
			validationErrs: &Validator{
				Errors: map[string]string{
					"date-to": "End date cannot be before the start date.",
				},
			},
			offSetPageLen: 9,
			offSetResult:  18, // database offset
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
			fieldNameForError: "ReconciliationStatus",
			fieldNameIsError:  false,
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
			fieldNameForError: "DateTo",
			fieldNameIsError:  false,
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
			fieldNameForError: "DateFrom",
			fieldNameIsError:  false,
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
			fieldNameForError: "ReconciliationStatus",
			fieldNameIsError:  false,
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("%d_%s", ii, tt.name), func(t *testing.T) {

			simulatedRequest := newRequest(t, tt.inputURL)

			form := NewSearchForm(new(defaultDateFrom), new(defaultDateTo))
			if err := DecodeURLParams(simulatedRequest.URL.Query(), form); err != nil {
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

			if tt.fieldNameForError != "" {
				if got, want := validator.FieldError(tt.fieldNameForError), tt.fieldNameIsError; got != want {
					t.Errorf("got %t want %t in FieldError check", got, want)
				}
			}

			if tt.offSetPageLen > 0 {
				if got, want := form.Offset(tt.offSetPageLen), tt.offSetResult; got != want {
					t.Errorf("offset got %d want %d", got, want)
				}
			}
		})
	}
}

// TestAsURLParams tests encoding to parameters.
func TestAsURLParams(t *testing.T) {

	// xero url
	want := `date-from=2025-06-01&date-to=2025-07-01&page=1&search=search+string&status=NotReconciled`

	sf := &SearchForm{
		ReconciliationStatus: "NotReconciled",
		DateFrom:             time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		DateTo:               time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
		SearchString:         "search string",
		Page:                 1,
		Refresh:              true, // should be omitted
	}
	got, err := sf.AsURLParams()
	if err != nil {
		t.Fatalf("unexpected AsURLParams error: %v", err)
	}
	if got != want {
		t.Errorf("in AsURLParams got:\n%v\nwant:\n:%v\n", got, want)
	}

	// salesforce url
	want = `date-from=2025-06-01&date-to=2025-07-01&page=1&payout-reference=payout-ref&search=search+string&status=All`

	sdf := &SearchDonationsForm{
		LinkageStatus:   "All",
		DateFrom:        time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		DateTo:          time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
		PayoutReference: "payout-ref",
		SearchString:    "search string",
		Page:            1,
		Refresh:         true,
	}
	got, err = sdf.AsURLParams()
	if err != nil {
		t.Fatalf("unexpected salesforce AsURLParams error: %v", err)
	}
	if got != want {
		t.Errorf("in salesforce AsURLParams got:\n%v\nwant:\n:%v\n", got, want)
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

// TestAsSalesforceIDRefs checks how a LinkOrUnlinkForm is converted into a slice of
// salesforce.IDRef.
func TestAsSalesforceIDRefs(t *testing.T) {

	louf1 := &LinkOrUnlinkForm{
		Action:      "link",
		DonationIDs: []string{"don1", "don2", "don3"},
	}
	louf2 := &LinkOrUnlinkForm{
		Action:      "link",
		DonationIDs: []string{},
	}
	louf3 := &LinkOrUnlinkForm{
		Action:      "unlink",
		DonationIDs: []string{"don4", "don5", "don6"},
	}
	louf4 := new(LinkOrUnlinkForm)

	for _, tt := range []*LinkOrUnlinkForm{louf1, louf2, louf3, louf4} {
		idRefs := tt.AsSalesforceIDRefs("def")
		if idRefs == nil {
			if tt == nil {
				continue
			}
			if tt.DonationIDs == nil {
				continue
			}
			t.Fatalf("got unexepected nil idRefs for %#v", tt)
		}
		if got, want := len(idRefs), len(tt.DonationIDs); got != want {
			t.Errorf("got %d want %d idrefs", got, want)
		}
		for _, rec := range idRefs {
			if (tt.Action == "unlink" && rec.Ref != "") ||
				(tt.Action == "link" && rec.Ref != "def") {
				t.Errorf("unexpected ref for action %s in %#v", tt.Action, rec)
			}
		}
	}

}

// TestSearchDonationsForm tests the SearchDonationsForm behaviour.
// Tests: NewSearchDonationsForm
//
//	Validate
//	Offset (database offset)
//	decode/encode (via DecodeURLParams)
func TestSearchDonationsForm(t *testing.T) {

	var pageLen = 5

	tests := []struct {
		name             string
		dateFrom, dateTo *time.Time
		url              string
		defaultURL       string // should always be the same
		afterEncodingURL string // the form after url has been encoded with DecodeURLParams
		isValid          bool
		offset           int // page offset
	}{

		{
			name:             "ok case",
			dateFrom:         new(time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)),
			dateTo:           new(time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)),
			url:              "status=Linked&date-from=2026-01-01&date-to=2026-01-04&payout-reference=pr1&search=hi&page=2",
			defaultURL:       "date-from=2026-01-02&date-to=2026-01-03&page=1&payout-reference=&search=&status=NotLinked",
			afterEncodingURL: "date-from=2026-01-01&date-to=2026-01-04&page=2&payout-reference=pr1&search=hi&status=Linked",
			isValid:          true,
			offset:           5,
		},
		{
			name:       "invalid status",
			dateFrom:   new(time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)),
			dateTo:     new(time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)),
			url:        "status=Nonsense&date-from=2026-01-01&date-to=2026-01-04&payout-reference=pr1&search=hi&page=2",
			defaultURL: "date-from=2026-01-02&date-to=2026-01-03&page=1&payout-reference=&search=&status=NotLinked",
			isValid:    false,
		},
		{
			name:       "invalid from date",
			dateFrom:   new(time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)),
			dateTo:     new(time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)),
			url:        "status=Linked&date-from=&date-to=2026-01-04&payout-reference=pr1&search=hi&page=2",
			defaultURL: "date-from=2026-01-02&date-to=2026-01-03&page=1&payout-reference=&search=&status=NotLinked",
			isValid:    false,
		},
		{
			name:       "to date before from",
			dateFrom:   new(time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)),
			dateTo:     new(time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)),
			url:        "status=Linked&date-from=2026-01-02&date-to=2026-01-01&payout-reference=pr1&search=hi&page=2",
			defaultURL: "date-from=2026-01-02&date-to=2026-01-03&page=1&payout-reference=&search=&status=NotLinked",
			isValid:    false,
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("%d_%s", ii, tt.name), func(t *testing.T) {

			form := NewSearchDonationsForm(tt.dateFrom, tt.dateTo)
			defaultURL, err := form.AsURLParams()
			if err != nil {
				t.Fatal(err) // should never happen on default
			}
			if got, want := defaultURL, tt.defaultURL; got != want {
				t.Errorf("default url mismatch:\n%s\n%s", got, want)
			}

			urlParams, err := url.ParseQuery(tt.url)
			if err != nil {
				t.Fatalf("unexpected url parsequery error: %v", err)
			}

			err = DecodeURLParams(urlParams, form)
			if err != nil {
				t.Fatalf("unexpected decoding params error: %v", err)
			}

			validator := NewValidator()
			form.Validate(validator)
			if got, want := validator.Valid(), tt.isValid; got != want {
				// fmt.Println(validator.Errors)
				t.Errorf("validator got %t want %t", got, want)
			}
			if !validator.Valid() {
				return
			}

			afterEncodingURL, err := form.AsURLParams()
			if err != nil {
				t.Fatalf("unexpected after encoding error: %v", err) // still hopefully unlikely
			}
			if got, want := afterEncodingURL, tt.afterEncodingURL; got != want {
				t.Errorf("after encoding url mismatch:\n%s\n%s", got, want)
			}

			if got, want := form.Offset(pageLen), tt.offset; got != want {
				t.Errorf("offset got %d want %d", got, want)
			}

		})
	}
}
