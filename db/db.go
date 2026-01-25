// Package db provides the database component of the reconciler project.
//
// Althought the current database backend is sqlite to allow for cross-platform desktop
// use, the database is not considered a simple storage layer. Each query below is held
// in an sql file held in the `sql` directory, which can be run on the sqlite command
// line. (For some queries it is advisable to run the sql in a transaction, so that the
// results can be rolled back.)
//
// The use of external, runnable sql files also as Go prepared statements is made
// possible through a novel parameterization scheme, as set out in parameterize.go.
package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"time"

	"reconciler/apiclients/salesforce"
	"reconciler/apiclients/xero"

	"github.com/jmoiron/sqlx" // helper library
	_ "modernc.org/sqlite"    // pure go sqlite driver
)

// parameterizedStmt describes an sql file parsed into an sqlx NamedStmt expecting the
// provided args.
type parameterizedStmt struct {
	sqlFile string
	args    []string
	*sqlx.NamedStmt
}

// verifyArgs determines if the number of arguments provided to a parameterizedStmt is
// as expected. This check could be more thorough.
func (p *parameterizedStmt) verifyArgs(args map[string]any) error {
	if got, want := len(args), len(p.args); got != want {
		return fmt.Errorf(
			"argument length to named statement from %q incorrect: got %d want %d",
			p.sqlFile,
			got,
			want,
		)
	}
	return nil
}

// DB provides a wrapper around the sql.DB connection for application-specific db operations.
type DB struct {
	*sqlx.DB
	accountCodes string

	// Prepared statements.
	accountUpsertStmt *parameterizedStmt

	invoicesGetStmt     *parameterizedStmt
	invoiceGetStmt      *parameterizedStmt
	invoiceUpsertStmt   *parameterizedStmt
	invoiceLIDeleteStmt *parameterizedStmt
	invoiceLIInsertStmt *parameterizedStmt

	bankTransactionsGetStmt     *parameterizedStmt
	bankTransactionGetStmt      *parameterizedStmt
	bankTransactionUpsertStmt   *parameterizedStmt
	bankTransactionLIDeleteStmt *parameterizedStmt
	bankTransactionLIInsertStmt *parameterizedStmt

	donationsGetStmt   *parameterizedStmt
	donationUpsertStmt *parameterizedStmt
}

// New creates a new connection to an SQLite database at the given path.
func New(dbPath string, sqlDir fs.FS, accountCodes string) (*DB, error) {
	dbDB, err := sql.Open("sqlite", fmt.Sprintf("%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", dbPath))
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
	db := &DB{
		DB:           sqlx.NewDb(dbDB, "sqlite"),
		accountCodes: accountCodes,
	}

	// Prepare the statements.
	//
	// Accounts.
	db.accountUpsertStmt, err = db.prepNamedStatement(sqlDir, "account_upsert.sql")
	if err != nil {
		return nil, fmt.Errorf("account upsert statement error: %w", err)
	}

	// Invoices.
	db.invoicesGetStmt, err = db.prepNamedStatement(sqlDir, "invoices.sql")
	if err != nil {
		return nil, fmt.Errorf("get invoices statement error: %w", err)
	}
	db.invoiceGetStmt, err = db.prepNamedStatement(sqlDir, "invoice.sql")
	if err != nil {
		return nil, fmt.Errorf("get invoice statement error: %w", err)
	}
	db.invoiceUpsertStmt, err = db.prepNamedStatement(sqlDir, "invoice_upsert.sql")
	if err != nil {
		return nil, fmt.Errorf("invoice upsert statement error: %w", err)
	}
	db.invoiceLIDeleteStmt, err = db.prepNamedStatement(sqlDir, "invoice_lis_delete.sql")
	if err != nil {
		return nil, fmt.Errorf("get invoice line item delete statement error: %w", err)
	}
	db.invoiceLIInsertStmt, err = db.prepNamedStatement(sqlDir, "invoice_lis_insert.sql")
	if err != nil {
		return nil, fmt.Errorf("get invoice line item insert statement error: %w", err)
	}

	// Bank Transactions.
	db.bankTransactionsGetStmt, err = db.prepNamedStatement(sqlDir, "bank_transactions.sql")
	if err != nil {
		return nil, fmt.Errorf("get bank transactions statement error: %w", err)
	}
	db.bankTransactionGetStmt, err = db.prepNamedStatement(sqlDir, "bank_transaction.sql")
	if err != nil {
		return nil, fmt.Errorf("get bank transaction statement error: %w", err)
	}
	db.bankTransactionUpsertStmt, err = db.prepNamedStatement(sqlDir, "bank_transaction_upsert.sql")
	if err != nil {
		return nil, fmt.Errorf("bank transaction upsert statement error: %w", err)
	}
	db.bankTransactionLIDeleteStmt, err = db.prepNamedStatement(sqlDir, "bank_transaction_lis_delete.sql")
	if err != nil {
		return nil, fmt.Errorf("get bankTransaction line item delete statement error: %w", err)
	}
	db.bankTransactionLIInsertStmt, err = db.prepNamedStatement(sqlDir, "bank_transaction_lis_insert.sql")
	if err != nil {
		return nil, fmt.Errorf("get bankTransaction line item insert statement error: %w", err)
	}

	// Donations.
	db.donationsGetStmt, err = db.prepNamedStatement(sqlDir, "donations.sql")
	if err != nil {
		return nil, fmt.Errorf("donations statement error: %w", err)
	}
	db.donationUpsertStmt, err = db.prepNamedStatement(sqlDir, "donation_upsert.sql")
	if err != nil {
		return nil, fmt.Errorf("donation upsert statement error: %w", err)
	}

	return db, nil
}

