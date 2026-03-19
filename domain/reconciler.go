// package domain provides the coordinated capabilities of the reconciler service.
package domain

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/rorycl/reconciler/apiclients/salesforce"
	"github.com/rorycl/reconciler/db"
)

// Reconciler represents the main domain operations of the system.
type Reconciler struct {
	db  *db.DB
	log *slog.Logger
}

// NewReconciler creates a new Reconciler.
func NewReconciler(db *db.DB, logger *slog.Logger) *Reconciler {
	return &Reconciler{
		db:  db,
		log: logger,
	}
}

// DBIsInMemory reports if the Reconciler database is an in-memory database.
func (r *Reconciler) DBIsInMemory() bool {
	return strings.Contains(r.db.Path, "memory")
}

// DBPath reports the Reconciler database path.
func (r *Reconciler) DBPath() string {
	return r.db.Path
}

// Close closes the Reconciler database. Other reconciler operations should cease after
// Close is called.
func (r *Reconciler) Close() error {
	return r.db.Close()
}

// InvoicesGet retrieves the invoices relating to the search terms.
func (r *Reconciler) InvoicesGet(
	ctx context.Context,
	status string,
	from time.Time,
	to time.Time,
	search string,
	pageLen int,
	offset int,
) ([]db.Invoice, error) {
	return r.db.InvoicesGet(ctx, status, from, to, search, pageLen, offset)
}

// TransactionsGet retrieves the bank transactions relating to the search terms.
func (r *Reconciler) TransactionsGet(
	ctx context.Context,
	status string,
	from time.Time,
	to time.Time,
	search string,
	pageLen int,
	offset int,
) ([]db.BankTransaction, error) {
	return r.db.BankTransactionsGet(ctx, status, from, to, search, pageLen, offset)
}

// DonationsGet retrieves the donations relating to the search terms, converting them to
// de-pointered objects.
func (r *Reconciler) DonationsGet(
	ctx context.Context,
	from time.Time,
	to time.Time,
	linkage string,
	payoutReference string,
	search string,
	pageLen int,
	offset int,
) ([]ViewDonation, error) {

	donations, err := r.db.DonationsGet(ctx, from, to, linkage, payoutReference, search, pageLen, offset)
	if err != nil && err != sql.ErrNoRows {
		return nil, ErrSystem{
			Detail: "db.DonationsGet error",
			Err:    err,
			Msg:    "A problem was encountered retrieving donations",
		}
	}
	return newViewDonations(donations), err // percolate sql.ErrNoRows if necessary.

}

// InvoiceDetailGet retrieves an invoice and its related line items which are returned
// as de-pointered objects.
func (r *Reconciler) InvoiceDetailGet(
	ctx context.Context,
	invoiceID string,
) (db.WRInvoice, []ViewLineItem, error) {

	var invoice db.WRInvoice
	var lineItems []db.WRLineItem
	var err error
	invoice, lineItems, err = r.db.InvoiceWRGet(ctx, invoiceID)
	if err != nil && err != sql.ErrNoRows {
		return invoice, nil, ErrSystem{
			Detail: "db.InvoiceWRGet error",
			Err:    err,
			Msg:    "A problem was encountered retrieving the invoice details",
		}
	}
	if err == sql.ErrNoRows {
		return invoice, nil, ErrUsage{
			Detail: "db.InvoiceWRGet not found error",
			Msg:    "The requested invoice was not found",
		}
	}
	viewLineItems := newViewLineItems(lineItems)
	return invoice, viewLineItems, nil
}

// TransactionDetailGet retrieves a bank transaction and its related line items which
// are returned as de-pointered objects.
func (r *Reconciler) TransactionDetailGet(
	ctx context.Context,
	transactionID string,
) (db.WRTransaction, []ViewLineItem, error) {

	var transaction db.WRTransaction
	var lineItems []db.WRLineItem
	var err error
	transaction, lineItems, err = r.db.BankTransactionWRGet(ctx, transactionID)
	if err != nil && err != sql.ErrNoRows {
		return transaction, nil, ErrSystem{
			Detail: "db.BankTransactionWRGet error",
			Err:    err,
			Msg:    "A problem was encountered retrieving the transaction details",
		}
	}
	if err == sql.ErrNoRows {
		return transaction, nil, ErrUsage{
			Detail: "db.BankTransactionWRGet not found error",
			Msg:    "The requested transaction was not found",
		}
	}
	viewLineItems := newViewLineItems(lineItems)
	return transaction, viewLineItems, nil

}

