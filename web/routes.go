package web

import (
	"net/http"
	"os"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

// routes connects all of the endpoints and provides middleware.
//
// Notes:
// The /connect endpoint is the entry point to the system, ensuring that the api
// platform connections are made. All data-related endpoints below this section need
// to be protected by the apisOK/web.apisConnectedOK middleware and are protected by the
// `protected` subrouter.
func (web *WebApp) routes() http.Handler {

	r := mux.NewRouter()

	// Mount and publish the static routes.
	fs := http.FileServerFS(web.staticFS)
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", fs))

	// handleApp converts an appHandler to a *mux.Route
	handleApp := func(router *mux.Router, path string, h appHandler) *mux.Route {
		return router.Handle(path, web.ErrorChecker(h))
	}

	/****************************************************************************************
	// public routes
	****************************************************************************************/

	handleApp(r, "/", web.handleRoot()).Methods("GET") // synonym for /connect
	handleApp(r, "/connect", web.handleConnect()).Methods("GET")
	handleApp(r, "/logout", web.handleLogout()).Methods("GET")
	handleApp(r, "/logout/confirmed", web.handleLogoutConfirmed()).Methods("GET")

	// Xero OAuth2 init and callback (the callback route is configured in web.cfg).
	handleApp(r, "/xero/init", web.xeroWebClient.InitiateWebLogin()).Methods("GET")
	handleApp(r, web.cfg.Web.XeroCallBack, web.xeroWebClient.WebLoginCallBack()).Methods("GET")

	// Salesforce OAuth2 init and callback (the callback route is configured in web.cfg).
	handleApp(r, "/salesforce/init", web.sfWebClient.InitiateWebLogin()).Methods("GET")
	handleApp(r, web.cfg.Web.SalesforceCallBack, web.sfWebClient.WebLoginCallBack()).Methods("GET")

	/****************************************************************************************
	// protected routes
	****************************************************************************************/

	// Protected routes require valid connections to have been made.
	protected := r.PathPrefix("").Subrouter()
	protected.Use(web.apisConnectedOK)

	// Refresh is the data refresh page.
	handleApp(protected, "/refresh", web.handleRefresh()).Methods("GET")
	handleApp(protected, "/refresh/update", web.handleRefreshUpdates()).Methods("GET")

	// Main listing pages.
	handleApp(protected, "/home", web.handleHome()).Methods("GET") // redirect to handleInvoices.
	handleApp(protected, "/invoices", web.handleInvoices()).Methods("GET")
	handleApp(protected, "/bank-transactions", web.handleBankTransactions()).Methods("GET")
	handleApp(protected, "/donations", web.handleDonations()).Methods("GET")
	// Todo: consider adding campaigns page

	// Detail pages.
	// Note that the regexp works for uuids and the system test data.
	handleApp(protected, "/invoice/{id:[A-Za-z0-9_-]+}", web.handleInvoiceDetail()).Methods("GET")
	handleApp(protected, "/invoice/{id:[A-Za-z0-9_-]+}/{action:link|unlink}", web.handleInvoiceDetail()).Methods("GET")
	handleApp(protected, "/bank-transaction/{id:[A-Za-z0-9_-]+}", web.handleBankTransactionDetail()).Methods("GET")
	handleApp(protected, "/bank-transaction/{id:[A-Za-z0-9_-]+}/{action:link|unlink}", web.handleBankTransactionDetail()).Methods("GET")

	// Donation linking/unlinking.
	handleApp(protected, "/donations/{type:(?:invoice|bank-transaction)}/{id}/{action}", web.handleDonationsLinkUnlink()).Methods("POST")

	/****************************************************************************************
	// global middleware
	****************************************************************************************/

	// Chain the desired middleware.
	r.Use(handlers.RecoveryHandler(handlers.PrintRecoveryStack(true)))
	logging := handlers.LoggingHandler(os.Stdout, r)
	sessionMiddleWare := web.sessions.LoadAndSave(logging)
	csrfMiddlware := enforceCSRF(sessionMiddleWare)
	return csrfMiddlware
}
