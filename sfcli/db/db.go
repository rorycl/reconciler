package db

import (
	"context"
	"database/sql"
	"fmt"
	"sfcli/app/salesforce"

	_ "modernc.org/sqlite" // Import the pure Go SQLite driver
)

// DB provides a wrapper around the sql.DB connection for application-specific database operations.
type DB struct {
	*sql.DB
}

// New creates a new connection to an SQLite database at the given path.
// It enables WAL mode for better concurrency and foreign key support.
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

// UpsertOpportunities performs a transactional upsert for a slice of Opportunities.
func (db *DB) UpsertOpportunities(opportunities []salesforce.Opportunity) error {
	if len(opportunities) == 0 {
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

	for _, opp := range opportunities {
		_, err := stmt.ExecContext(context.Background(),
			opp.ID,
			opp.Name,
			opp.Amount,
			opp.CloseDate,
			opp.StageName,
			opp.RecordType.Name,
			opp.PayoutReference, // sql.driver will handle nil pointer correctly
			opp.LastModifiedDate.ToString(),
		)
		if err != nil {
			return fmt.Errorf("failed to upsert opportunity %s: %w", opp.ID, err)
		}
	}

	return tx.Commit()
}