// InvoiceOrBankTransactionInfoGet returns the DFK (Invoice ID or Bank Transaction
// Reference) and Date from an invoice or Bank Transaction identified by ID (a uuid).
func (r *Reconciler) InvoiceOrBankTransactionInfoGet(ctx context.Context, typer string, id string) (string, time.Time, error) {
	var rt time.Time

	switch typer {
	case "invoice":
		invoice, _, err := r.db.InvoiceWRGet(ctx, id)
		if err != nil {
			if err == sql.ErrNoRows {
				return "", rt, ErrUsage{
					Detail: "InvoiceWRGet error",
					Msg:    fmt.Sprintf("Invoice %q could not be found", id),
				}
			}
			return "", rt, ErrSystem{
				Detail: "InvoiceWRGet error",
				Err:    err,
				Msg:    fmt.Sprintf("An error was encountered retrieving invoice %q", id),
			}

		}
		return invoice.InvoiceNumber, invoice.Date, nil
	case "bank-transaction":
		transaction, _, err := r.db.BankTransactionWRGet(ctx, id)
		if err != nil {
			if err == sql.ErrNoRows {
				return "", rt, ErrUsage{
					Detail: "BankTransactionWRGet error",
					Msg:    fmt.Sprintf("Transaction %q could not be found", id),
				}
			}
			return "", rt, ErrSystem{
				Detail: "BankTransactionWRGet error",
				Err:    err,
				Msg:    fmt.Sprintf("An error was encountered retreiving transaction %q", id),
			}
		}
		ref := *transaction.Reference
		return ref, transaction.Date, nil
	default:
		return "", rt, ErrSystem{
			Detail: "InvoiceOrBankTransactionInfoGet error",
			Err:    fmt.Errorf("invalid typer %q provided", typer),
			Msg:    "An invalid record type was requested",
		}
	}
}

// DonationsLinkUnlink links or unlinks donations over the API and then updates the
// local record store accordingly.
func (r *Reconciler) DonationsLinkUnlink(
	ctx context.Context,
	sfClient SalesforceClient, // see types.go
	idRefs []salesforce.IDRef,
	dataStartDate time.Time,
	lastRefreshed time.Time,
) error {

	if len(idRefs) == 0 {
		return ErrUsage{
			Detail: "LinkUnlinkDonations error",
			Msg:    "no records were provided to link/unlink",
		}
	}

	// Update the donations. If it is an unlink action, update the dfk with "", else
	// the actual dfk from the bank transaction or invoice. The form contents (many
	// salesforce IDs given the same DFK reference) must be translated to
	// a slice of salesforce.IDRef, hence the use of `salesforce.IDRef`s.
	_, err := sfClient.BatchUpdateOpportunityRefs(ctx, idRefs, false)
	if err != nil {
		return ErrSystem{
			Detail: "BatchUpdateOpportunityRefs error",
			Err:    err,
			Msg:    "A problem was encountered batch updating salesforce references",
		}
	}

	// Upsert the updated opportunities.
	// The refresh window is rough; double upserts shouldn't be a major issue.
	r.log.Info(fmt.Sprintf("GetOpportunities %s %s", dataStartDate.Format(time.DateTime), lastRefreshed.Format(time.DateTime)))
	updatedDonations, err := sfClient.GetOpportunities(ctx, dataStartDate, lastRefreshed)
	if err != nil {
		return ErrSystem{
			Detail: "GetOpportunities error",
			Err:    err,
			Msg:    "A problem was encountered retrieving updated salesforce records",
		}
	}
	if err := r.db.UpsertDonations(ctx, updatedDonations); err != nil {
		return ErrSystem{
			Detail: "UpsertDonations error",
			Err:    err,
			Msg:    "A problem was encountered upserting updated salesforce records",
		}
	}
	return nil
}

// RefreshXeroResults reports the organisation ShortCode and number of accounts
// retrieved and upserted in AccountsNo (when doing a full refresh), together with the
// number of invoices and bank transactions retrieved and upserted.
type RefreshXeroResults struct {
	FullRefresh    bool
	ShortCode      string
	AccountsNo     int
	InvoicesNo     int // the filtered invoices
	TransactionsNo int // the filtered transactions
}

