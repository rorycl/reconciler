package web

// types is the interfaces and interface factories needed for a WebApp and testing its
// methods.

import (
	"context"
	"github.com/rorycl/reconciler/apiclients/salesforce"
	"github.com/rorycl/reconciler/apiclients/xero"
	"github.com/rorycl/reconciler/config"
	"github.com/rorycl/reconciler/internal/token"
	"log/slog"
	"regexp"
	"time"
)

// xeroClienter is an interface to the capabilities of a xero API client.
type xeroClienter interface {
	GetOrganisation(ctx context.Context) (xero.Organisation, error)
	GetAccounts(ctx context.Context, ifModifiedSince time.Time) ([]xero.Account, error)
	GetBankTransactions(ctx context.Context, fromDate time.Time, ifModifiedSince time.Time, accountsRegexp *regexp.Regexp) ([]xero.BankTransaction, error)
	GetInvoices(ctx context.Context, fromDate time.Time, ifModifiedSince time.Time, accountsRegexp *regexp.Regexp) ([]xero.Invoice, error)
}

// NewXeroClienter is a factory function for returning the default xeroClient as an
// xeroClienter
func newXeroClienter(ctx context.Context, logger *slog.Logger, accountsRegexp *regexp.Regexp, et *token.ExtendedToken) (xeroClienter, error) {
	return xero.NewClient(ctx, logger, accountsRegexp, et)
}

// xeroFactoryFunc is the signature of newXeroClienter.
type xeroFactoryFunc func(ctx context.Context, logger *slog.Logger, accountsRegexp *regexp.Regexp, et *token.ExtendedToken) (xeroClienter, error)

// sfClienter is an interface to the capabilities of a saleforce API client.
type sfClienter interface {
	BatchUpdateOpportunityRefs(ctx context.Context, idRefs []salesforce.IDRef, allOrNone bool) (salesforce.CollectionsUpdateResponse, error)
	GetOpportunities(ctx context.Context, fromDate, ifModifiedSince time.Time) ([]salesforce.Donation, error)
}

// NewSalesforceClienter is a factory function for returning the default sfClient as an
// sfClienter.
func newSalesforceClienter(ctx context.Context, cfg *config.Config, logger *slog.Logger, et *token.ExtendedToken) (sfClienter, error) {
	return salesforce.NewClient(ctx, cfg, logger, et)
}

// sfFactoryFunc is the signature of newXeroClienter.
type sfFactoryFunc func(ctx context.Context, cfg *config.Config, logger *slog.Logger, et *token.ExtendedToken) (sfClienter, error)
