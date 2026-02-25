package web

import (
	"context"
	"fmt"
	"reconciler/apiclients/salesforce"
	"reconciler/apiclients/xero"
	"reconciler/internal/token"
	"time"
)

// refreshXeroRecords retrieves the Xero organisation details, accounts, bank
// transactions and invoices (depending on lastRefresh) and inserts these in the
// database. If lastRefresh is time.IsZero, all records are provided. Otherwise, only
// the invoices and bank transactions modified since lastRefresh are updated.
//
// Information requiring to be returned can be added to the returnMap, for example
// needed to update session information.
func (web *WebApp) refreshXeroRecords(ctx context.Context) (map[string]string, error) {

	dataStartDate := web.cfg.DataStartDate
	accountsRegexp := web.cfg.DonationAccountCodesAsRegex()

	sessionRefreshKey := "xero-refreshed-datetime"
	updateStart := time.Now()

	// Get the last refreshed time.
	lastRefresh := web.sessions.GetTime(ctx, sessionRefreshKey)
	if !lastRefresh.IsZero() {
		lastRefresh = lastRefresh.Add(refreshDurationWindow) // window for platform updates
	}
	web.log.Info(fmt.Sprintf("Xero last refresh: %s", lastRefresh.Format(time.DateTime)))

	// Retrieve the oauth2 tokens from the session
	xeroToken, err := web.getValidTokenFromSession(ctx, token.XeroToken)
	if err != nil {
		// Todo: report errors to client.
		return nil, fmt.Errorf("failed to refresh xero token: %v", err)
	}

	// Connect the Xero client.
	xeroClient, err := xero.NewClient(ctx, web.log, web.cfg.DonationAccountCodesAsRegex(), xeroToken)
	if err != nil {
		// Todo: report errors to client.
		return nil, fmt.Errorf("failed to create xero client: %v", err)
	}
	web.log.Info("Xero client authenticated successfully.")

	fullUpdate := lastRefresh.IsZero()
	returnMap := map[string]string{}

	// Organisation
	// Note that the shortcode is needed in the session.
	if fullUpdate {
		organisation, err := xeroClient.GetOrganisation(ctx)
		if err != nil {
			return nil, fmt.Errorf("organisation retrieval error: %v", err)
		}
		if err := web.db.OrganisationUpsert(ctx, organisation); err != nil {
			return nil, fmt.Errorf("failed to upsert organisation record: %v", err)
		}
		web.log.Info("retrieved and upserted organisation record successfully")
		returnMap["xero-shortcode"] = organisation.ShortCode
	}

	// Accounts
	if fullUpdate {
		accounts, err := xeroClient.GetAccounts(ctx, dataStartDate)
		if err != nil {
			return nil, fmt.Errorf("accounts retrieval error: %v", err)
		}
		if err := web.db.AccountsUpsert(ctx, accounts); err != nil {
			return nil, fmt.Errorf("failed to upsert account records: %v", err)
		}
		web.log.Info(fmt.Sprintf("retrieved and upserted %d accounts records successfully", len(accounts)))
	}

	// Bank Transactions
	transactions, err := xeroClient.GetBankTransactions(ctx, dataStartDate, lastRefresh, accountsRegexp)
	if err != nil {
		return nil, fmt.Errorf("bank transaction retrieval error: %v", err)
	}
	if err = web.db.BankTransactionsUpsert(ctx, transactions); err != nil {
		return nil, fmt.Errorf("failed to upsert bank transactions: %v", err)
	}
	web.log.Info(fmt.Sprintf("retrieved and upserted %d bank transaction records successfully", len(transactions)))

	// Invoices
	invoices, err := xeroClient.GetInvoices(ctx, dataStartDate, lastRefresh, accountsRegexp)
	if err != nil {
		return nil, fmt.Errorf("invoices retrieval error: %v", err)
	}
	if err := web.db.InvoicesUpsert(ctx, invoices); err != nil {
		return nil, fmt.Errorf("failed to upsert invoices: %v", err)
	}
	web.log.Info(fmt.Sprintf("retrieved and upserted %d invoice records successfully", len(invoices)))

	// Update the session key
	web.sessions.Put(ctx, sessionRefreshKey, updateStart)

	return returnMap, nil
}

// refreshSalesforceRecords retrieves the Salesforce donation (opportunity) records. If
// lastRefresh is time.IsZero() sfUpdate will simply get all records, otherwise only get
// those modified since lastRefresh.
func (web *WebApp) refreshSalesforceRecords(ctx context.Context) error {

	dataStartDate := web.cfg.DataStartDate

	sessionRefreshKey := "sf-refreshed-datetime"
	updateStart := time.Now()

	// Get the last refreshed time.
	lastRefresh := web.sessions.GetTime(ctx, sessionRefreshKey)
	if !lastRefresh.IsZero() {
		lastRefresh = lastRefresh.Add(refreshDurationWindow) // window for platform updates
	}
	web.log.Info(fmt.Sprintf("Salesforce last refresh: %s", lastRefresh.Format(time.DateTime)))

	sfToken, err := web.getValidTokenFromSession(ctx, token.SalesforceToken)
	if err != nil {
		// Todo: report errors to client.
		return fmt.Errorf("failed to refresh saleforce token: %v", err)
	}

	// Connect the Salesforce client.
	sfClient, err := salesforce.NewClient(ctx, web.cfg, web.log, sfToken)
	if err != nil {
		// Todo: report errors to client.
		return fmt.Errorf("failed to create salesforce client: %v", err)
	}

	// Donations.
	donations, err := sfClient.GetOpportunities(ctx, dataStartDate, lastRefresh)
	if err != nil {
		return fmt.Errorf("failed to retrieve donations: %v", err)
	}

	if err := web.db.UpsertDonations(ctx, donations); err != nil {
		return fmt.Errorf("failed to upsert donations: %v", err)
	}
	web.log.Info(fmt.Sprintf("retrieved and upserted %d donations records successfully", len(donations)))

	// Update the session key
	web.sessions.Put(ctx, sessionRefreshKey, updateStart)

	return nil
}
