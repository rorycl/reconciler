package db

// salesforce.go deals with Salesforce-related database calls.
//
// A donation is a salesforce opportunity in a salesforce instance using the non-profit
// success pack (NPSP).

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

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
