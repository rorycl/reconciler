package web

// types is the interfaces and interface factories needed for a WebApp and testing its
// methods.

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/rorycl/reconciler/apiclients/salesforce"
	"github.com/rorycl/reconciler/apiclients/xero"
	"github.com/rorycl/reconciler/config"
	"github.com/rorycl/reconciler/db"
	"github.com/rorycl/reconciler/domain"
	"github.com/rorycl/reconciler/internal/token"
)

// newDefaultXeroClient returns the default xeroClient as an domain.XeroClient.
func newDefaultXeroClient(ctx context.Context, logger *slog.Logger, accountsRegexp *regexp.Regexp, et *token.ExtendedToken) (domain.XeroClient, error) {
	return xero.NewClient(ctx, logger, accountsRegexp, et)
}

// xeroClientMaker is the signature of a newXeroClient factory function.
type xeroClientMaker func(ctx context.Context, logger *slog.Logger, accountsRegexp *regexp.Regexp, et *token.ExtendedToken) (domain.XeroClient, error)

// NewDefaultSalesforceClient returns the default sfClient as a domain.SalesforceClient.
func newDefaultSalesforceClient(ctx context.Context, cfg *config.Config, logger *slog.Logger, et *token.ExtendedToken) (domain.SalesforceClient, error) {
	return salesforce.NewClient(ctx, cfg, logger, et)
}

// sfClientMaker is the signature of newSalesforceClient.
type sfClientMaker func(ctx context.Context, cfg *config.Config, logger *slog.Logger, et *token.ExtendedToken) (domain.SalesforceClient, error)

// appHandler is a type of handler that returns an error. All normal web handlers are
// appHandlers and are wrapped by ErrorChecker which centralises error reporting.
type appHandler func(http.ResponseWriter, *http.Request) error

// errInternal is an internal error that should trigger a 500 response. The err should
// only be logged, not reported to the client.
type errInternal struct {
	msg string
	err error
}

func (ei errInternal) Error() string {
	return fmt.Sprintf("%s: %s", ei.msg, ei.err)
}

// errUsage is a usage error that should trigger a 4xx response. The required status is
// noted in the status field.
type errUsage struct {
	msg    string
	status int
}

func (eu errUsage) Error() string {
	return fmt.Sprintf("%s (%d)", eu.msg, eu.status)
}

// errHTMX is an error to send back to the htmx front end. The error should be logged
// but not shown to the client -- use msg for that.
type errHTMX struct {
	msg string
	err error
}

func (eh errHTMX) Error() string {
	return fmt.Sprintf("%s: %v", eh.msg, eh.err)
}

// reconcilerer is the snappy name of an interface matching the main methods of
// domain.Reconciler.
type reconcilerer interface {
	// Donations.
	DonationsGet(context.Context, time.Time, time.Time, string, string, string, int, int) ([]domain.ViewDonation, error)
	DonationsLinkUnlink(context.Context, domain.SalesforceClient, []salesforce.IDRef, time.Time, time.Time) error
	// Invoices.
	InvoiceDetailGet(context.Context, string) (db.WRInvoice, []domain.ViewLineItem, error)
	InvoicesGet(context.Context, string, time.Time, time.Time, string, int, int) ([]db.Invoice, error)
	// Transactions (bank transactions).
	TransactionDetailGet(context.Context, string) (db.WRTransaction, []domain.ViewLineItem, error)
	TransactionsGet(context.Context, string, time.Time, time.Time, string, int, int) ([]db.BankTransaction, error)
	// Detail summary for an Invoice or Bank Transaction.
	InvoiceOrBankTransactionInfoGet(context.Context, string, string) (string, time.Time, error)
	// Data refresh.
	SalesforceRecordsRefresh(context.Context, domain.SalesforceClient, time.Time, time.Time) (*domain.RefreshSalesforceResults, error)
	XeroRecordsRefresh(context.Context, domain.XeroClient, time.Time, time.Time, *regexp.Regexp, bool) (*domain.RefreshXeroResults, error)
	// Database.
	DBIsInMemory() bool
	DBPath() string
	Close() error
}
