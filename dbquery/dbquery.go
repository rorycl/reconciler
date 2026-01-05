package dbquery

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx" // helper library
	_ "modernc.org/sqlite"    // pure go sqlite driver
)

// DB provides a wrapper around the sql.DB connection for application-specific database operations.
type DB struct {
	*sqlx.DB
	accountCodes string
}

// New creates a new connection to an SQLite database at the given path.
func New(path string, accountCodes string) (*DB, error) {
	dbDB, err := sql.Open("sqlite", fmt.Sprintf("%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", path))
	if err != nil {
		return nil, err
	}

	// RegisterFunctions registers the custom REXEXP function. This can
	// occur per call to "New" as it is a singleton using sync.Once.
	RegisterFunctions()

	if err := dbDB.Ping(); err != nil {
		return nil, err
	}

	// Wrap the standard library *sql.DB with sqlx.
	db := sqlx.NewDb(dbDB, "sqlite")

	return &DB{db, accountCodes}, nil
}

// Invoice is the concrete type of each row returned by GetInvoices.
type Invoice struct {
	InvoiceID     string    `db:"id"`
	InvoiceNumber string    `db:"invoice_number"`
	Date          time.Time `db:"date"`
	ContactName   string    `db:"contact_name"`
	Total         float64   `db:"total"`
	DonationTotal float64   `db:"donation_total"`
	CRMSTotal     float64   `db:"crms_total"`
	IsReconciled  bool      `db:"is_reconciled"`
	// UpdatedDateUTC string     `db:"UpdatedDateUTC"`
	// Status         string     `db:"Status"`
	// Reference      string     `db:"Reference,omitempty"`
	// AmountPaid     float64    `json:"AmountPaid"`
}

// GetInvoices gets invoices with summed up line item and donation
// values. It isn't necessary to run this query in a transaction.
func (db *DB) GetInvoices(ctx context.Context, reconciliationStatus string, dateFrom, dateTo time.Time, search string) ([]Invoice, error) {

	// Parameterize the sql query file by replacing the example
	// variables.
	query, err := ParameterizeFile("sql/invoices.sql")
	if err != nil {
		return nil, fmt.Errorf("invoices query file error: %w", err)
	}

	// Determine reconciliation status.
	switch reconciliationStatus {
	case "All", "Reconciled", "NotReconciled":
	default:
		return nil, fmt.Errorf(
			"reconciliation must be one of All, Reconciled or NotReconciled, got %q",
			reconciliationStatus,
		)
	}

	// Parse the query and map the named parameters.
	stmt, err := db.PrepareNamedContext(ctx, string(query.Body))
	if err != nil {
		return nil, fmt.Errorf("failed to prepare invoices statement: %w", err)
	}
	defer stmt.Close()
	// _ = os.WriteFile("/tmp/parsed_query.sql", []byte(stmt.QueryString+"\n"+strings.Join(stmt.Params, " | ")), 0644) // temporary

	// Args uses sqlx's named query capability.
	namedArgs := map[string]any{
		"DateFrom":             dateFrom.Format("2006-01-02"),
		"DateTo":               dateTo.Format("2006-01-02"),
		"AccountCodes":         db.accountCodes,
		"ReconciliationStatus": reconciliationStatus,
		"TextSearch":           search,
	}
	if got, want := len(namedArgs), len(query.Parameters); got != want {
		return nil, fmt.Errorf("namedArgs has %d arguments, expected %d", got, want)
	}

	// Use sqlx to scan results into the provided slice.
	var invoices []Invoice
	err = stmt.SelectContext(ctx, &invoices, namedArgs)
	if err != nil {
		return nil, fmt.Errorf("invoices select error: %v", err)
	}
	return invoices, nil
}

// BankTransaction is the concrete type of each row returned by
// GetBankTransactions.
type BankTransaction struct {
	ID            string    `db:"id"`
	Reference     string    `db:"reference"`
	Date          time.Time `db:"date"`
	ContactName   string    `db:"contact_name"`
	Total         float64   `db:"total"`
	DonationTotal float64   `db:"donation_total"`
	CRMSTotal     float64   `db:"crms_total"`
	IsReconciled  bool      `db:"is_reconciled"`
	// UpdatedDateUTC string     `db:"UpdatedDateUTC"`
	// Status         string     `db:"Status"`
	// Reference      string     `db:"Reference,omitempty"`
	// AmountPaid     float64    `json:"AmountPaid"`
}

// GetBankTransactions gets bank transactions with summed up line item
// and donation values. It isn't necessary to run this query in a transaction.
func (db *DB) GetBankTransactions(ctx context.Context, reconciliationStatus string, dateFrom, dateTo time.Time, search string) ([]BankTransaction, error) {

	// Parameterize the sql query file by replacing the example
	// variables.
	query, err := ParameterizeFile("sql/bank_transactions.sql")
	if err != nil {
		return nil, fmt.Errorf("bank transactions query file error: %w", err)
	}

	// Determine reconciliation status.
	switch reconciliationStatus {
	case "All", "Reconciled", "NotReconciled":
	default:
		return nil, fmt.Errorf(
			"reconciliation must be one of All, Reconciled or NotReconciled, got %q",
			reconciliationStatus,
		)
	}

	// Parse the query and map the named parameters.
	stmt, err := db.PrepareNamedContext(ctx, string(query.Body))
	if err != nil {
		return nil, fmt.Errorf("failed to prepare invoices statement: %w", err)
	}
	defer stmt.Close()

	// Args uses sqlx's named query capability.
	namedArgs := map[string]any{
		"DateFrom":             dateFrom.Format("2006-01-02"),
		"DateTo":               dateTo.Format("2006-01-02"),
		"AccountCodes":         db.accountCodes,
		"ReconciliationStatus": reconciliationStatus,
		"TextSearch":           search,
	}
	if got, want := len(namedArgs), len(query.Parameters); got != want {
		return nil, fmt.Errorf("namedArgs has %d arguments, expected %d", got, want)
	}

	// Use sqlx to scan results into the provided slice.
	var transactions []BankTransaction
	err = stmt.SelectContext(ctx, &transactions, namedArgs)
	if err != nil {
		return nil, fmt.Errorf("bank transactions select error: %v", err)
	}
	return transactions, nil
}