// prepareNamedStatment prepares the SQL queries.
func (db *DB) prepNamedStatement(fileFS fs.FS, filePath string) (*parameterizedStmt, error) {
	query, err := ParameterizeFile(fileFS, filePath)
	if err != nil {
		return nil, fmt.Errorf("could not parameterize %q: %w", filePath, err)
	}

	pQuery, err := db.PrepareNamed(string(query.Body))
	if err != nil {
		return nil, fmt.Errorf("could not prepare statement %q: %w", filePath, err)
	}
	return &parameterizedStmt{
		filePath,
		query.Parameters,
		pQuery,
	}, nil
}

// InitSchema creates the necessary tables if they don't already exist. The schema file
// can be run idempotently.
func (db *DB) InitSchema(fileFS fs.FS, filePath string) error {

	schema, err := fs.ReadFile(fileFS, filePath)
	if err != nil {
		return fmt.Errorf("could not read schema file at %q: %w", filePath, err)
	}

	_, err = db.ExecContext(context.Background(), string(schema))
	if err != nil {
		return fmt.Errorf("failed to execute schema initialization: %w", err)
	}
	return nil
}

// UpsertAccounts upserts Xero account records.
func (db *DB) UpsertAccounts(ctx context.Context, accounts []xero.Account) error {
	if len(accounts) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after commit.

	stmt := db.accountUpsertStmt

	for _, acc := range accounts {
		namedArgs := map[string]any{
			"AccountID":     acc.AccountID,
			"Code":          acc.Code,
			"Name":          acc.Name,
			"Description":   acc.Description,
			"Type":          acc.Type,
			"TaxType":       acc.TaxType,
			"Status":        acc.Status,
			"SystemAccount": acc.SystemAccount,
			"CurrencyCode":  acc.CurrencyCode,
			"Updated":       acc.Updated.Format("2006-01-02T15:04:05Z"),
		}
		if err := stmt.verifyArgs(namedArgs); err != nil {
			return err
		}
		_, err := stmt.ExecContext(ctx, namedArgs)
		if err != nil {
			logQuery("accounts", stmt, namedArgs, err)
			return fmt.Errorf("failed to upsert account %s: %w", acc.AccountID, err)
		}
	}
	return tx.Commit()
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
	// Reference      string     `db:"Reference,omitempty"`
	// AmountPaid     float64    `json:"AmountPaid"`
}

