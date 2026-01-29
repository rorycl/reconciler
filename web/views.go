package web

/* view types for the web server */

import (
	"html/template"
	"reconciler/db"
)

// viewDonation  is a view version of the db.Donations type,
// with non-pointer fields.
type viewDonation struct {
	ID              string
	Name            string
	Amount          float64
	CloseDateStr    string
	PayoutReference any // string or specific web-safe template.HTML
	CreatedDateStr  string
	CreatedName     string
	ModifiedDateStr string
	ModifiedName    string
	IsLinked        bool
	RowCount        int
}

// newViewDonations maps db.Donation records to a slice of viewDonation.
func newViewDonations(donations []db.Donation) []viewDonation {
	dv := make([]viewDonation, len(donations))
	for i, d := range donations {
		dv[i].ID = d.ID
		dv[i].Name = d.Name
		dv[i].Amount = d.Amount
		dv[i].IsLinked = d.IsLinked
		dv[i].RowCount = d.RowCount
		// de-pointer
		if d.PayoutReference == nil {
			dv[i].PayoutReference = template.HTML("&mdash;")
		} else {
			dv[i].PayoutReference = *d.PayoutReference
		}
		if d.CloseDate != nil {
			dv[i].CloseDateStr = d.CloseDate.Format("02/01/2006")
		}
		if d.CreatedDate != nil {
			dv[i].CreatedDateStr = d.CreatedDate.Format("02/01/2006")
		}
		if d.ModifiedDate != nil {
			dv[i].ModifiedDateStr = d.ModifiedDate.Format("02/01/2006")
		}
		if d.CreatedName != nil {
			dv[i].CreatedName = *d.CreatedName
		}
		if d.ModifiedName != nil {
			dv[i].ModifiedName = *d.ModifiedName
		}
	}
	return dv
}

// viewLineItems is a view version of the db.WRLineItem with
// non-pointer fields.
type viewLineItem struct {
	AccountCode    string
	AccountName    string
	Description    string
	TaxAmount      float64
	LineAmount     float64
	DonationAmount float64
}

// newViewLineItems converts a slice of WRLineItem to a slice of
// viewLineItem.
func newViewLineItems(lineItems []db.WRLineItem) []viewLineItem {
	viewItems := make([]viewLineItem, len(lineItems))
	for i, li := range lineItems {
		if li.AccountCode != nil {
			viewItems[i].AccountCode = *li.AccountCode
		}
		if li.AccountName != nil {
			viewItems[i].AccountName = *li.AccountName
		}
		if li.Description != nil {
			viewItems[i].Description = *li.Description
		}
		if li.TaxAmount != nil {
			viewItems[i].TaxAmount = *li.TaxAmount
		}
		if li.LineAmount != nil {
			viewItems[i].LineAmount = *li.LineAmount
		}
		if li.DonationAmount != nil {
			viewItems[i].DonationAmount = *li.DonationAmount
		}
	}
	return viewItems
}
