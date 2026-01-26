package xero

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// xeroDateRegex is used to extract the milliseconds timestamp from Xero's custom date format.
var xeroDateRegex = regexp.MustCompile(`\/Date\((\d+).*\)\/`)

// parseXeroDate converts a Xero /Date(1234...)/ string into a time.Time object.
func parseXeroDate(xeroDate string) (time.Time, error) {
	matches := xeroDateRegex.FindStringSubmatch(xeroDate)
	if len(matches) != 2 {
		return time.Time{}, fmt.Errorf("invalid xero date format: %s", xeroDate)
	}

	timestamp, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("could not parse timestamp from xero date: %w", err)
	}

	// Xero provides milliseconds, time.Unix needs seconds.
	return time.Unix(timestamp/1000, 0).UTC(), nil
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
	BankTransactionID string      `json:"BankTransactionID"`
	Type              string      `json:"Type"`
	Reference         string      `json:"Reference"`
	Contact           Contact     `json:"Contact"`
	BankAccount       BankAccount `json:"BankAccount"`
	UpdatedDateUTC    string      `json:"UpdatedDateUTC"`
	DateString        string      `json:"DateString"`
	IsReconciled      bool        `json:"IsReconciled"`
	Status            string      `json:"Status"`
	Total             float64     `json:"Total"`
	LineItems         []LineItem  `json:"LineItems"`

	Date    time.Time `json:"-"`
	Updated time.Time `json:"-"`
}

// UnmarshalJSON provides custom JSON decoding for the BankTransaction type.
// It parses Xero's specific date formats into standard time.Time objects.
func (bt *BankTransaction) UnmarshalJSON(data []byte) error {
	type bankTransactionAlias BankTransaction
	alias := &bankTransactionAlias{}

	if err := json.Unmarshal(data, alias); err != nil {
		return err
	}
	*bt = BankTransaction(*alias)

	var err error
	if bt.DateString != "" {
		bt.Date, err = time.Parse("2006-01-02T15:04:05", bt.DateString)
		if err != nil {
			return fmt.Errorf("failed to parse BankTransaction.DateString: %w", err)
		}
	}
	if bt.UpdatedDateUTC != "" {
		bt.Updated, err = parseXeroDate(bt.UpdatedDateUTC)
		if err != nil {
			return fmt.Errorf("failed to parse BankTransaction.UpdatedDateUTC: %w", err)
		}
	}
	return nil
}

// Contact represents the contact associated with a transaction.
type Contact struct {
	ContactID string `json:"ContactID"`
	Name      string `json:"Name"`
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

// BankAccount represents the bank account for the transaction.
type BankAccount struct {
	AccountID string `json:"AccountID"`
	Code      string `json:"Code,omitempty"`
	Name      string `json:"Name"`
}

// InvoiceResponse is the top-level structure of the /Invoices API response.
type InvoiceResponse struct {
	Invoices []Invoice `json:"Invoices"`
}

// Invoice represents a single invoice record.
type Invoice struct {
	InvoiceID      string     `json:"InvoiceID"`
	Type           string     `json:"Type"`
	InvoiceNumber  string     `json:"InvoiceNumber"`
	Contact        Contact    `json:"Contact"`
	LineItems      []LineItem `json:"LineItems"`
	DateString     string     `json:"DateString"`
	UpdatedDateUTC string     `json:"UpdatedDateUTC"`
	Status         string     `json:"Status"`
	Reference      string     `json:"Reference,omitempty"`
	Total          float64    `json:"Total"`
	AmountPaid     float64    `json:"AmountPaid"`

	Date    time.Time `json:"-"`
	Updated time.Time `json:"-"`
}

// UnmarshalJSON provides custom JSON decoding for the Invoice type.
// It parses Xero's specific date formats into standard time.Time objects.
func (inv *Invoice) UnmarshalJSON(data []byte) error {
	type invoiceAlias Invoice
	alias := &invoiceAlias{}

	if err := json.Unmarshal(data, alias); err != nil {
		return err
	}
	*inv = Invoice(*alias)

	var err error
	if inv.DateString != "" {
		inv.Date, err = time.Parse("2006-01-02T15:04:05", inv.DateString)
		if err != nil {
			return fmt.Errorf("failed to parse Invoice.DateString: %w", err)
		}
	}
	if inv.UpdatedDateUTC != "" {
		inv.Updated, err = parseXeroDate(inv.UpdatedDateUTC)
		if err != nil {
			return fmt.Errorf("failed to parse Invoice.UpdatedDateUTC: %w", err)
		}
	}
	return nil
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
