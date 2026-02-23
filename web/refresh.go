package web

import (
	"context"
	"fmt"
	"log/slog"
	"reconciler/apiclients/salesforce"
	"reconciler/apiclients/xero"
	"reconciler/db"
	"regexp"
	"time"
)

// refreshXeroRecords retrieves the Xero organisation details, accounts, bank
// transactions and invoices (depending on lastUpdate) and inserts these in the
// database. If lastUpdate is time.IsZero, all records are provided. Otherwise, only
// update the invoices and bank transactions modified since lastUpdate.
//
// Information requiring to be returned can be added to the returnMap.
func refreshXeroRecords(
	ctx context.Context,
	xeroClient *xero.Client,
	db *db.DB,
	log *slog.Logger,
	dataStartDate,
	lastUpdate time.Time,
	accountsRegexp *regexp.Regexp,
) (map[string]string, error) {

	fullUpdate := lastUpdate.IsZero()
	returnMap := map[string]string{}

	// Organisation -- also put org shortcode in session
	if fullUpdate {
		organisation, err := xeroClient.GetOrganisation(ctx)
		if err != nil {
			return nil, fmt.Errorf("organisation retrieval error: %v", err)
		}
		if err := db.OrganisationUpsert(ctx, organisation); err != nil {
			return nil, fmt.Errorf("failed to upsert organisation record: %v", err)
		}
		log.Info("retrieved and upserted organisation record successfully")
		returnMap["xero-shortcode"] = organisation.ShortCode
	}

	// Accounts
	if fullUpdate {
		accounts, err := xeroClient.GetAccounts(ctx, dataStartDate)
		if err != nil {
			return nil, fmt.Errorf("accounts retrieval error: %v", err)
		}
		if err := db.AccountsUpsert(ctx, accounts); err != nil {
			return nil, fmt.Errorf("failed to upsert account records: %v", err)
		}
		log.Info(fmt.Sprintf("retrieved and upserted %d accounts records successfully", len(accounts)))
	}

	// Bank Transactions
	transactions, err := xeroClient.GetBankTransactions(ctx, dataStartDate, lastUpdate, accountsRegexp)
	if err != nil {
		return nil, fmt.Errorf("bank transaction retrieval error: %v", err)
	}
	if err = db.BankTransactionsUpsert(ctx, transactions); err != nil {
		return nil, fmt.Errorf("failed to upsert bank transactions: %v", err)
	}
	log.Info(fmt.Sprintf("retrieved and upserted %d bank transaction records successfully", len(transactions)))

	// Invoices
	invoices, err := xeroClient.GetInvoices(ctx, dataStartDate, lastUpdate, accountsRegexp)
	if err != nil {
		return nil, fmt.Errorf("invoices retrieval error: %v", err)
	}
	if err := db.InvoicesUpsert(ctx, invoices); err != nil {
		return nil, fmt.Errorf("failed to upsert invoices: %v", err)
	}
	log.Info(fmt.Sprintf("retrieved and upserted %d invoice records successfully", len(invoices)))

	return returnMap, nil
}

// refreshSalesforceRecords retrieves the Salesforce donation (opportunity) records.
func refreshSalesforceRecords(
	ctx context.Context,
	sfClient *salesforce.Client,
	db *db.DB,
	log *slog.Logger,
	dataStartDate,
	lastUpdate time.Time,
) error {

	// Donations.
	donations, err := sfClient.GetOpportunities(ctx, dataStartDate, lastUpdate)
	if err != nil {
		return fmt.Errorf("failed to retrieve donations: %v", err)
	}

	if err := db.UpsertDonations(ctx, donations); err != nil {
		return fmt.Errorf("failed to upsert donations: %v", err)
	}
	log.Info(fmt.Sprintf("retrieved and upserted %d donations records successfully", len(donations)))

	return nil
}
