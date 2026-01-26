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
	"fmt"
	"io/fs"
	"log"
	"strings"

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
	sqlFS        fs.FS

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

var prepareNamedStatementsOnStartup bool = true

// NewConnection creates a new connection to an SQLite database at the given path.
func NewConnection(dbPath string, sqlDir fs.FS, accountCodes string) (*DB, error) {

	// dataSource is the default setting for file-based databases.
	dataSource := fmt.Sprintf("%s?_dataSource=foreign_keys(1)&_dataSource=journal_mode(WAL)", dbPath)

	// for in-memory test databases, check the necessary cached setting is used.
	if strings.Contains(dbPath, ":memory:") {
		if !strings.Contains(dbPath, "cache=shared") {
			return nil, fmt.Errorf("in-memory connection %q should contain '?cache=shared'", dbPath)
		}
		dataSource = dbPath
	}
	dbDB, err := sql.Open("sqlite", dataSource)
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
		sqlFS:        sqlDir,
	}

	// Normally prepared statements are run on startup, but need to be deferred for
	// loading of schema and test data for testing.
	if prepareNamedStatementsOnStartup {
		err = db.prepareNamedStatements()
		if err != nil {
			return nil, fmt.Errorf("could not prepare named statements: %w", err)
		}
	}

	return db, nil
}

// prepareNamedStatements prepares all the named statements for this database connection.
func (db *DB) prepareNamedStatements() error {
	var err error

	// Accounts.
	db.accountUpsertStmt, err = db.prepNamedStatement(db.sqlFS, "account_upsert.sql")
	if err != nil {
		return fmt.Errorf("account upsert statement error: %w", err)
	}

	// Invoices.
	db.invoicesGetStmt, err = db.prepNamedStatement(db.sqlFS, "invoices.sql")
	if err != nil {
		return fmt.Errorf("get invoices statement error: %w", err)
	}
	db.invoiceGetStmt, err = db.prepNamedStatement(db.sqlFS, "invoice.sql")
	if err != nil {
		return fmt.Errorf("get invoice statement error: %w", err)
	}
	db.invoiceUpsertStmt, err = db.prepNamedStatement(db.sqlFS, "invoice_upsert.sql")
	if err != nil {
		return fmt.Errorf("invoice upsert statement error: %w", err)
	}
	db.invoiceLIDeleteStmt, err = db.prepNamedStatement(db.sqlFS, "invoice_lis_delete.sql")
	if err != nil {
		return fmt.Errorf("get invoice line item delete statement error: %w", err)
	}
	db.invoiceLIInsertStmt, err = db.prepNamedStatement(db.sqlFS, "invoice_lis_insert.sql")
	if err != nil {
		return fmt.Errorf("get invoice line item insert statement error: %w", err)
	}

	// Bank Transactions.
	db.bankTransactionsGetStmt, err = db.prepNamedStatement(db.sqlFS, "bank_transactions.sql")
	if err != nil {
		return fmt.Errorf("get bank transactions statement error: %w", err)
	}
	db.bankTransactionGetStmt, err = db.prepNamedStatement(db.sqlFS, "bank_transaction.sql")
	if err != nil {
		return fmt.Errorf("get bank transaction statement error: %w", err)
	}
	db.bankTransactionUpsertStmt, err = db.prepNamedStatement(db.sqlFS, "bank_transaction_upsert.sql")
	if err != nil {
		return fmt.Errorf("bank transaction upsert statement error: %w", err)
	}
	db.bankTransactionLIDeleteStmt, err = db.prepNamedStatement(db.sqlFS, "bank_transaction_lis_delete.sql")
	if err != nil {
		return fmt.Errorf("get bankTransaction line item delete statement error: %w", err)
	}
	db.bankTransactionLIInsertStmt, err = db.prepNamedStatement(db.sqlFS, "bank_transaction_lis_insert.sql")
	if err != nil {
		return fmt.Errorf("get bankTransaction line item insert statement error: %w", err)
	}

	// Donations.
	db.donationsGetStmt, err = db.prepNamedStatement(db.sqlFS, "donations.sql")
	if err != nil {
		return fmt.Errorf("donations statement error: %w", err)
	}
	db.donationUpsertStmt, err = db.prepNamedStatement(db.sqlFS, "donation_upsert.sql")
	if err != nil {
		return fmt.Errorf("donation upsert statement error: %w", err)
	}

	return nil
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
