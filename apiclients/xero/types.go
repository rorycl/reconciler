package xero

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// xeroDateRegex is used to extract the milliseconds timestamp from Xero's custom date format.
// Beware of inconsistent `\/` date escaping.
var xeroDateRegex = regexp.MustCompile(`Date\((-?\d+)(?:[+-]\d+)?\)`)

// parseXeroDate converts a Xero /Date(1234...)/ string into a time.Time object.
func parseXeroDate(xeroDate string) (time.Time, error) {
	matches := xeroDateRegex.FindStringSubmatch(xeroDate)
	if len(matches) != 2 {
		return time.Time{}, fmt.Errorf("invalid xero date format: %s", xeroDate)
	}

	timestamp, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("could not parse timestamp %q from xero date: %w", xeroDate, err)
	}

	// Xero provides milliseconds, time.Unix needs seconds.
	return time.Unix(timestamp/1000, 0).UTC(), nil
}

// FlattenedName flattens an obj.Name string to a string.
type FlattenedName string

// UnmarshalJSON implements the json.Unmarshaler interface for a FlattenedName,
// extracting the "Name" field of the object pointed to by the struct tag into the
// string field.
func (fn *FlattenedName) UnmarshalJSON(data []byte) error {
	// Handle the case of a JSON null value.
	if string(data) == "null" {
		*fn = ""
		return nil
	}
	// Use a helper struct to extract the 'Name' field from the object.
	var helper struct {
		Name string `json:"Name"`
	}
	if err := json.Unmarshal(data, &helper); err != nil {
		return err
	}
	*fn = FlattenedName(helper.Name)
	return nil
}

// XeroDateTime is a custom date type.
type XeroDateTime struct {
	time.Time
}

// UnmarshalJSON implements the json.Unmarshaler interface, marshalling a Xero date into
// a time.Time. The stated format falls back to a `/Date(1234...)/` date parser on
// failure.
func (xdt *XeroDateTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "null" || s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02T15:04:05", s)
	if err != nil {
		t, err = parseXeroDate(s) // Xero custom format
		if err != nil {
			return err
		}
	}
	xdt.Time = t
	return nil
}

// Connection represents an organisation as it appears in the /connections endpoint.
type Connection struct {
	ID         string `json:"id"`
	TenantID   string `json:"tenantId"`
	TenantName string `json:"tenantName"`
}

// BankTransactionsResponse is the top-level structure of the API response.
type BankTransactionsResponse struct {
	BankTransactions []BankTransaction `json:"BankTransactions"`
}

// BankTransaction represents a single bank transaction record.
type BankTransaction struct {
	BankTransactionID string       `json:"BankTransactionID"`
	Type              string       `json:"Type"`
	Reference         string       `json:"Reference"`
	Date              XeroDateTime `json:"DateString"`
	Updated           XeroDateTime `json:"UpdatedDateUTC"`
	Status            string       `json:"Status"`
	Total             float64      `json:"Total"`
	IsReconciled      bool         `json:"IsReconciled"`
	LineItems         []LineItem   `json:"LineItems"`
	// Fields promoted from the Contact and BankAccount json objects.
	Contact       string `json:"-"`
	BankAccountID string `json:"-"`
	BankAccount   string `json:"-"`
}

// UnmarshalJSON implements the json.Unmarshaler interface for custom unmarshalling of a
// BankTransaction.
func (bt *BankTransaction) UnmarshalJSON(data []byte) error {

	// type alias to stop recursion.
	type Alias BankTransaction

	// helpers for specified fields.
	type bankAccountHelper struct {
		AccountID string `json:"AccountID"`
		Name      string `json:"Name"`
	}
	type contactHelper struct {
		Name string `json:"Name"`
	}

	// Define an anonymous struct that includes all the fields of BankTransaction
	// (via the Alias) and the specific nested field we want to process.
	helper := &struct {
		*Alias
		BankAccount bankAccountHelper `json:"BankAccount"`
		Contact     contactHelper     `json:"Contact"`
	}{
		// populate main struct fields
		Alias: (*Alias)(bt),
	}

	// Unmarshal the data into our helper struct.
	if err := json.Unmarshal(data, &helper); err != nil {
		return err
	}

	// Assign sub fields to "flattened" names.
	bt.BankAccountID = helper.BankAccount.AccountID
	bt.BankAccount = helper.BankAccount.Name
	bt.Contact = helper.Contact.Name

	return nil
}

// LineItem represents a single line in a transaction or invoice, crucial for splits.
type LineItem struct {
	Description string  `json:"Description"`
	UnitAmount  float64 `json:"UnitAmount"`
	AccountCode string  `json:"AccountCode"`
	LineItemID  string  `json:"LineItemID"`
	Quantity    float64 `json:"Quantity"`
	TaxAmount   float64 `json:"TaxAmount"`
	LineAmount  float64 `json:"LineAmount"`
}

// InvoiceResponse is the top-level structure of the /Invoices API response.
type InvoiceResponse struct {
	Invoices []Invoice `json:"Invoices"`
}

// Invoice represents a single invoice record.
type Invoice struct {
	InvoiceID     string        `json:"InvoiceID"`
	Type          string        `json:"Type"`
	InvoiceNumber string        `json:"InvoiceNumber"`
	Contact       FlattenedName `json:"Contact"`
	Date          XeroDateTime  `json:"DateString"`
	Updated       XeroDateTime  `json:"UpdatedDateUTC"`
	Status        string        `json:"Status"`
	Reference     string        `json:"Reference,omitempty"`
	Total         float64       `json:"Total"`
	AmountPaid    float64       `json:"AmountPaid"`
	LineItems     []LineItem    `json:"LineItems"`
}

// AccountResponse is the top-level structure of the /Accounts API response.
type AccountResponse struct {
	Accounts []Account `json:"Accounts"`
}

// Account represents a single account record.
type Account struct {
	AccountID               string `json:"AccountID"`
	Code                    string `json:"Code"`
	Name                    string `json:"Name"`
	Description             string `json:"Description"`
	Type                    string `json:"Type"`
	TaxType                 string `json:"TaxType"`
	EnablePaymentsToAccount bool   `json:"EnablePaymentsToAccount"`
	Status                  string `json:"Status"`
	UpdatedDateUTC          string `json:"UpdatedDateUTC"`

	// optional
	BankAccountNumber string `json:"BankAccountNumber,omitempty"`
	BankAccountType   string `json:"BankAccountType,omitempty"`
	CurrencyCode      string `json:"CurrencyCode,omitempty"`
	SystemAccount     string `json:"SystemAccount,omitempty"`

	// set from UpdatedDateUTC
	Updated time.Time `json:"-"`
}

// UnmarshalJSON provides custom JSON decoding for the Account type.
// It parses Xero's specific date formats into standard time.Time objects.
func (acc *Account) UnmarshalJSON(data []byte) error {
	type accountAlias Account
	alias := &accountAlias{}

	if err := json.Unmarshal(data, alias); err != nil {
		return err
	}
	*acc = Account(*alias)

	var err error
	if acc.UpdatedDateUTC != "" {
		acc.Updated, err = parseXeroDate(acc.UpdatedDateUTC)
		if err != nil {
			return fmt.Errorf("failed to parse Account.UpdatedDateUTC: %w", err)
		}
	}
	return nil
}