// GetInvoices gets invoices with summed up line item and donation
// values. It isn't necessary to run this query in a transaction.
func (db *DB) GetInvoices(ctx context.Context, reconciliationStatus string, dateFrom, dateTo time.Time, search string, limit, offset int) ([]Invoice, error) {

	// Set named statement and parameter list.
	stmt := db.invoicesGetStmt

	// Determine reconciliation status.
	switch reconciliationStatus {
	case "All", "Reconciled", "NotReconciled":
	default:
		return nil, fmt.Errorf(
			"reconciliation must be one of All, Reconciled or NotReconciled, got %q",
			reconciliationStatus,
		)
	}

	// namedArgs uses sqlx's named query capability.
	namedArgs := map[string]any{
		"DateFrom":             dateFrom.Format("2006-01-02"),
		"DateTo":               dateTo.Format("2006-01-02"),
		"AccountCodes":         db.accountCodes,
		"ReconciliationStatus": reconciliationStatus,
		"TextSearch":           search,
		"HereLimit":            limit,
		"HereOffset":           offset,
	}
	if err := stmt.verifyArgs(namedArgs); err != nil {
		return nil, err
	}

	// Scan results into the provided slice.
	var invoices []Invoice
	err := stmt.SelectContext(ctx, &invoices, namedArgs)
	logQuery("invoices", stmt, namedArgs, err)
	if err != nil {
		return nil, fmt.Errorf("invoices select error: %v", err)
	}

	// Return early if no rows were returned.
	if len(invoices) == 0 {
		return nil, sql.ErrNoRows
	}
	return invoices, nil
}

// UpsertInvoices performs a upserts for a slice of Invoices. It replaces all line items
// for each invoice in the set to ensure consistency.
func (db *DB) UpsertInvoices(ctx context.Context, invoices []xero.Invoice) error {
	if len(invoices) == 0 {
		return nil
	}

	// Start transaction.
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after a commit.

	for _, inv := range invoices {

		// Delete any existing line items for this invoice.
		stmt := db.invoiceLIDeleteStmt
		namedArgs := map[string]any{
			"InvoiceID": inv.InvoiceID,
		}
		if err := stmt.verifyArgs(namedArgs); err != nil {
			return err
		}
		_, err := stmt.ExecContext(ctx, namedArgs)
		if err != nil {
			return fmt.Errorf("failed to delete old line items for invoice %s: %w", inv.InvoiceID, err)
		}

		// Upsert the invoice record.
		stmt = db.invoiceUpsertStmt
		namedArgs = map[string]any{
			"InvoiceID":     inv.InvoiceID,
			"Type":          inv.Type,
			"Status":        inv.Status,
			"InvoiceNumber": inv.InvoiceNumber,
			"Reference":     inv.Reference,
			"Total":         inv.Total,
			"AmountPaid":    inv.AmountPaid,
			"Date":          inv.Date.Format("2006-01-02"),
			"Updated":       inv.Updated.Format("2006-01-02T15:04:05Z"),
			"ContactID":     inv.Contact.ContactID,
			"ContactName":   inv.Contact.Name,
		}
		if err := stmt.verifyArgs(namedArgs); err != nil {
			return err
		}
		_, err = stmt.ExecContext(ctx, namedArgs)
		if err != nil {
			return fmt.Errorf("failed to upsert invoice %s: %w", inv.InvoiceID, err)
		}

		// Add the related line items for this invoice.
		for _, line := range inv.LineItems {
			stmt := db.invoiceLIInsertStmt
			namedArgs := map[string]any{
				"LineItemID":  line.LineItemID,
				"InvoiceID":   inv.InvoiceID,
				"Description": line.Description,
				"Quantity":    line.Quantity,
				"UnitAmount":  line.UnitAmount,
				"LineAmount":  line.LineAmount,
				"AccountCode": line.AccountCode,
				"TaxAmount":   line.TaxAmount,
			}
			if err := stmt.verifyArgs(namedArgs); err != nil {
				return err
			}
			_, err := stmt.ExecContext(ctx, namedArgs)
			if err != nil {
				return fmt.Errorf("failed to upsert line item %s invoice %s: %w", line.LineItemID, inv.InvoiceID, err)
			}
		}
	}

	return tx.Commit()
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
	// AmountPaid     float64    `json:"AmountPaid"`
}

