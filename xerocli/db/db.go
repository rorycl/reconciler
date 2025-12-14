package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
	"xerocli/app/xero"
)

// DB provides a wrapper around the sql.DB connection for application-specific database operations.
type DB struct {
	*sql.DB
}

// New creates a new connection to a DuckDB database at the given path.
func New(path string) (*DB, error) {
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &DB{db}, nil
}

// InitSchema creates the necessary tables if they don't already exist.
func (db *DB) InitSchema() error {
	_, err := db.ExecContext(context.Background(), schema)
	if err != nil {
		return fmt.Errorf("failed to execute schema initialization: %w", err)
	}
	return nil
}

// GetBankTransactionUpdatedTime retrieves the last updated timestamp for a single bank transaction.
func (db *DB) GetBankTransactionUpdatedTime(uuid string) (time.Time, error) {
	var updatedTime time.Time
	query := `SELECT updated_at FROM bank_transactions WHERE id = ?;`
	err := db.QueryRowContext(context.Background(), query, uuid).Scan(&updatedTime)
	if err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, fmt.Errorf("record not found in local database")
		}
		return time.Time{}, err
	}
	return updatedTime, nil
}

// UpsertBankTransactions performs a transactional upsert for a slice of BankTransactions.
// It replaces all line items for a given transaction to ensure consistency.
func (db *DB) UpsertBankTransactions(transactions []xero.BankTransaction) error {
	if len(transactions) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // Rollback is a no-op if Commit succeeds

	btStmt, err := tx.PrepareContext(context.Background(), btUpsertSQL)
	if err != nil {
		return fmt.Errorf("failed to prepare bank_transactions upsert statement: %w", err)
	}
	defer btStmt.Close()

	lineDeleteStmt, err := tx.PrepareContext(context.Background(), btLineDeleteSQL)
	if err != nil {
		return fmt.Errorf("failed to prepare line item delete statement: %w", err)
	}
	defer lineDeleteStmt.Close()

	lineInsertStmt, err := tx.PrepareContext(context.Background(), btLineInsertSQL)
	if err != nil {
		return fmt.Errorf("failed to prepare line item insert statement: %w", err)
	}
	defer lineInsertStmt.Close()

	for _, t := range transactions {
		_, err := btStmt.ExecContext(context.Background(),
			t.BankTransactionID, t.Type, t.Status, t.Reference, t.Total, t.IsReconciled,
			t.Date, t.Updated, t.Contact.ContactID, t.Contact.Name, t.BankAccount.AccountID,
			t.BankAccount.Name, t.BankAccount.Code,
		)
		if err != nil {
			return fmt.Errorf("failed to upsert bank transaction %s: %w", t.BankTransactionID, err)
		}

		if _, err := lineDeleteStmt.ExecContext(context.Background(), t.BankTransactionID); err != nil {
			return fmt.Errorf("failed to delete old line items for transaction %s: %w", t.BankTransactionID, err)
		}

		for _, line := range t.LineItems {
			_, err := lineInsertStmt.ExecContext(context.Background(),
				line.LineItemID, t.BankTransactionID, line.Description, line.Quantity,
				line.UnitAmount, line.LineAmount, line.AccountCode, line.TaxAmount,
			)
			if err != nil {
				return fmt.Errorf("failed to insert line item %s for transaction %s: %w", line.LineItemID, t.BankTransactionID, err)
			}
		}
	}

	return tx.Commit()
}

// UpsertInvoices performs a transactional upsert for a slice of Invoices.
// It replaces all line items for a given invoice to ensure consistency.
func (db *DB) UpsertInvoices(invoices []xero.Invoice) error {
	if len(invoices) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	invStmt, err := tx.PrepareContext(context.Background(), invUpsertSQL)
	if err != nil {
		return fmt.Errorf("failed to prepare invoices upsert statement: %w", err)
	}
	defer invStmt.Close()

	lineDeleteStmt, err := tx.PrepareContext(context.Background(), invLineDeleteSQL)
	if err != nil {
		return fmt.Errorf("failed to prepare line item delete statement: %w", err)
	}
	defer lineDeleteStmt.Close()

	lineInsertStmt, err := tx.PrepareContext(context.Background(), invLineInsertSQL)
	if err != nil {
		return fmt.Errorf("failed to prepare line item insert statement: %w", err)
	}
	defer lineInsertStmt.Close()

	for _, inv := range invoices {
		_, err := invStmt.ExecContext(context.Background(),
			inv.InvoiceID, inv.Type, inv.Status, inv.InvoiceNumber, inv.Reference,
			inv.Total, inv.AmountPaid, inv.Date, inv.Updated, inv.Contact.ContactID, inv.Contact.Name,
		)
		if err != nil {
			return fmt.Errorf("failed to upsert invoice %s: %w", inv.InvoiceID, err)
		}

		if _, err := lineDeleteStmt.ExecContext(context.Background(), inv.InvoiceID); err != nil {
			return fmt.Errorf("failed to delete old line items for invoice %s: %w", inv.InvoiceID, err)
		}

		for _, line := range inv.LineItems {
			_, err := lineInsertStmt.ExecContext(context.Background(),
				line.LineItemID, inv.InvoiceID, line.Description, line.Quantity,
				line.UnitAmount, line.LineAmount, line.AccountCode, line.TaxAmount,
			)
			if err != nil {
				return fmt.Errorf("failed to insert line item %s for invoice %s: %w", line.LineItemID, inv.InvoiceID, err)
			}
		}
	}

	return tx.Commit()
}
