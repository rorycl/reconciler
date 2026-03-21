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
// to be protected by the apisOK/web.apisConnectedOK middleware.
func (web *WebApp) routes() http.Handler {

	r := mux.NewRouter()

	fs := http.FileServerFS(web.staticFS)
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", fs))

	// apisConnectedOk is a middleware ensuring certain endpoints are protected by a
	// func that checks that both Salesforce and Xero apis are connected ok. 'connOK' is
	// a shortcut.
	apisOK := web.apisConnectedOK

	// Note that the OAuth2 handlers are in the apiclient modules.
	r.Handle("/", web.ErrorChecker(web.handleRoot())).Methods("GET") // synonym for /connect
	r.Handle("/connect", web.ErrorChecker(web.handleConnect())).Methods("GET")
	r.Handle("/logout", web.ErrorChecker(web.handleLogout())).Methods("GET")
	r.Handle("/logout/confirmed", web.ErrorChecker(web.handleLogoutConfirmed())).Methods("GET")

	// Xero OAuth2 init and callback.
	r.Handle("/xero/init", web.ErrorChecker(web.xeroWebClient.InitiateWebLogin())).Methods("GET")
	r.Handle(web.cfg.Web.XeroCallBack, web.ErrorChecker(web.xeroWebClient.WebLoginCallBack())).Methods("GET")

	// Salesforce OAuth2 init and callback.
	r.Handle("/salesforce/init", web.ErrorChecker(web.sfWebClient.InitiateWebLogin())).Methods("GET")
	r.Handle(web.cfg.Web.SalesforceCallBack, web.ErrorChecker(web.sfWebClient.WebLoginCallBack())).Methods("GET")

	// Refresh is the data refresh page.
	r.Handle("/refresh", apisOK(web.ErrorChecker(web.handleRefresh()))).Methods("GET")
	r.Handle("/refresh/update", apisOK(web.ErrorChecker(web.handleRefreshUpdates()))).Methods("GET")

	// Main listing pages.
	r.Handle("/home", apisOK(web.ErrorChecker(web.handleHome()))).Methods("GET") // redirect to handleInvoices.
	r.Handle("/invoices", apisOK(web.ErrorChecker(web.handleInvoices()))).Methods("GET")
	r.Handle("/bank-transactions", apisOK(web.ErrorChecker(web.handleBankTransactions()))).Methods("GET")
	r.Handle("/donations", apisOK(web.ErrorChecker(web.handleDonations()))).Methods("GET")
	// Todo: consider adding campaigns page

	// Detail pages.
	// Note that the regexp works for uuids and the system test data.
	r.Handle("/invoice/{id:[A-Za-z0-9_-]+}", apisOK(web.ErrorChecker(web.handleInvoiceDetail()))).Methods("GET")
	r.Handle("/invoice/{id:[A-Za-z0-9_-]+}/{action:link|unlink}", apisOK(web.ErrorChecker(web.handleInvoiceDetail()))).Methods("GET")
	r.Handle("/bank-transaction/{id:[A-Za-z0-9_-]+}", apisOK(web.ErrorChecker(web.handleBankTransactionDetail()))).Methods("GET")
	r.Handle("/bank-transaction/{id:[A-Za-z0-9_-]+}/{action:link|unlink}", apisOK(web.ErrorChecker(web.handleBankTransactionDetail()))).Methods("GET")

	// Donation linking/unlinking.
	r.Handle("/donations/{type:(?:invoice|bank-transaction)}/{id}/{action}", apisOK(web.ErrorChecker(web.handleDonationsLinkUnlink()))).Methods("POST")

	// Todo: logout
	// Logout -- delete the api connection tokens and redirect to /connect

	// Chain the desired middleware. Todo: add recover handler.
	logging := handlers.LoggingHandler(os.Stdout, r)
	sessionMiddleWare := web.sessions.LoadAndSave(logging)
	csrfMiddlware := enforceCSRF(sessionMiddleWare)
	return csrfMiddlware
}
