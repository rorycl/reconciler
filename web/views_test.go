package web

import (
	"html/template"
	"reconciler/db"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

/*
// litterOutput provides a way of dumping a struct.
func litterOutput(data any) string {
	// https://github.com/sanity-io/litter/issues/12#issuecomment-1144643251
	litter.Config.FormatTime = true
	// litter.Config.FieldExclusions = regexp.MustCompile("^(Reader|Encoding)$")
	litter.Config.DisablePointerReplacement = true
	return litter.Sdump(data)
}
*/

func TestViewDonation(t *testing.T) {

	donations := []db.Donation{
		db.Donation{
			ID:              "id123",
			Name:            "name1",
			Amount:          123.4,
			CloseDate:       new(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)),
			PayoutReference: new("payout ref"),
			CreatedDate:     new(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
			CreatedName:     new("creator"),
			ModifiedDate:    new(time.Date(2026, 2, 1, 11, 0, 0, 0, time.UTC)),
			ModifiedName:    new("modname"),
			IsLinked:        false,
			LinkID:          "link id",
			LinkTyper:       "link type",
			RowCount:        1,
		},
		db.Donation{
			ID:              "id_with_missing_fields",
			Name:            "name1",
			Amount:          123.4,
			CloseDate:       nil,
			PayoutReference: nil,
			CreatedDate:     nil,
			CreatedName:     nil,
			ModifiedDate:    nil,
			ModifiedName:    nil,
			IsLinked:        true,
			LinkID:          "link id2",
			LinkTyper:       "link type2",
			RowCount:        2,
		},
	}

	expectedDonations := []viewDonation{
		viewDonation{
			ID:              "id123",
			Name:            "name1",
			Amount:          123.4,
			CloseDateStr:    "01/02/2026",
			PayoutReference: "payout ref",
			CreatedDateStr:  "01/01/2026",
			CreatedName:     "creator",
			ModifiedDateStr: "01/02/2026",
			ModifiedName:    "modname",
			IsLinked:        false,
			LinkID:          "link id",
			LinkTyper:       "link type",
			RowCount:        1,
		},
		viewDonation{
			ID:              "id_with_missing_fields",
			Name:            "name1",
			Amount:          123.4,
			CloseDateStr:    "",
			PayoutReference: template.HTML("&mdash;"),
			CreatedDateStr:  "",
			CreatedName:     "",
			ModifiedDateStr: "",
			ModifiedName:    "",
			IsLinked:        true,
			LinkID:          "link id2",
			LinkTyper:       "link type2",
			RowCount:        2,
		},
	}

	viewDonations := newViewDonations(donations)
	if diff := cmp.Diff(viewDonations, expectedDonations); diff != "" {
		t.Errorf("unexpected diff:\n%s", diff)
	}
}

func TestViewLineItem(t *testing.T) {

	lineItems := []db.WRLineItem{
		db.WRLineItem{
			AccountCode:    new("accode"),
			AccountName:    new("acname"),
			Description:    new("desc"),
			TaxAmount:      new(123.4),
			LineAmount:     new(0.25),
			DonationAmount: new(0.20),
		},
		db.WRLineItem{
			AccountCode:    nil,
			AccountName:    nil,
			Description:    nil,
			TaxAmount:      nil,
			LineAmount:     nil,
			DonationAmount: nil,
		},
	}

	expectedLineItems := []viewLineItem{
		viewLineItem{
			AccountCode:    "accode",
			AccountName:    "acname",
			Description:    "desc",
			TaxAmount:      123.4,
			LineAmount:     0.25,
			DonationAmount: 0.2,
		},
		viewLineItem{
			AccountCode:    "",
			AccountName:    "",
			Description:    "",
			TaxAmount:      0.0,
			LineAmount:     0.0,
			DonationAmount: 0.0,
		},
	}

	viewLineItems := newViewLineItems(lineItems)
	if diff := cmp.Diff(viewLineItems, expectedLineItems); diff != "" {
		t.Errorf("unexpected diff:\n%s", diff)
	}

}
