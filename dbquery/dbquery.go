package dbquery

import (
	"context"
	"database/sql"
	"fmt"
	"os"
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
	// UpdatedDateUTC string     `db:"UpdatedDateUTC"`
	// Status         string     `db:"Status"`
	// Reference      string     `db:"Reference,omitempty"`
	Total         float64 `db:"total"`
	DonationTotal float64 `db:"donation_total"`
	CRMSTotal     float64 `db:"crms_total"`
	// AmountPaid     float64    `json:"AmountPaid"`
	IsReconciled bool `db:"is_reconciled"`
}

// GetInvoices gets invoices with summed up line item and donation
// values. It isn't necessary to run this query in a transaction.
func (db *DB) GetInvoices(ctx context.Context, reconciled *bool, dateFrom, dateTo time.Time, search string) ([]Invoice, error) {

	b, err := os.ReadFile("sql/invoices.sql")
	if err != nil {
		return nil, fmt.Errorf("get invoices query file load error: %w", err)
	}

	query, err := Parameterize(b)
	if err != nil {
		return nil, fmt.Errorf("invoices query template error: %w", err)
	}
	_ = os.WriteFile("/tmp/query.sql", query.Body, 0644)

	// Determine reconciliation status.
	var reconciliationStatus string
	switch {
	case reconciled == nil:
		reconciliationStatus = "All"
	case *reconciled:
		reconciliationStatus = "Reconciled"
	default:
		reconciliationStatus = "NotReconciled"
	}

	// Parse the query and map the named parameters.
	stmt, err := db.PrepareNamedContext(ctx, string(query.Body))
	if err != nil {
		return nil, fmt.Errorf("failed to prepare invoices statement: %w", err)
	}
	defer stmt.Close()

	// Args uses sqlx's named query capability. (Keys should not use $
	// prefix.)
	namedArgs := map[string]any{
		"DateFrom":             dateFrom,
		"DateTo":               dateTo,
		"AccountCodes":         db.accountCodes,
		"ReconciliationStatus": reconciliationStatus,
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