// XeroRecordsRefresh retrieves remote records and updates the local store accordingly.
// If a fullRefresh is not required, not all remote records are retrieved.
func (r *Reconciler) XeroRecordsRefresh(
	ctx context.Context,
	xeroClient XeroClient,
	dataStartDate time.Time,
	lastRefresh time.Time,
	accountsRegexp *regexp.Regexp,
	fullRefresh bool,
) (*RefreshXeroResults, error) {

	results := &RefreshXeroResults{
		FullRefresh: fullRefresh,
	}

	// Organisation.
	if fullRefresh {
		organisation, err := xeroClient.GetOrganisation(ctx)
		if err != nil {
			return results, ErrSystem{
				Detail: "xero GetOrganisation error",
				Err:    err,
				Msg:    "A problem was encountered retrieving the Xero organisation record",
			}
		}
		if err := r.db.OrganisationUpsert(ctx, organisation); err != nil {
			return results, ErrSystem{
				Detail: "xero OrganisationUpsert error",
				Err:    err,
				Msg:    "A problem was encountered upserting the Xero organisation record",
			}
		}
		results.ShortCode = organisation.ShortCode
		r.log.Info("retrieved and upserted organisation record")
	}

	// Accounts
	if fullRefresh {
		accounts, err := xeroClient.GetAccounts(ctx, dataStartDate)
		if err != nil {
			return results, ErrSystem{
				Detail: "xero GetAccounts error",
				Err:    err,
				Msg:    "A problem was encountered retrieving the Xero accounts records",
			}
		}
		if err := r.db.AccountsUpsert(ctx, accounts); err != nil {
			return results, ErrSystem{
				Detail: "xero GetOrganisation error",
				Err:    err,
				Msg:    "A problem was encountered retrieving the Xero organisation record",
			}
		}
		results.AccountsNo = len(accounts)
		r.log.Info("retrieved and upserted accounts", "records", results.AccountsNo)
	}

	// Bank Transactions
	transactions, err := xeroClient.GetBankTransactions(ctx, dataStartDate, lastRefresh, accountsRegexp)
	if err != nil {
		return results, ErrSystem{
			Detail: "xero GetBankTransactions error",
			Err:    err,
			Msg:    "A problem was encountered retrieving the Xero bank transactions",
		}
	}
	if err = r.db.BankTransactionsUpsert(ctx, transactions); err != nil {
		return results, ErrSystem{
			Detail: "xero BankTransactionsUpsert error",
			Err:    err,
			Msg:    "A problem was encountered upserting the Xero bank transactions",
		}
	}
	results.TransactionsNo = len(transactions)
	r.log.Info("retrieved and upserted bank transactions", "records", results.TransactionsNo)

	// Invoices
	invoices, err := xeroClient.GetInvoices(ctx, dataStartDate, lastRefresh, accountsRegexp)
	if err != nil {
		return results, ErrSystem{
			Detail: "xero GetInvoices error",
			Err:    err,
			Msg:    "A problem was encountered retrieving the Xero invoices",
		}
	}
	if err := r.db.InvoicesUpsert(ctx, invoices); err != nil {
		return results, ErrSystem{
			Detail: "xero InvoicesUpsert error",
			Err:    err,
			Msg:    "A problem was encountered upserting the Xero invoices",
		}
	}
	results.InvoicesNo = len(invoices)
	r.log.Info("retrieved and upserted invoices", "records", results.InvoicesNo)

	return results, nil
}

// RefreshSalesforceResults reports the refresh status and number of records retrieved
// and upserted as the result of a SalesforceRecordsRefresh call.
type RefreshSalesforceResults struct {
	FullRefresh bool
	RecordsNo   int
}

// SalesforceRecordsRefresh retrieves remote records and updates the local store accordingly.
func (r *Reconciler) SalesforceRecordsRefresh(
	ctx context.Context,
	sfClient SalesforceClient,
	dataStartDate time.Time,
	lastRefresh time.Time,
) (*RefreshSalesforceResults, error) {

	results := &RefreshSalesforceResults{
		FullRefresh: lastRefresh.IsZero(),
	}

	// Donations.
	donations, err := sfClient.GetOpportunities(ctx, dataStartDate, lastRefresh)
	if err != nil {
		return results, ErrSystem{
			Detail: "salesforce GetOpportunities error",
			Err:    err,
			Msg:    "A problem was encountered retrieving the Salesforce records",
		}
	}

	if err := r.db.UpsertDonations(ctx, donations); err != nil {
		return results, ErrSystem{
			Detail: "salesforce UpsertDonations error",
			Err:    err,
			Msg:    "A problem was encountered upserting the Salesforce records",
		}
	}
	results.RecordsNo = len(donations)
	r.log.Info("retrieved and upserted donations", "records", results.RecordsNo)

	return results, nil

}
