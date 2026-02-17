package db

// salesforce.go deals with Salesforce-related database calls.
//
// A donation is a salesforce opportunity in a salesforce instance using the non-profit
// success pack (NPSP).

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reconciler/apiclients/salesforce"
	"time"
)

// Donation is the concrete type of each row returned by
// DonationsGet
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

// DonationsGet retrieves donations from the database with the specified
// filters.
func (db *DB) DonationsGet(ctx context.Context, dateFrom, dateTo time.Time, linkageStatus, payoutReference, search string, limit, offset int) ([]Donation, error) {

	db.log.Info(fmt.Sprintf("DonationsGet %s %s linkage %s <%s> %q", dateFrom.Format("2006-01-02"), dateTo.Format("2006-01-02"), linkageStatus, payoutReference, search))

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
		db.log.Error(fmt.Sprintf("donationsGet verify args error: type %v", err))
		return nil, fmt.Errorf("donations get verify arguments error: %w", err)
	}

	// Use sqlx to scan results into the provided slice.
	var donations []Donation
	err := stmt.SelectContext(ctx, &donations, namedArgs)
	db.logQuery("donations", stmt, namedArgs, err)
	if err != nil {
		db.log.Error(fmt.Sprintf("donations select error with named args %v", err))
		return nil, fmt.Errorf("donations select error with named args %v\nlook for colons in sql\nerror: %w", namedArgs, err)
	}
	// Return early if no rows were returned.
	if len(donations) == 0 {
		return nil, sql.ErrNoRows
	}
	return donations, nil
}

// UpsertDonations upserts donations records into the database.
func (db *DB) UpsertDonations(ctx context.Context, donations []salesforce.Donation) error {
	if len(donations) == 0 {
		db.log.Info("no donations received for upsert")
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		db.log.Error(fmt.Sprintf("upsertDonations: could not begin transaction: %v", err))
		return fmt.Errorf("upsertDonations: could not begin transaction: %w", err)
	}
	defer tx.Rollback() // no-op after commit.

	stmt := db.donationUpsertStmt

	for _, dnt := range donations {
		additionalFieldsJSON, err := json.Marshal(dnt.AdditionalFields)
		if err != nil {
			db.log.Error(fmt.Sprintf("upsertDonations: failed to marshal fields %s: %v",
				dnt.ID,
				err,
			))
			return fmt.Errorf(
				"failed to marshal additional fields for donation %s: %w",
				dnt.ID,
				err,
			)
		}

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
			db.log.Error(fmt.Sprintf("upsertDonations verify arguments err: %v", err))
			return fmt.Errorf("upsertDonations verify arguments error: %w", err)
		}
		_, err = stmt.ExecContext(ctx, namedArgs)
		if err != nil {
			db.log.Error("upsertDonations: failed to upsert donation %s: %v", dnt.ID, err)
			return fmt.Errorf(" upsertDonations: failed to upsert donation %s: %w", dnt.ID, err)
		}
	}

	db.log.Info(fmt.Sprintf("upsertDonations: upserted %d donations successfully", len(donations)))
	return tx.Commit()
}
