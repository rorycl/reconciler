package db

// tests for salesforce-related database queries

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// Test06 GetDonations(ctx context.Context, dateFrom, dateTo time.Time, linkageStatus, payoutReference, search string, limit, offset int) ([]Donation, error)
// Test09 UpsertOpportunities(ctx context.Context, donations []salesforce.Donation) error

// Test06_DonationsQuery tests searching the donation SQL records.
func Test06_DonationsQuery(t *testing.T) {

	testDB, closeDB := setupTestDB(t)
	t.Cleanup(closeDB)
	ctx := context.Background()

	tests := []struct {
		name            string
		dateFrom        time.Time
		dateTo          time.Time
		linkageStatus   string
		payoutReference string
		searchString    string
		limit, offset   int

		err error

		RecordsNo  int
		lastRecord Donation
	}{
		{
			name:            "all 21 records",
			dateFrom:        time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:          time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			linkageStatus:   "All",
			payoutReference: "",
			searchString:    "",
			limit:           -1,
			offset:          0,
			RecordsNo:       21,
			lastRecord: Donation{
				ID:              "sf-opp-odd-01",
				Name:            "Data Entry Error Donation",
				Amount:          50,
				CloseDate:       ptrTime(time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC)),
				PayoutReference: ptrStr("INV-2025-101"),
				CreatedDate:     nil,
				CreatedName:     nil,
				ModifiedDate:    nil,
				ModifiedName:    nil,
				IsLinked:        true,
				RowCount:        21,
			},
		},
		{
			name:            "all 21 records limited to 0",
			dateFrom:        time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:          time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			linkageStatus:   "All",
			payoutReference: "",
			searchString:    "",
			limit:           0,
			offset:          0,
			err:             sql.ErrNoRows,
		},
		{
			name:            "all 17 linked records",
			dateFrom:        time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:          time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			linkageStatus:   "Linked",
			payoutReference: "",
			searchString:    "",
			limit:           20,
			offset:          0,
			RecordsNo:       17,
			lastRecord: Donation{
				ID:              "sf-opp-odd-01",
				Name:            "Data Entry Error Donation",
				Amount:          50,
				CloseDate:       ptrTime(time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC)),
				PayoutReference: ptrStr("INV-2025-101"),
				CreatedDate:     nil,
				CreatedName:     nil,
				ModifiedDate:    nil,
				ModifiedName:    nil,
				IsLinked:        true,
				RowCount:        17,
			},
		},
		{
			name:            "all 17 linked records limited to last 7",
			dateFrom:        time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:          time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			linkageStatus:   "Linked",
			payoutReference: "",
			searchString:    "",
			limit:           10,
			offset:          10,
			RecordsNo:       7, // number of records after limiting
			lastRecord: Donation{
				ID:              "sf-opp-odd-01",
				Name:            "Data Entry Error Donation",
				Amount:          50,
				CloseDate:       ptrTime(time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC)),
				PayoutReference: ptrStr("INV-2025-101"),
				CreatedDate:     nil,
				CreatedName:     nil,
				ModifiedDate:    nil,
				ModifiedName:    nil,
				IsLinked:        true,
				RowCount:        17, // for pagination
			},
		},
		{
			name:            "search for 1 linked record",
			dateFrom:        time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:          time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			linkageStatus:   "Linked",
			payoutReference: "INV-2025-101",
			searchString:    "data entry", // a regex which is a lower() to lower() match so (sort of) an iregex
			limit:           -1,
			offset:          0,
			RecordsNo:       1,
			lastRecord: Donation{
				ID:              "sf-opp-odd-01",
				Name:            "Data Entry Error Donation",
				Amount:          50,
				CloseDate:       ptrTime(time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC)),
				PayoutReference: ptrStr("INV-2025-101"),
				CreatedDate:     nil,
				CreatedName:     nil,
				ModifiedDate:    nil,
				ModifiedName:    nil,
				IsLinked:        true,
				RowCount:        1,
			},
		},
		{
			name:            "list 4 unlinked records",
			dateFrom:        time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:          time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			linkageStatus:   "NotLinked",
			payoutReference: "",
			searchString:    "",
			limit:           -1,
			offset:          0,
			RecordsNo:       4,
			lastRecord: Donation{
				ID:              "sf-opp-odd-02",
				Name:            "Unlinked Donation",
				Amount:          75,
				CloseDate:       ptrTime(time.Date(2025, 4, 30, 0, 0, 0, 0, time.UTC)),
				PayoutReference: nil,
				CreatedDate:     nil,
				CreatedName:     nil,
				ModifiedDate:    nil,
				ModifiedName:    nil,
				IsLinked:        false,
				RowCount:        4,
			},
		},
		{
			name:            "search 1 unlinked record",
			dateFrom:        time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:          time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			linkageStatus:   "NotLinked",
			payoutReference: "",
			searchString:    "unlinked donation", // a regex which is a lower() to lower() match so (sort of) an iregex
			limit:           -1,
			offset:          0,
			RecordsNo:       1,
			lastRecord: Donation{
				ID:              "sf-opp-odd-02",
				Name:            "Unlinked Donation",
				Amount:          75,
				CloseDate:       ptrTime(time.Date(2025, 4, 30, 0, 0, 0, 0, time.UTC)),
				PayoutReference: nil,
				CreatedDate:     nil,
				CreatedName:     nil,
				ModifiedDate:    nil,
				ModifiedName:    nil,
				IsLinked:        false,
				RowCount:        1,
			},
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("%d_%s", ii, tt.name), func(t *testing.T) {

			donations, err := testDB.GetDonations(ctx, tt.dateFrom, tt.dateTo, tt.linkageStatus, tt.payoutReference, tt.searchString, tt.limit, tt.offset)
			if err != nil {
				if err != tt.err {
					t.Fatalf("get donations error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("get donations error: %v", err)
			}
			if got, want := len(donations), tt.RecordsNo; got != want {
				t.Fatalf("got %d records want %d records", got, want)
			}
			if len(donations) == 0 {
				return
			}
			if diff := cmp.Diff(tt.lastRecord, donations[len(donations)-1]); diff != "" {
				t.Error(diff)
			}
		})
	}
}
