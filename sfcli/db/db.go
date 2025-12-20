package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sfcli/app/salesforce"

	_ "modernc.org/sqlite" // Import the pure Go SQLite driver
)

// DB provides a wrapper around the sql.DB connection for application-specific database operations.
type DB struct {
	*sql.DB
}

// New creates a new connection to an SQLite database at the given path.
func New(path string) (*DB, error) {
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

// UpsertOpportunities performs a transactional upsert for a slice of Salesforce Records.
func (db *DB) UpsertOpportunities(records []salesforce.Record) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // Rollback is a no-op if Commit succeeds

	stmt, err := tx.PrepareContext(context.Background(), oppsUpsertSQL)
	if err != nil {
		return fmt.Errorf("failed to prepare opportunities upsert statement: %w", err)
	}
	defer stmt.Close()

	for _, rec := range records {
		additionalFieldsJSON, err := json.Marshal(rec.AdditionalFields)
		if err != nil {
			return fmt.Errorf("failed to marshal additional fields for record %s: %w", rec.ID, err)
		}

		_, err = stmt.ExecContext(context.Background(),
			rec.ID,
			rec.Name,
			rec.Amount,
			rec.CloseDate,
			rec.PayoutReference,
			rec.CreatedDate.Time,      // Pass the underlying time.Time object
			rec.CreatedBy.Name,
			rec.LastModifiedDate.Time, // Pass the underlying time.Time object
			rec.LastModifiedBy.Name,
			string(additionalFieldsJSON),
		)
		if err != nil {
			return fmt.Errorf("failed to upsert opportunity %s: %w", rec.ID, err)
		}
	}

	return tx.Commit()
}
