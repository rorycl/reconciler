package web

// types is the interfaces and interface factories needed for a WebApp and testing its
// methods.

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/rorycl/reconciler/apiclients/salesforce"
	"github.com/rorycl/reconciler/apiclients/xero"
	"github.com/rorycl/reconciler/config"
	"github.com/rorycl/reconciler/domain"
	"github.com/rorycl/reconciler/internal/token"
)

// NewXeroClienter is a factory function for returning the default xeroClient as an
// xeroClienter
func newXeroClientMaker(ctx context.Context, logger *slog.Logger, accountsRegexp *regexp.Regexp, et *token.ExtendedToken) (domain.XeroClient, error) {
	return xero.NewClient(ctx, logger, accountsRegexp, et)
}

// xeroFactoryFunc is the signature of newXeroClienter.
type xeroClientMaker func(ctx context.Context, logger *slog.Logger, accountsRegexp *regexp.Regexp, et *token.ExtendedToken) (domain.XeroClient, error)

// NewSalesforceClienter is a factory function for returning the default sfClient as an
// sfClientMaker.
func newSalesforceClientMaker(ctx context.Context, cfg *config.Config, logger *slog.Logger, et *token.ExtendedToken) (domain.SalesforceClient, error) {
	return salesforce.NewClient(ctx, cfg, logger, et)
}

// sfFactoryFunc is the signature of newXeroClienter.
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
