package domain

// types is the interfaces and interface factories needed for abstracting the
// capabilities of the API clients and testing its methods. Functional types and error
// types are kept in reconciler.go

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/rorycl/reconciler/apiclients/salesforce"
	"github.com/rorycl/reconciler/apiclients/xero"
)

// XeroClient is an interface to the capabilities of a xero API client.
type XeroClient interface {
	GetOrganisation(ctx context.Context) (xero.Organisation, error)
	GetAccounts(ctx context.Context, ifModifiedSince time.Time) ([]xero.Account, error)
	GetBankTransactions(ctx context.Context, fromDate time.Time, ifModifiedSince time.Time, accountsRegexp *regexp.Regexp) ([]xero.BankTransaction, error)
	GetInvoices(ctx context.Context, fromDate time.Time, ifModifiedSince time.Time, accountsRegexp *regexp.Regexp) ([]xero.Invoice, error)
}

// SalesforceClient is an interface to the capabilities of a saleforce API client.
type SalesforceClient interface {
	BatchUpdateOpportunityRefs(ctx context.Context, idRefs []salesforce.IDRef, allOrNone bool) (salesforce.CollectionsUpdateResponse, error)
	GetOpportunities(ctx context.Context, fromDate, ifModifiedSince time.Time) ([]salesforce.Donation, error)
}

// ErrUsage is an error in usage
type ErrUsage struct {
	Detail string
	Msg    string // user facing message
}

func (e ErrUsage) Error() string {
	return fmt.Sprintf("%s: %s", e.Detail, e.Msg)
}

// ErrSystem is a system error, potentially recording a domain logic issue or an
// infrastructure problem such as an interrupted network or external API error.
type ErrSystem struct {
	Detail string
	Err    error
	Msg    string // user facing message
}

func (e ErrSystem) Error() string {
	return fmt.Sprintf("%s: %s: %v", e.Detail, e.Msg, e.Err)
}
