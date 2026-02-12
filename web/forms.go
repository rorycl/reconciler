package web

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"time"

	"github.com/gorilla/schema"
)

// ------------------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------------------

// Validator holds a map of validation errors, keyed by the form field name.
type Validator struct {
	Errors map[string]string
}

// NewValidator creates a new, initialized Validator.
func NewValidator() *Validator {
	return &Validator{Errors: make(map[string]string)}
}

// Valid returns true if the Errors map is empty.
func (v *Validator) Valid() bool {
	return len(v.Errors) == 0
}

// AddError adds an error message to the map for a given field if one
// doesn't already exist for that field.
func (v *Validator) AddError(key, message string) {
	if _, exists := v.Errors[key]; !exists {
		v.Errors[key] = message
	}
}

// Check is a helper for conditional validation. If `ok` is false, it
// calls AddError with the provided key and message.
func (v *Validator) Check(ok bool, key, message string) {
	if !ok {
		v.AddError(key, message)
	}
}

// FieldError is a helper to check if the specified field has triggered
// an error.
func (v *Validator) FieldError(field string) bool {
	_, ok := v.Errors[field]
	return ok
}

// ------------------------------------------------------------------------------
// URL query parsing
// ------------------------------------------------------------------------------

// validQuery checks the url query parameters for the desired keys returning a
// url.Values map and error.
func validQuery(thisURL *url.URL, keys ...string) (url.Values, error) {
	vq, err := url.ParseQuery(thisURL.RawQuery)
	if err != nil {
		return nil, fmt.Errorf("url parsequery error: %v", err)
	}
	for _, k := range keys {
		if _, ok := vq[k]; !ok {
			return nil, fmt.Errorf("%q not in url", k)
		}
	}
	return vq, nil
}

// ------------------------------------------------------------------------------
// URL parameter parsing, using gorilla mux.Vars
// ------------------------------------------------------------------------------

// validMuxVars checks that the required keys are in the url route variable parameters,
// such as the `id` in
//
//	"/invoice/{id:[A-Za-z0-9_-]+}"
func validMuxVars(vars map[string]string, keys ...string) (map[string]string, error) {
	for _, key := range keys {
		if _, ok := vars[key]; !ok {
			return nil, fmt.Errorf("parameter %q missing", key)
		}
	}
	return vars, nil
}

// ------------------------------------------------------------------------------
// Forms
// ------------------------------------------------------------------------------

// SearchForm represents the URL query parameter filters (invoices, bank
// transactions, and so on.)
type SearchForm struct {
	ReconciliationStatus string    `schema:"status"`
	DateFrom             time.Time `schema:"date-from"`
	DateTo               time.Time `schema:"date-to"`
	SearchString         string    `schema:"search"`
	Page                 int       `schema:"page"`
}

// defaultDateToAndFrom sets the default dateFrom and dateTo dates.
// Todo: set this from settings.
func defaultDateToAndFrom() (time.Time, time.Time) {
	now := time.Now().UTC()
	year := now.Year()
	if now.Month() < time.April {
		year--
	}

	df := time.Date(year, time.April, 1, 0, 0, 0, 0, time.UTC)
	dt := time.Date(year+1, time.March, 31, 0, 0, 0, 0, time.UTC)
	return df, dt
}

// NewSearchForm creates a SearchForm with defaults.
func NewSearchForm() *SearchForm {
	dateFrom, dateTo := defaultDateToAndFrom()
	return &SearchForm{
		ReconciliationStatus: "NotReconciled",
		DateFrom:             dateFrom,
		DateTo:               dateTo,
		Page:                 1, // 1-based pagination.
	}
}

// Validate checks SearchForm fields and populates Validator with any
// errors. Note tha the `Check` is like an assertion of truth, if that
// fails, the provided message is recorded against the field.
func (f *SearchForm) Validate(v *Validator) {

	// Reconciliation status is one of three valid states.
	allowedStatus := map[string]bool{"All": true, "Reconciled": true, "NotReconciled": true}
	v.Check(allowedStatus[f.ReconciliationStatus], "status", "Invalid status value provided.")

	v.Check(!f.DateTo.Before(f.DateFrom), "date-to", "End date cannot be before the start date.")
	v.Check(!f.DateFrom.IsZero(), "date-from", "From date must be provided.")

	if f.Page < 1 {
		f.Page = 1
	}
}

// Offset calculates the database offset for (1-based) pagination.
func (f *SearchForm) Offset() int {
	return (f.Page - 1) * pageLen
}

