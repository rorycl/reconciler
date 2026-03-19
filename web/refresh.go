package web

import (
	"context"
	"fmt"
	"time"

	"github.com/rorycl/reconciler/domain"
	"github.com/rorycl/reconciler/internal/token"
)

// refreshXeroRecords retrieves the Xero organisation details, accounts, bank
// transactions and invoices (depending on lastRefresh) and inserts these in the
// database. If lastRefresh is time.IsZero, all records are provided. Otherwise, only
// the invoices and bank transactions modified since lastRefresh are updated.
//
// Information requiring to be returned can be added to the returnMap, for example
// needed to update session information.
func (web *WebApp) refreshXeroRecords(ctx context.Context) (*domain.RefreshXeroResults, error) {

	dataStartDate := web.cfg.DataStartDate
	accountsRegexp := web.cfg.DonationAccountCodesAsRegex()

	sessionRefreshKey := "xero-refreshed-datetime"
	updateStart := time.Now()

	// Get the last refreshed time.
	lastRefresh := web.sessions.GetTime(ctx, sessionRefreshKey)
	if !lastRefresh.IsZero() {
		lastRefresh = lastRefresh.Add(refreshDurationWindow) // window for platform updates
	}
	web.log.Debug("Xero refresh", "time", lastRefresh.Format(time.DateTime))

	// Retrieve the oauth2 tokens from the session
	xeroToken, err := web.getValidTokenFromSession(ctx, token.XeroToken)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh xero token: %w", err)
	}

	// Connect the Xero client.
	xeroClient, err := web.newXeroClient(ctx, web.log, web.cfg.DonationAccountCodesAsRegex(), xeroToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create xero client: %w", err)
	}
	web.log.Debug("Xero client authenticated successfully.")

	fullUpdate := lastRefresh.IsZero()

	// Run the Xero refresher.
	results, err := web.reconciler.XeroRecordsRefresh(
		ctx,
		xeroClient,
		dataStartDate,
		lastRefresh,
		accountsRegexp,
		fullUpdate,
	)
	if err != nil {
		return nil, err
	}
	// Update the session key
	web.sessions.Put(ctx, sessionRefreshKey, updateStart)

	return results, nil
}

// refreshSalesforceRecords retrieves the Salesforce donation (opportunity) records. If
// lastRefresh is time.IsZero() sfUpdate will simply get all records, otherwise only get
// those modified since lastRefresh.
func (web *WebApp) refreshSalesforceRecords(ctx context.Context) (*domain.RefreshSalesforceResults, error) {

	dataStartDate := web.cfg.DataStartDate

	sessionRefreshKey := "sf-refreshed-datetime"
	updateStart := time.Now()

	// Get the last refreshed time.
	lastRefresh := web.sessions.GetTime(ctx, sessionRefreshKey)
	if !lastRefresh.IsZero() {
		lastRefresh = lastRefresh.Add(refreshDurationWindow) // window for platform updates
	}
	web.log.Debug("Salesforce last refresh", "time", lastRefresh.Format(time.DateTime))

	sfToken, err := web.getValidTokenFromSession(ctx, token.SalesforceToken)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh saleforce token: %w", err)
	}

	// Connect the Salesforce client.
	sfClient, err := web.newSFClient(ctx, web.cfg, web.log, sfToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create salesforce client: %w", err)
	}

	// Run the Salesforce refresher.
	results, err := web.reconciler.SalesforceRecordsRefresh(ctx, sfClient, dataStartDate, lastRefresh)
	if err != nil {
		return nil, err
	}

	// Update the session key
	web.sessions.Put(ctx, sessionRefreshKey, updateStart)

	return results, nil
}