// Donation is the concrete type of each row returned by
// GetDonations
type Donation struct {
	ID              string     `db:"id"`
	Name            string     `db:"name"`
	Amount          float64    `db:"amount"`
	CloseDate       *time.Time `db:"close_date"`
	PayoutReference *string    `db:"payout_reference_dfk"`
	CreatedDate     *time.Time `db:"created_date"`
	CreatedName     *string    `db:"created_by_name"`
	ModifiedDate    *time.Time `db:"last_modified_date"`
	ModifiedName    *string    `db:"last_modified_by_name"`
}

// GetDonations retrieves donations from the database with the specified
// filters.
func (db *DB) GetDonations(ctx context.Context, dateFrom, dateTo time.Time, linkageStatus, payoutReference, search string) ([]Donation, error) {

	// Parameterize the sql query file by replacing the example
	// variables.
	query, err := ParameterizeFile("sql/donations.sql")
	if err != nil {
		return nil, fmt.Errorf("donations query file error: %w", err)
	}

	// Determine reconciliation status.
	switch linkageStatus {
	case "All", "Linked", "NotLinked":
	default:
		return nil, fmt.Errorf(
			"linkage status must be one of All, Linked or NotLinked, got %q",
			linkageStatus,
		)
	}

	// Parse the query and map the named parameters.
	stmt, err := db.PrepareNamedContext(ctx, string(query.Body))
	if err != nil {
		return nil, fmt.Errorf("failed to prepare donations statement: %w", err)
	}
	defer stmt.Close()

	// Args uses sqlx's named query capability.
	namedArgs := map[string]any{
		"DateFrom":        dateFrom.Format("2006-01-02"),
		"DateTo":          dateTo.Format("2006-01-02"),
		"LinkageStatus":   linkageStatus,
		"PayoutReference": payoutReference,
		"TextSearch":      search,
	}
	if got, want := len(namedArgs), len(query.Parameters); got != want {
		return nil, fmt.Errorf("namedArgs has %d arguments, expected %d", got, want)
	}

	// Use sqlx to scan results into the provided slice.
	var donations []Donation
	err = stmt.SelectContext(ctx, &donations, namedArgs)
	if err != nil {
		return nil, fmt.Errorf("donations select error: %v", err)
	}
	return donations, nil
}

// InvoiceWithLineItems is the concrete type of each row returned by
// GetInvoiceWLI.
type InvoiceWithLineItems struct {
	ID               string    `db:"id"`
	InvoiceNumber    string    `db:"invoice_number"`
	Date             time.Time `db:"date"`
	Type             *string   `db:"type"`
	Status           string    `db:"status"`
	Reference        *string   `db:"reference"`
	ContactName      string    `db:"contact_name"`
	Total            float64   `db:"total"`
	DonationTotal    float64   `db:"donation_total"`
	CRMSTotal        float64   `db:"crms_total"`
	IsReconciled     bool      `db:"is_reconciled"`
	LiAccountCode    *string   `db:"li_account_code"`
	LiAccountName    *string   `db:"account_name"`
	LiDescription    *string   `db:"li_description"`
	LiTaxAmount      *float64  `db:"li_tax_amount"`
	LiLineAmount     *float64  `db:"li_line_amount"`
	LiDonationAmount *float64  `db:"li_donation_amount"`
}

// InvoicesWithLineItems is a slice of InvoiceWithLineItems.
type InvoicesWithLineItems []InvoiceWithLineItems

// Invoice returns the first invoice in a slice.
func (iwli InvoicesWithLineItems) Invoice() InvoiceWithLineItems {
	if len(iwli) == 0 {
		return InvoiceWithLineItems{}
	}
	return iwli[0]
}

// GetInvoiceWLI retrieves a single invoice from the database with it's
// constituent line items. This query returns rows for each line item.
func (db *DB) GetInvoiceWLI(ctx context.Context, invoiceNumber string) (InvoicesWithLineItems, error) {

	// Parameterize the sql query file by replacing the example
	// variables.
	query, err := ParameterizeFile("sql/invoice.sql")
	if err != nil {
		return nil, fmt.Errorf("invoice query file error: %w", err)
	}

	// Parse the query and map the named parameters.
	stmt, err := db.PrepareNamedContext(ctx, string(query.Body))
	if err != nil {
		return nil, fmt.Errorf("failed to prepare invoice statement: %w", err)
	}
	defer stmt.Close()

	// Args uses sqlx's named query capability.
	namedArgs := map[string]any{
		"AccountCodes":  db.accountCodes,
		"InvoiceNumber": invoiceNumber,
	}
	if got, want := len(namedArgs), len(query.Parameters); got != want {
		return nil, fmt.Errorf("namedArgs has %d arguments, expected %d", got, want)
	}

	// Use sqlx to scan results into the provided slice.
	var iwli InvoicesWithLineItems
	err = stmt.SelectContext(ctx, &iwli, namedArgs)
	if err != nil {
		return nil, fmt.Errorf("invoice select error: %v", err)
	}
	return iwli, nil
}