// GetBankTransactions gets bank transactions with summed up line item
// and donation values. It isn't necessary to run this query in a transaction.
func (db *DB) GetBankTransactions(ctx context.Context, reconciliationStatus string, dateFrom, dateTo time.Time, search string, limit, offset int) ([]BankTransaction, error) {

	// Set named statement and parameter list.
	stmt := db.bankTransactionsGetStmt

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
	if err := stmt.verifyArgs(namedArgs); err != nil {
		return nil, err
	}

	// Use sqlx to scan results into the provided slice.
	var transactions []BankTransaction
	err := stmt.SelectContext(ctx, &transactions, namedArgs)
	if err != nil {
		logQuery("bank transactions", stmt, namedArgs, err)
		return nil, fmt.Errorf("bank transactions select error: %v", err)
	}

	// Return early if no rows were returned.
	if len(transactions) == 0 {
		return nil, sql.ErrNoRows
	}
	return transactions, nil
}

// UpsertBankTransactions performs upserts for a slice of BankTransactions. It replaces
// all line items for each Bank Transaction (transaction) in the set to ensure
// consistency.
func (db *DB) UpsertBankTransactions(ctx context.Context, transactions []xero.BankTransaction) error {
	if len(transactions) == 0 {
		return nil
	}

	// Start transaction.
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after a commit.

	for _, tr := range transactions {

		// Delete any existing line items for this bank transaction.
		stmt := db.bankTransactionLIDeleteStmt
		namedArgs := map[string]any{
			"BankTransactionID": tr.BankTransactionID,
		}
		if err := stmt.verifyArgs(namedArgs); err != nil {
			return err
		}
		_, err := stmt.ExecContext(ctx, namedArgs)
		if err != nil {
			return fmt.Errorf("failed to delete old line items for transaction %s: %w", tr.BankTransactionID, err)
		}

		// Upsert the new bank transaction.
		stmt = db.bankTransactionUpsertStmt
		namedArgs = map[string]any{
			"BankTransactionID":    tr.BankTransactionID,
			"Type":                 tr.Type,
			"Status":               tr.Status,
			"Reference":            tr.Reference,
			"Total":                tr.Total,
			"IsReconciled":         tr.IsReconciled,
			"Date":                 tr.Date.Format("2006-01-02"),
			"Updated":              tr.Updated.Format("2006-01-02T15:04:05Z"),
			"ContactID":            tr.Contact.ContactID,
			"ContactName":          tr.Contact.Name,
			"BankAccountAccountID": tr.BankAccount.AccountID,
			"BankAccountName":      tr.BankAccount.Name,
			"BankAccountCode":      tr.BankAccount.Code,
		}

		_, err = stmt.ExecContext(ctx, namedArgs)
		if err != nil {
			return fmt.Errorf("failed to upsert bank transaction %s: %w", tr.BankTransactionID, err)
		}

		// Insert the bank transaction line items.
		stmt = db.bankTransactionLIInsertStmt

		for _, line := range tr.LineItems {
			namedArgs := map[string]any{
				"LineItemID":        line.LineItemID,
				"BankTransactionID": tr.BankTransactionID,
				"Description":       line.Description,
				"Quantity":          line.Quantity,
				"UnitAmount":        line.UnitAmount,
				"LineAmount":        line.LineAmount,
				"AccountCode":       line.AccountCode,
				"TaxAmount":         line.TaxAmount,
			}
			if err := stmt.verifyArgs(namedArgs); err != nil {
				return err
			}
			_, err = stmt.ExecContext(ctx, namedArgs)
			if err != nil {
				return fmt.Errorf("failed to insert line item %s for transaction %s: %w", line.LineItemID, tr.BankTransactionID, err)
			}
		}
	}

	return tx.Commit()
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
	CreatedName     *string    `db:"created_by"`
	ModifiedDate    *time.Time `db:"last_modified_date"`
	ModifiedName    *string    `db:"last_modified_by"`
	IsLinked        bool       `db:"is_linked"`
	RowCount        int        `db:"row_count"`
}

