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
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"reconciler/internal"
	"strings"

	"github.com/jmoiron/sqlx" // helper library
	_ "modernc.org/sqlite"    // pure go sqlite driver
)

//go:embed sql
var SQLEmbeddedFS embed.FS

// testingMode defers setup of the schema and prepared statements
var testingMode = false

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
	if p == nil {
		panic("verify args called on uninitialised parameterized statement")
	}
	if args == nil {
		return fmt.Errorf("empty args received in verifyArgs")
	}
	if p.args == nil {
		return fmt.Errorf("parameterized args is nil")
	}
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
	log          *slog.Logger

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

// NewConnection creates a new connection to an SQLite database at the given path. The
// directory containing the SQL files are either mounted at either the provided sqlDir
// or at the embedded path (SQLEmbeddedFS) which is the default. The accountCodes
// are passed to the sql statements to ensure that only bank transactions and invoices
// containing line items starting with those codes are returned.
func NewConnection(
	dbPath string,
	sqlDir string,
	accountCodes string,
	logger *slog.Logger,
) (*DB, error) {

	// mount the sql fs either using the embedded fs or via the provided path.
	// The path is likely to need to be relative to "here" as ".." type paths are not
	// accepted by fs mounting.
	sqlFS, err := internal.NewFileMount("sql", SQLEmbeddedFS, sqlDir)
	if err != nil {
		return nil, fmt.Errorf("mount error: %v", err)
	}

	// dataSource is the default setting for file-based databases.
	dataSource := fmt.Sprintf("%s?_dataSource=foreign_keys(1)&_dataSource=journal_mode(WAL)", dbPath)

	// for in-memory test databases, check the necessary cached setting is used.
	if strings.Contains(dbPath, ":memory:") {
		if !strings.Contains(dbPath, "?cache=shared") {
			return nil, fmt.Errorf("in-memory connection %q must contain '?cache=shared'", dbPath)
		}
		dataSource = fmt.Sprintf("%s&dataSource=foreign_keys(1)&_dataSource=journal_mode(WAL)", dbPath)
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

	// Logger setup.
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(
			os.Stdout,
			&slog.HandlerOptions{Level: slog.LevelDebug},
		))
	}

	// Wrap the standard library *sql.DB with sqlx.
	db := &DB{
		DB:           sqlx.NewDb(dbDB, "sqlite"),
		accountCodes: accountCodes,
		sqlFS:        sqlFS,
		log:          logger,
	}

	// Return early in testing mode, so that prepared statments and schema loading can
	// be tested..
	if testingMode {
		return db, nil
	}

	// Initialize the data schema. This is idempotent.
	err = db.InitSchema(sqlFS, "schema.sql")
	if err != nil {
		db.log.Error(fmt.Sprintf("schema setup error: %v", err))
		return nil, fmt.Errorf("schema setup error: %w", err)
	}

	// Initialize the databse named statements.
	err = db.prepareNamedStatements()
	if err != nil {
		db.log.Error(fmt.Sprintf("could not prepare named statements: %v", err))
		return nil, fmt.Errorf("could not prepare named statements: %w", err)
	}

	return db, nil
}

// NewConnectionInTestMode runs a new connection in test mode, loading the test data.
func NewConnectionInTestMode(
	dbPath string,
	sqlDir string,
	accountCodes string,
	logger *slog.Logger,
) (*DB, error) {

	if !strings.Contains(dbPath, ":memory:") {
		return nil, fmt.Errorf("db path %q invalid for test mode", dbPath)
	}

	testDB, err := NewConnection(dbPath, sqlDir, accountCodes, logger)
	if err != nil {
		return nil, fmt.Errorf("could not initialise test database: %w", err)
	}

	// Load the schema definitions.
	if err := testDB.InitSchema(testDB.sqlFS, "schema.sql"); err != nil {
		_ = testDB.Close()
		return nil, fmt.Errorf("Failed to initialize schema for test database: %w", err)
	}

	// Load the test data.
	data, err := fs.ReadFile(testDB.sqlFS, "load_data.sql")
	if err != nil {
		return nil, fmt.Errorf("Failed to read file for loading data for test DB: %w", err)
	}
	_, err = testDB.Exec(string(data))
	if err != nil {
		_ = testDB.Close()
		testDB.log.Error(fmt.Sprintf("Failed to load data for test database: %v", err))
		return nil, fmt.Errorf("Failed to load data for test database: %w", err)
	}

	// Prepare the functions and named statements.
	err = testDB.prepareNamedStatements()
	if err != nil {
		testDB.log.Error(fmt.Sprintf("could not prepare named statements: %v", err))
		return nil, fmt.Errorf("could not prepare named statements: %v", err)
	}

	// Run a rough smoke test if desired.
	/*
		err = _donations_smoke_test(testDB)
		if err != nil {
			return nil, fmt.Errorf("smoke test failed: %v", err)
		}
	*/

	return testDB, nil

}

// SetLogLevel adjusts the logging level of the db module.
func (db *DB) SetLogLevel(lvl slog.Level) {
	opts := &slog.HandlerOptions{Level: lvl}
	handler := slog.NewTextHandler(os.Stdout, opts)
	db.log = slog.New(handler)
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
		db.log.Error(fmt.Sprintf("could not prepare statement %q: %v", filePath, err))
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
		db.log.Error(fmt.Sprintf("could not read schema file at %q: %v", filePath, err))
		return fmt.Errorf("could not read schema file at %q: %w", filePath, err)
	}

	_, err = db.ExecContext(context.Background(), string(schema))
	if err != nil {
		db.log.Error(fmt.Sprintf("failed to execute schema initialization: %v", err))
		return fmt.Errorf("failed to execute schema initialization: %w", err)
	}
	return nil
}

// logQuery is for helping debug SQL issues.
func (db *DB) logQuery(name string, stmt *parameterizedStmt, args map[string]any, err error) {
	db.log.Debug(
		fmt.Sprintf(
			"sql: %s\n---\nquery:\n%q\n---\nargs: %#v\nerror: %v\n",
			name,
			stmt.QueryString,
			args,
			err,
		),
	)
}

func _donations_smoke_test(testDB *DB) error {
	// Messy smoke test for donations
	fmt.Println("getting 3 donations")
	type D2 struct {
		Donation
		AddFields *string `db:"additional_fields_json"`
	}
	rows, err := testDB.Queryx("select * from donations limit 3")
	if err != nil {
		return fmt.Errorf("smoke test select error :%v", err)
	}
	for rows.Next() {
		var d D2
		err = rows.StructScan(&d)
		if err != nil {
			return fmt.Errorf("smoke test scan error :%v", err)
		}
		testDB.log.Warn(fmt.Sprintf("row: %#v\n", d))
	}
	return nil
}
