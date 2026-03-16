// package domain provides the coordinated capabilities of the reconciler service.
package domain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/rorycl/reconciler/apiclients/salesforce"
	"github.com/rorycl/reconciler/db"
)

// ErrUsage is an error in usage
type ErrUsage struct {
	detail string
	msg    string // user facing message
}

func (e ErrUsage) Error() string {
	return fmt.Sprintf("%s: %s", e.detail, e.msg)
}

// ErrSystem is a system error, potentially recording a domain logic issue or an
// infrastructure problem such as an interrupted network or external API error.
type ErrSystem struct {
	detail string
	err    error
	msg    string // user facing message
}

func (e ErrSystem) Error() string {
	return fmt.Sprintf("%s: %s: %v", e.detail, e.msg, e.err)
}

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

// BankTransactionsGet retrieves the bank transactions relating to the search terms.
func (r *Reconciler) BankTransactionsGet(
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
			detail: "db.DonationsGet error",
			err:    err,
			msg:    "A problem was encountered retrieving donations",
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
			detail: "db.InvoiceWRGet error",
			err:    err,
			msg:    "A problem was encountered retrieving the invoice details",
		}
	}
	if err == sql.ErrNoRows {
		return invoice, nil, ErrUsage{
			detail: "db.InvoiceWRGet not found error",
			msg:    "The requested invoice was not found",
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
			detail: "db.BankTransactionWRGet error",
			err:    err,
			msg:    "A problem was encountered retrieving the transaction details",
		}
	}
	if err == sql.ErrNoRows {
		return transaction, nil, ErrUsage{
			detail: "db.BankTransactionWRGet not found error",
			msg:    "The requested transaction was not found",
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
					detail: "InvoiceWRGet error",
					msg:    fmt.Sprintf("Invoice %q could not be found", id),
				}
			}
			return "", rt, ErrSystem{
				detail: "InvoiceWRGet error",
				err:    err,
				msg:    fmt.Sprintf("An error was encountered retrieving invoice %q", id),
			}

		}
		return invoice.InvoiceNumber, invoice.Date, nil
	case "transaction":
		transaction, _, err := r.db.BankTransactionWRGet(ctx, id)
		if err != nil {
			if err == sql.ErrNoRows {
				return "", rt, ErrUsage{
					detail: "BankTransactionWRGet error",
					msg:    fmt.Sprintf("Transaction %q could not be found", id),
				}
			}
			return "", rt, ErrSystem{
				detail: "BankTransactionWRGet error",
				err:    err,
				msg:    fmt.Sprintf("An error was encountered retreiving transaction %q", id),
			}
		}
		ref := *transaction.Reference
		return ref, transaction.Date, nil
	default:
		return "", rt, ErrSystem{
			detail: "InvoiceOrBankTransactionInfoGet error",
			err:    errors.New("invalid typer provided"),
			msg:    "An invalid record type was requested",
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
			detail: "LinkUnlinkDonations error",
			msg:    "no records were provided to link/unlink",
		}
	}

	// Update the donations. If it is an unlink action, update the dfk with "", else
	// the actual dfk from the bank transaction or invoice. The form contents (many
	// salesforce IDs given the same DFK reference) must be translated to
	// a slice of salesforce.IDRef, hence the use of form.AsSalesforceIDRefs.
	_, err := sfClient.BatchUpdateOpportunityRefs(ctx, idRefs, false)
	if err != nil {
		return ErrSystem{
			detail: "BatchUpdateOpportunityRefs error",
			err:    err,
			msg:    "A problem was encountered batch updating salesforce references",
		}
	}

	// Upsert the updated opportunities.
	// The refresh window is rough; double upserts shouldn't be a major issue.
	updatedDonations, err := sfClient.GetOpportunities(ctx, dataStartDate, lastRefreshed)
	if err != nil {
		return ErrSystem{
			detail: "GetOpportunities error",
			err:    err,
			msg:    "A problem was encountered retrieving updated salesforce records",
		}
	}
	if err := r.db.UpsertDonations(ctx, updatedDonations); err != nil {
		return ErrSystem{
			detail: "UpsertDonations error",
			err:    err,
			msg:    "A problem was encountered upserting updated salesforce records",
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
				detail: "xero GetOrganisation error",
				err:    err,
				msg:    "A problem was encountered retrieving the Xero organisation record",
			}
		}
		if err := r.db.OrganisationUpsert(ctx, organisation); err != nil {
			return results, ErrSystem{
				detail: "xero OrganisationUpsert error",
				err:    err,
				msg:    "A problem was encountered upserting the Xero organisation record",
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
				detail: "xero GetAccounts error",
				err:    err,
				msg:    "A problem was encountered retrieving the Xero accounts records",
			}
		}
		if err := r.db.AccountsUpsert(ctx, accounts); err != nil {
			return results, ErrSystem{
				detail: "xero GetOrganisation error",
				err:    err,
				msg:    "A problem was encountered retrieving the Xero organisation record",
			}
		}
		results.AccountsNo = len(accounts)
		r.log.Info("retrieved and upserted accounts", "records", results.AccountsNo)
	}

	// Bank Transactions
	transactions, err := xeroClient.GetBankTransactions(ctx, dataStartDate, lastRefresh, accountsRegexp)
	if err != nil {
		return results, ErrSystem{
			detail: "xero GetBankTransactions error",
			err:    err,
			msg:    "A problem was encountered retrieving the Xero bank transactions",
		}
	}
	if err = r.db.BankTransactionsUpsert(ctx, transactions); err != nil {
		return results, ErrSystem{
			detail: "xero BankTransactionsUpsert error",
			err:    err,
			msg:    "A problem was encountered upserting the Xero bank transactions",
		}
	}
	results.TransactionsNo = len(transactions)
	r.log.Info("retrieved and upserted bank transactions", "records", results.TransactionsNo)

	// Invoices
	invoices, err := xeroClient.GetInvoices(ctx, dataStartDate, lastRefresh, accountsRegexp)
	if err != nil {
		return results, ErrSystem{
			detail: "xero GetInvoices error",
			err:    err,
			msg:    "A problem was encountered retrieving the Xero invoices",
		}
	}
	if err := r.db.InvoicesUpsert(ctx, invoices); err != nil {
		return results, ErrSystem{
			detail: "xero InvoicesUpsert error",
			err:    err,
			msg:    "A problem was encountered upserting the Xero invoices",
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
func (r *Reconciler) RefreshSalesforceRecords(
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
			detail: "salesforce GetOpportunities error",
			err:    err,
			msg:    "A problem was encountered retrieving the Salesforce records",
		}
	}

	if err := r.db.UpsertDonations(ctx, donations); err != nil {
		return results, ErrSystem{
			detail: "salesforce UpsertDonations error",
			err:    err,
			msg:    "A problem was encountered upserting the Salesforce records",
		}
	}
	results.RecordsNo = len(donations)
	r.log.Info("retrieved and upserted donations", "records", results.RecordsNo)

	return results, nil

}