// GetDonations retrieves donations from the database with the specified
// filters.
func (db *DB) GetDonations(ctx context.Context, dateFrom, dateTo time.Time, linkageStatus, payoutReference, search string, limit, offset int) ([]Donation, error) {

	log.Printf("GetDonations %s %s linkage %s <%s> %q", dateFrom.Format("2006-01-02"), dateTo.Format("2006-01-02"), linkageStatus, payoutReference, search)

	// Set named statement and parameter list.
	stmt := db.donationsGetStmt

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
	if err := stmt.verifyArgs(namedArgs); err != nil {
		return nil, err
	}

	// Use sqlx to scan results into the provided slice.
	var donations []Donation
	err := stmt.SelectContext(ctx, &donations, namedArgs)
	logQuery("donations", stmt, namedArgs, err)
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
	TotalOutstanding float64   `db:"total_outstanding"`
	IsReconciled     bool      `db:"is_reconciled"`
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
	stmt := db.invoiceGetStmt

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
	if err := stmt.verifyArgs(namedArgs); err != nil {
		return invoice, nil, err
	}

	// Use sqlx to scan results into the provided slice.
	var iwli invoicesWLI
	err := stmt.SelectContext(ctx, &iwli, namedArgs)
	logQuery("invoiceWLI", stmt, namedArgs, err)
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
	ID               string    `db:"id"`
	Reference        *string   `db:"reference"`
	Date             time.Time `db:"date"`
	Type             *string   `db:"type"`
	Status           string    `db:"status"`
	ContactName      string    `db:"contact_name"`
	Total            float64   `db:"total"`
	DonationTotal    float64   `db:"donation_total"`
	CRMSTotal        float64   `db:"crms_total"`
	TotalOutstanding float64   `db:"total_outstanding"`
	IsReconciled     bool      `db:"is_reconciled"`
}

// GetTransactionWR (a wide rows query) retrieves a single bank transaction
// (transaction) from the database with it's constituent line items. This query returns
// rows for each line item.
func (db *DB) GetTransactionWR(ctx context.Context, transactionID string) (WRTransaction, []WRLineItem, error) {

	// Set named statement and parameter list.
	stmt := db.bankTransactionGetStmt

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
	if err := stmt.verifyArgs(namedArgs); err != nil {
		return transaction, nil, err
	}

	// Use sqlx to scan results into the provided slice.
	var twli transactionsWLI
	err := stmt.SelectContext(ctx, &twli, namedArgs)
	logQuery("transactionWLI", stmt, namedArgs, err)
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

// UpsertOpportunities performs a transactional upsert for a slice of Salesforce Records
// into the donations table.
func (db *DB) UpsertOpportunities(ctx context.Context, donations []salesforce.Donation) error {
	if len(donations) == 0 {
		return nil
	}

	// Begin Transaction.
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op if commit succeeds.

	stmt := db.donationUpsertStmt

	for _, dnt := range donations {
		additionalFieldsJSON, err := json.Marshal(dnt.AdditionalFields)
		if err != nil {
			return fmt.Errorf("failed to marshal additional fields for donation %s: %w", dnt.ID, err)
		}

		// namedArgs uses sqlx's named query capability.
		namedArgs := map[string]any{
			"ID":                   dnt.ID,
			"Name":                 dnt.Name,
			"Amount":               dnt.Amount,
			"CloseDate":            dnt.CloseDate.Time,
			"PayoutReference":      dnt.PayoutReference,
			"CreatedDate":          dnt.CreatedDate.Time,
			"CreatedBy":            dnt.CreatedBy,
			"LastModifiedDate":     dnt.LastModifiedDate.Time,
			"LastModifiedBy":       dnt.LastModifiedBy,
			"AdditionalFieldsJSON": string(additionalFieldsJSON),
		}
		if err := stmt.verifyArgs(namedArgs); err != nil {
			return err
		}

		_, err = stmt.ExecContext(ctx, namedArgs)
		if err != nil {
			return fmt.Errorf("failed to upsert donation %s: %w", dnt.ID, err)
		}
	}

	return tx.Commit()
}

// logQuery is for helping debug SQL issues.
func logQuery(name string, stmt *parameterizedStmt, args map[string]any, err error) {
	const debug = false
	if !debug {
		return
	}
	log.Printf(
		"sql: %s\n---\nquery:\n%q\n---\nargs: %#v\nerror: %v\n",
		name,
		stmt.QueryString,
		args,
		err,
	)
}