// SearchDonationsForm represents the URL query parameter filters for
// donations.
type SearchDonationsForm struct {
	LinkageStatus   string    `schema:"status"`
	DateFrom        time.Time `schema:"date-from"`
	DateTo          time.Time `schema:"date-to"`
	PayoutReference string    `schema:"payout-reference"`
	SearchString    string    `schema:"search"`
	Page            int       `schema:"page"`
}

// NewSearchDonationsForm creates a SearchDonationsForm with defaults.
func NewSearchDonationsForm() *SearchDonationsForm {
	dateFrom, dateTo := defaultDateToAndFrom()
	return &SearchDonationsForm{
		LinkageStatus: "NotLinked",
		DateFrom:      dateFrom,
		DateTo:        dateTo,
		Page:          1, // 1-based pagination.
	}
}

// Validate checks SearchDonationsForm fields and populates Validator with any
// errors. Note tha the `Check` is like an assertion of truth, if that
// fails, the provided message is recorded against the field.
func (f *SearchDonationsForm) Validate(v *Validator) {

	// Reconciliation status is one of three valid states.
	allowedStatus := map[string]bool{"All": true, "Linked": true, "NotLinked": true}
	v.Check(allowedStatus[f.LinkageStatus], "status", "Invalid status value provided.")

	v.Check(!f.DateTo.Before(f.DateFrom), "date-to", "End date cannot be before the start date.")
	v.Check(!f.DateFrom.IsZero(), "date-from", "From date must be provided.")

	if f.Page < 1 {
		f.Page = 1
	}
}

// Offset calculates the database offset for (1-based) pagination.
func (f *SearchDonationsForm) Offset() int {
	return (f.Page - 1) * pageLen
}

// LinkOrUnlinkForm is a form for linking or unlinking donations in Salesforce to a Xero
// Invoice or BankTransaction.
type LinkOrUnlinkForm struct {
	Typer       string   `schema:"type"`
	ID          string   `schema:"id"` // the invoice id or bank-transaction reference
	DFK         string   `schema:"dfk"`
	Action      string   `schema:"action"` // "link" or "unlink"
	DonationIDs []string `schema:"donation-ids"`
}

// CheckLinkOrUnlinkForm coleects the postData and routeVars into a map for schema
// decoding.
func CheckLinkOrUnlinkForm(postData map[string][]string, routeVars map[string]string) (*LinkOrUnlinkForm, error) {
	// collapse the routeVars into to the postData
	for k, v := range routeVars {
		postData[k] = []string{v}
	}

	// decode the form
	var loul LinkOrUnlinkForm
	decoder := newSchemaDecoder()
	if err := decoder.Decode(&loul, postData); err != nil {
		return nil, fmt.Errorf("post data decoding error: %v", err)
	}
	return &loul, nil

}

// Validate valides the link or unlink form.
func (f *LinkOrUnlinkForm) Validate(v *Validator) {
	if f == nil {
		log.Println("no LinkOrUnlinkForm received")
		return
	}

	allowedTyper := map[string]bool{"invoice": true, "bank-transaction": true}
	v.Check(allowedTyper[f.Typer], "status", "Invalid type value provided.")

	v.Check(f.ID != "", "id", "An empty ID was provided")
	v.Check(f.DFK != "", "dfk", "An empty DFK (distributed foreign key reference) was provided")

	allowedActions := map[string]bool{"link": true, "unlink": true}
	v.Check(allowedActions[f.Action], "action", "Invalid action provided.")

	v.Check(len(f.DonationIDs) > 0, "donation-ids", "No donation ids found.")

}

// ------------------------------------------------------------------------------
// General decoding funcs
// ------------------------------------------------------------------------------

// newSchemaDecoder creates a new schema.Decoder instance and registers
// a custom converter for the time.Time type.
func newSchemaDecoder() *schema.Decoder {
	decoder := schema.NewDecoder()

	decoder.RegisterConverter(time.Time{}, func(value string) reflect.Value {
		t, err := time.Parse("2006-01-02", value) // other patterns can be tried here.
		if err != nil {
			return reflect.ValueOf(time.Time{})
		}
		return reflect.ValueOf(t)
	})

	return decoder
}

// DecodeURLParams is helper that decodes URL query parameters from a request
// into a destination struct (dst).
func DecodeURLParams(r *http.Request, dst any) error {
	decoder := newSchemaDecoder()
	if err := decoder.Decode(dst, r.URL.Query()); err != nil {
		return fmt.Errorf("url parameter decoding error: %v", err)
	}
	return nil
}
