package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"xerocli/app/xero"

	_ "modernc.org/sqlite" // Import the pure Go SQLite driver
)

// DB provides a wrapper around the sql.DB connection for application-specific database operations.
type DB struct {
	*sql.DB
}

// New creates a new connection to an SQLite database at the given path.
// It enables WAL mode for better concurrency and foreign key support.
func New(path string) (*DB, error) {
	// The "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)" query parameters
	// are a convenient way to set essential connection properties.
	db, err := sql.Open("sqlite", fmt.Sprintf("%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", path))
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

	// In SQLite, child records with a foreign key constraint and ON DELETE CASCADE
	// are automatically deleted when the parent is, but we upsert the parent,
	// so we still need to manage the children explicitly. Deleting first is the simplest way.
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
		if _, err := lineDeleteStmt.ExecContext(context.Background(), t.BankTransactionID); err != nil {
			return fmt.Errorf("failed to delete old line items for transaction %s: %w", t.BankTransactionID, err)
		}

		_, err := btStmt.ExecContext(context.Background(),
			t.BankTransactionID, t.Type, t.Status, t.Reference, t.Total, t.IsReconciled,
			t.Date, t.Updated, t.Contact.ContactID, t.Contact.Name, t.BankAccount.AccountID,
			t.BankAccount.Name, t.BankAccount.Code,
		)
		if err != nil {
			return fmt.Errorf("failed to upsert bank transaction %s: %w", t.BankTransactionID, err)
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

// UpsertAccounts performs a transactional upsert for a slice of Account.
func (db *DB) UpsertAccounts(accounts []xero.Account) error {
	if len(accounts) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	accStmt, err := tx.PrepareContext(context.Background(), accUpsertSQL)
	if err != nil {
		return fmt.Errorf("failed to prepare accounts upsert statement: %w", err)
	}
	defer accStmt.Close()

	for _, acc := range accounts {
		_, err := accStmt.ExecContext(context.Background(),
			acc.AccountID, acc.Code, acc.Name, acc.Description, acc.Type,
			acc.TaxType, acc.Status, acc.SystemAccount, acc.CurrencyCode, acc.Updated,
		)
		if err != nil {
			return fmt.Errorf("failed to upsert account %s: %w", acc.AccountID, err)
		}
	}
	return tx.Commit()
}
