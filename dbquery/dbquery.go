package dbquery

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jmoiron/sqlx" // helper library
	_ "modernc.org/sqlite"    // pure go sqlite driver
)

type parameterizedStmt struct {
	namedStatement *sqlx.NamedStmt
	args           []string
}

// DB provides a wrapper around the sql.DB connection for application-specific database operations.
type DB struct {
	*sqlx.DB
	accountCodes string

	// Prepared statements.
	getInvoicesStmt          *parameterizedStmt
	getBankTransactionsStmt  *parameterizedStmt
	getDonationsStmt         *parameterizedStmt
	getInvoiceWRStmt         *parameterizedStmt
	getBankTransactionWRStmt *parameterizedStmt
}

// New creates a new connection to an SQLite database at the given path.
func New(dbPath, sqlDir string, accountCodes string) (*DB, error) {
	dbDB, err := sql.Open("sqlite", fmt.Sprintf("%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", dbPath))
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(sqlDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("sql directory %q not found", sqlDir)
	}

	// RegisterFunctions registers the custom REXEXP function. This can
	// occur per call to "New" as it is a singleton using sync.Once.
	RegisterFunctions()

	if err := dbDB.Ping(); err != nil {
		return nil, err
	}

	// Wrap the standard library *sql.DB with sqlx.
	db := &DB{
		DB:           sqlx.NewDb(dbDB, "sqlite"),
		accountCodes: accountCodes,
	}

	// Prepare all the statements.
	db.getInvoicesStmt, err = db.prepNamedStatement(filepath.Join(sqlDir, "invoices.sql"))
	if err != nil {
		return nil, fmt.Errorf("invoices statement error: %w", err)
	}
	db.getBankTransactionsStmt, err = db.prepNamedStatement(filepath.Join(sqlDir, "bank_transactions.sql"))
	if err != nil {
		return nil, fmt.Errorf("bank_transactions statement error: %w", err)
	}
	db.getDonationsStmt, err = db.prepNamedStatement(filepath.Join(sqlDir, "donations.sql"))
	if err != nil {
		return nil, fmt.Errorf("donations statement error: %w", err)
	}
	db.getInvoiceWRStmt, err = db.prepNamedStatement(filepath.Join(sqlDir, "invoice.sql"))
	if err != nil {
		return nil, fmt.Errorf("invoice statement error: %w", err)
	}
	db.getBankTransactionWRStmt, err = db.prepNamedStatement(filepath.Join(sqlDir, "bank_transaction.sql"))
	if err != nil {
		return nil, fmt.Errorf("bank_transaction statement error: %w", err)
	}

	return db, nil
}

// prepareNamedStatment prepares the SQL queries.
func (db *DB) prepNamedStatement(filePath string) (*parameterizedStmt, error) {
	query, err := ParameterizeFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not parameterize %q: %w", filePath, err)
	}

	pQuery, err := db.PrepareNamed(string(query.Body))
	if err != nil {
		return nil, fmt.Errorf("could not prepare statement %q: %w", filePath, err)
	}
	return &parameterizedStmt{
		pQuery,
		query.Parameters,
	}, nil
}

// Invoice is the concrete type of each row returned by GetInvoices.
type Invoice struct {
	InvoiceID     string    `db:"id"`
	InvoiceNumber string    `db:"invoice_number"`
	Date          time.Time `db:"date"`
	ContactName   string    `db:"contact_name"`
	Status        string    `db:"status"`
	Total         float64   `db:"total"`
	DonationTotal float64   `db:"donation_total"`
	CRMSTotal     float64   `db:"crms_total"`
	IsReconciled  bool      `db:"is_reconciled"`
	RowCount      int       `db:"row_count"`
	// UpdatedDateUTC string     `db:"UpdatedDateUTC"`
	// Reference      string     `db:"Reference,omitempty"`
	// AmountPaid     float64    `json:"AmountPaid"`
}

// GetInvoices gets invoices with summed up line item and donation
// values. It isn't necessary to run this query in a transaction.
func (db *DB) GetInvoices(ctx context.Context, reconciliationStatus string, dateFrom, dateTo time.Time, search string, limit, offset int) ([]Invoice, error) {

	// Set named statement and parameter list.
	stmt := db.getInvoicesStmt.namedStatement
	params := db.getInvoicesStmt.args

	// Determine reconciliation status.
	switch reconciliationStatus {
	case "All", "Reconciled", "NotReconciled":
	default:
		return nil, fmt.Errorf(
			"reconciliation must be one of All, Reconciled or NotReconciled, got %q",
			reconciliationStatus,
		)
	}

	// Args uses sqlx's named query capability.
	namedArgs := map[string]any{
		"DateFrom":             dateFrom.Format("2006-01-02"),
		"DateTo":               dateTo.Format("2006-01-02"),
		"AccountCodes":         db.accountCodes,
		"ReconciliationStatus": reconciliationStatus,
		"TextSearch":           search,
		"HereLimit":            limit,
		"HereOffset":           offset,
	}
	if got, want := len(namedArgs), len(params); got != want {
		fmt.Println(params)
		return nil, fmt.Errorf("namedArgs has %d arguments, expected %d", got, want)
	}

	// Use sqlx to scan results into the provided slice.
	var invoices []Invoice
	err := stmt.SelectContext(ctx, &invoices, namedArgs)
	if err != nil {
		return nil, fmt.Errorf("invoices select error: %v", err)
	}

	// Return early if no rows were returned.
	if len(invoices) == 0 {
		return nil, sql.ErrNoRows
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
	Status        string    `db:"status"`
	Total         float64   `db:"total"`
	DonationTotal float64   `db:"donation_total"`
	CRMSTotal     float64   `db:"crms_total"`
	IsReconciled  bool      `db:"is_reconciled"`
	RowCount      int       `db:"row_count"`
	// UpdatedDateUTC string     `db:"UpdatedDateUTC"`
	// Status         string     `db:"Status"`
	// Reference      string     `db:"Reference,omitempty"`
	// AmountPaid     float64    `json:"AmountPaid"`
}

// GetBankTransactions gets bank transactions with summed up line item
// and donation values. It isn't necessary to run this query in a transaction.
func (db *DB) GetBankTransactions(ctx context.Context, reconciliationStatus string, dateFrom, dateTo time.Time, search string, limit, offset int) ([]BankTransaction, error) {

	// Set named statement and parameter list.
	stmt := db.getBankTransactionsStmt.namedStatement
	params := db.getBankTransactionsStmt.args

	// Determine reconciliation status.
	switch reconciliationStatus {
	case "All", "Reconciled", "NotReconciled":
	default:
		return nil, fmt.Errorf(
			"reconciliation must be one of All, Reconciled or NotReconciled, got %q",
			reconciliationStatus,
		)
	}

	// Args uses sqlx's named query capability.
	namedArgs := map[string]any{
		"DateFrom":             dateFrom.Format("2006-01-02"),
		"DateTo":               dateTo.Format("2006-01-02"),
		"AccountCodes":         db.accountCodes,
		"ReconciliationStatus": reconciliationStatus,
		"TextSearch":           search,
		"HereLimit":            limit,
		"HereOffset":           offset,
	}
	if got, want := len(namedArgs), len(params); got != want {
		return nil, fmt.Errorf("namedArgs has %d arguments, expected %d", got, want)
	}

	// Use sqlx to scan results into the provided slice.
	var transactions []BankTransaction
	err := stmt.SelectContext(ctx, &transactions, namedArgs)
	if err != nil {
		return nil, fmt.Errorf("bank transactions select error: %v", err)
	}

	// Return early if no rows were returned.
	if len(transactions) == 0 {
		return nil, sql.ErrNoRows
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
	IsLinked        bool       `db:"is_linked"`
	RowCount        int        `db:"row_count"`
}

// GetDonations retrieves donations from the database with the specified
// filters.
func (db *DB) GetDonations(ctx context.Context, dateFrom, dateTo time.Time, linkageStatus, payoutReference, search string, limit, offset int) ([]Donation, error) {

	// Set named statement and parameter list.
	stmt := db.getDonationsStmt.namedStatement
	params := db.getDonationsStmt.args

	// Determine reconciliation status.
	switch linkageStatus {
	case "All", "Linked", "NotLinked":
	default:
		return nil, fmt.Errorf(
			"linkage status must be one of All, Linked or NotLinked, got %q",
			linkageStatus,
		)
	}

	// Args uses sqlx's named query capability.
	namedArgs := map[string]any{
		"DateFrom":        dateFrom.Format("2006-01-02"),
		"DateTo":          dateTo.Format("2006-01-02"),
		"LinkageStatus":   linkageStatus,
		"PayoutReference": payoutReference,
		"TextSearch":      search,
		"HereLimit":       limit,
		"HereOffset":      offset,
	}
	if got, want := len(namedArgs), len(params); got != want {
		return nil, fmt.Errorf("namedArgs has %d arguments, expected %d", got, want)
	}

	// Use sqlx to scan results into the provided slice.
	var donations []Donation
	err := stmt.SelectContext(ctx, &donations, namedArgs)
	if err != nil {
		return nil, fmt.Errorf("donations select error: %v", err)
	}
	// Return early if no rows were returned.
	if len(donations) == 0 {
		return nil, sql.ErrNoRows
	}
	return donations, nil
}

// WRInvoice is the invoice component of a wide rows invoice with line
// items query.
type WRInvoice struct {
	ID            string    `db:"id"`
	InvoiceNumber string    `db:"invoice_number"`
	Date          time.Time `db:"date"`
	Type          *string   `db:"type"`
	Status        string    `db:"status"`
	Reference     *string   `db:"reference"`
	ContactName   string    `db:"contact_name"`
	Total         float64   `db:"total"`
	DonationTotal float64   `db:"donation_total"`
	CRMSTotal     float64   `db:"crms_total"`
	IsReconciled  *bool     `db:"is_reconciled"`
}

// WRLineItem is the line item component of a wide rows invoice with
// line items query. All values could be null.
type WRLineItem struct {
	AccountCode    *string  `db:"li_account_code"`
	AccountName    *string  `db:"account_name"`
	Description    *string  `db:"li_description"`
	TaxAmount      *float64 `db:"li_tax_amount"`
	LineAmount     *float64 `db:"li_line_amount"`
	DonationAmount *float64 `db:"li_donation_amount"`
}

// GetInvoiceWR (a wide rows query) retrieves a single invoice from
// the database with it's constituent line items. This query returns
// rows for each line item.
func (db *DB) GetInvoiceWR(ctx context.Context, invoiceID string) (WRInvoice, []WRLineItem, error) {

	// Set named statement and parameter list.
	stmt := db.getInvoiceWRStmt.namedStatement
	params := db.getInvoiceWRStmt.args

	// invoiceWithLineItems is the concrete type of each row returned by
	// GetInvoiceWR.
	type invoiceWithLineItems struct {
		WRInvoice
		WRLineItem
	}

	// invoicesWithLineItems is a slice of InvoiceWithLineItems.
	type invoicesWLI []invoiceWithLineItems

	// Initialise the invoice return type.
	var invoice WRInvoice

	// Args uses sqlx's named query capability.
	namedArgs := map[string]any{
		"AccountCodes": db.accountCodes,
		"InvoiceID":    invoiceID,
	}
	if got, want := len(namedArgs), len(params); got != want {
		return invoice, nil, fmt.Errorf("namedArgs has %d arguments, expected %d", got, want)
	}

	// Use sqlx to scan results into the provided slice.
	var iwli invoicesWLI
	err := stmt.SelectContext(ctx, &iwli, namedArgs)
	if err != nil {
		return invoice, nil, fmt.Errorf("invoice select error: %v", err)
	}

	// Return early if no errors were returned.
	if len(iwli) == 0 {
		return invoice, nil, sql.ErrNoRows
	}

	// Return invoice and child line items.
	invoice = iwli[0].WRInvoice
	lineItems := make([]WRLineItem, len(iwli))
	for i, li := range iwli {
		lineItems[i] = li.WRLineItem
	}
	return invoice, lineItems, nil
}

// WRTransaction is the bank transaction component of a wide rows bank
// transaction with line items query.
type WRTransaction struct {
	ID            string    `db:"id"`
	Reference     *string   `db:"reference"`
	Date          time.Time `db:"date"`
	Type          *string   `db:"type"`
	Status        string    `db:"status"`
	ContactName   string    `db:"contact_name"`
	Total         float64   `db:"total"`
	DonationTotal float64   `db:"donation_total"`
	CRMSTotal     float64   `db:"crms_total"`
	IsReconciled  *bool     `db:"is_reconciled"`
}

// GetTransactionWR (a wide rows query) retrieves a single invoice from
// the database with it's constituent line items. This query returns
// rows for each line item.
func (db *DB) GetTransactionWR(ctx context.Context, transactionID string) (WRTransaction, []WRLineItem, error) {

	// Set named statement and parameter list.
	stmt := db.getBankTransactionWRStmt.namedStatement
	params := db.getBankTransactionWRStmt.args

	// transactionWithLineItems is the concrete type of each row returned by
	// GetTransactionWR.
	type transactionWithLineItems struct {
		WRTransaction
		WRLineItem
	}

	// transactionsWithLineItems is a slice of transactionWithLineItems.
	type transactionsWLI []transactionWithLineItems

	// Initialise the transaction return type.
	var transaction WRTransaction

	// Args uses sqlx's named query capability.
	namedArgs := map[string]any{
		"AccountCodes":      db.accountCodes,
		"BankTransactionID": transactionID,
	}
	if got, want := len(namedArgs), len(params); got != want {
		return transaction, nil, fmt.Errorf("namedArgs has %d arguments, expected %d", got, want)
	}

	// Use sqlx to scan results into the provided slice.
	var twli transactionsWLI
	err := stmt.SelectContext(ctx, &twli, namedArgs)
	if err != nil {
		return transaction, nil, fmt.Errorf("transaction select error: %v", err)
	}

	// Return early if no errors were returned.
	if len(twli) == 0 {
		return transaction, nil, sql.ErrNoRows
	}

	// Return transaction and child line items.
	transaction = twli[0].WRTransaction
	lineItems := make([]WRLineItem, len(twli))
	for i, li := range twli {
		lineItems[i] = li.WRLineItem
	}
	return transaction, lineItems, nil
}
