package web

// This file describes the web server for the Reconciler project.
//
// Note that modules called by this server should provide self-describing errors since
// these are sent directly to an internal server error func:
//
//	web.ServerError(w, r, err)
//
// This web server also sets out each endpoint handler as a HandlerFunc. This allows for
// the router to provide arguments to the handler, as discussed in Mat Ryer's post at
//
//	https://grafana.com/blog/how-i-write-http-services-in-go-after-13-years/
//
// Another use of this pattern is to initialise only the templates needed for a specific
// endpoint. This allows for endpoint-specific template error catching, and potential
// use-case specific overriding of template `block` components, if required.
//
// Helper functions, such as `ServerError` and `clientError` are at the end of the file.
//
// HTMX partials are used in this project to avoid duplicating content and to allow for
// in-place updates of page components rather than requiring full page refreshes. These
// are:
//
// templates/partial-donations.html
//	- load the donation tabs, searchform and searchresults
// templates/partial-donations-linked.html
//	- linked donations listing
// templates/partial-donations-searchform.html
//	- donations search form
// templates/partial-donations-searchresults.html
//	- donations search results
// templates/partial-donations-tabs.html
//	- donations tabs (linked and search)
// templates/partial-listingTabs.html
//	- donations tab headers

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/gob"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"reconciler/apiclients/salesforce"
	"reconciler/config"
	"reconciler/db"
	"reconciler/internal/token"

	"github.com/alexedwards/scs/v2"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"golang.org/x/oauth2"
)

// pageLen is the number of items to show in a page listing.
const pageLen = 15

// missingTransactionReference indicates a missing transaction reference, which should
// not be used for linking.
const missingTransactionReference = "missing reference"

//go:embed static
var StaticEmbeddedFS embed.FS

//go:embed templates
var TemplatesEmbeddedFS embed.FS

// refreshDurationWindow is the duration window given for a remote platform to update
// it's records.
var refreshDurationWindow = time.Duration(1000000000 * 15) // 15s

// refreshTrucation is the user-oriented refresh rounding duration.
var refreshTruncation = time.Duration(1000000000 * 10) // 10s

// WebApp is the configuration object for the web server.
type WebApp struct {
	log            *slog.Logger
	cfg            *config.Config
	db             *db.DB
	staticFS       fs.FS // the fs holding the static web resources.
	templateFS     fs.FS // the fs holding the web templates.
	server         *http.Server
	sessions       *scs.SessionManager
	accountsRegexp *regexp.Regexp

	// web clients for oauth2
	xeroWebClient *token.TokenWebClient
	sfWebClient   *token.TokenWebClient

	// in development mode
	inDevelopment bool
}

// New initialises a WebApp. An error type is returned for future use.
func New(
	logger *slog.Logger,
	cfg *config.Config,
	db *db.DB,
	staticFS fs.FS,
	templateFS fs.FS,
) (*WebApp, error) {

	// Add settings for the http server.
	// The timeout settings are intended to allow the API data refresh handler to run
	// without interruption.
	server := &http.Server{
		Addr:              cfg.Web.ListenAddress,
		ReadHeaderTimeout: time.Duration(300 * time.Second),
		WriteTimeout:      time.Duration(300 * time.Second),
		MaxHeaderBytes:    1 << 17, // 125k ish
	}

	// Initialise in-memory session store and related custom gob types.
	// Sessions have an absolute validity limit of 8 hours.
	// Sessions time out after 2 hours of inactivity.
	scsSessionStore := scs.New()
	scsSessionStore.Lifetime = 8 * time.Hour
	scsSessionStore.IdleTimeout = 2 * time.Hour

	gob.Register(time.Time{})
	gob.Register(token.ExtendedToken{})

	// Compile the donation accounts filtering regexp
	// (this is a safe assignment, checked at config ingestion).
	accountsRegexp := cfg.DonationAccountCodesAsRegex()

	webApp := &WebApp{
		log:            logger,
		cfg:            cfg,
		db:             db,
		staticFS:       staticFS,
		templateFS:     templateFS,
		server:         server,
		sessions:       scsSessionStore,
		accountsRegexp: accountsRegexp,
	}

	// Attach the salesforce and xero OAuth2 web client handler constructors.
	sfWebClient, err := token.NewTokenWebClient(
		token.SalesforceToken,
		cfg.Salesforce.OAuth2Config,
		scsSessionStore,
		webApp,     // implements ServerError
		"/connect", // the redirect url
	)
	if err != nil {
		return nil, fmt.Errorf("could not make salesforce web oauth2 client: %v", err)
	}
	webApp.sfWebClient = sfWebClient
	xeroWebClient, err := token.NewTokenWebClient(
		token.XeroToken,
		cfg.Xero.OAuth2Config,
		scsSessionStore,
		webApp,     // implements ServerError
		"/connect", // the redirect url
	)
	if err != nil {
		return nil, fmt.Errorf("could not make xero web oauth2 client: %v", err)
	}
	webApp.xeroWebClient = xeroWebClient

	// In-Development mode triggers a warning as it bypasses the API connection check
	// middleware.
	if cfg.InDevelopmentMode {
		webApp.inDevelopment = true
		webApp.log.Warn("******************************************")
		webApp.log.Warn("       Warning: IN DEVELOPMENT mode       ")
		webApp.log.Warn("    ** API connection check disabled **   ")
		webApp.log.Warn("       Warning: IN DEVELOPMENT mode       ")
		webApp.log.Warn("******************************************")
	}

	return webApp, nil
}

// RestartRoutes reruns the route setup. This should only be used in development mode as
// it may panic.
func (web *WebApp) RestartRoutes() {
	web.server.Handler = web.routes()
}

// StartServer starts a WebApp.
func (web *WebApp) StartServer() error {
	web.server.Handler = web.routes()
	// Print to the console, regardless of the log level.
	fmt.Printf("Starting server on %s\n", web.cfg.Web.ListenAddress)
	return web.server.ListenAndServe()
}

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
	r.Handle("/", web.handleRoot()).Methods("GET") // synonym for /connect
	r.Handle("/connect", web.handleConnect()).Methods("GET")
	r.Handle("/logout", web.handleLogout()).Methods("GET")
	r.Handle("/logout/confirmed", web.handleLogoutConfirmed()).Methods("GET")

	// Xero OAuth2 init and callback.
	r.Handle("/xero/init", web.xeroWebClient.InitiateWebLogin()).Methods("GET")
	r.Handle(web.cfg.Web.XeroCallBack, web.xeroWebClient.WebLoginCallBack()).Methods("GET")

	// Salesforce OAuth2 init and callback.
	r.Handle("/salesforce/init", web.sfWebClient.InitiateWebLogin()).Methods("GET")
	r.Handle(web.cfg.Web.SalesforceCallBack, web.sfWebClient.WebLoginCallBack()).Methods("GET")

	// Refresh is the data refresh page.
	r.Handle("/refresh", apisOK(web.handleRefresh())).Methods("GET")
	r.Handle("/refresh/update", apisOK(web.handleRefreshUpdates())).Methods("GET")

	// Main listing pages.
	r.Handle("/home", apisOK(web.handleHome())).Methods("GET") // redirect to handleInvoices.
	r.Handle("/invoices", apisOK(web.handleInvoices())).Methods("GET")
	r.Handle("/bank-transactions", apisOK(web.handleBankTransactions())).Methods("GET")
	r.Handle("/donations", apisOK(web.handleDonations())).Methods("GET")
	// Todo: consider adding campaigns page

	// Detail pages.
	// Note that the regexp works for uuids and the system test data.
	r.Handle("/invoice/{id:[A-Za-z0-9_-]+}", apisOK(web.handleInvoiceDetail())).Methods("GET")
	r.Handle("/invoice/{id:[A-Za-z0-9_-]+}/{action:link|unlink}", apisOK(web.handleInvoiceDetail())).Methods("GET")
	r.Handle("/bank-transaction/{id:[A-Za-z0-9_-]+}", apisOK(web.handleBankTransactionDetail())).Methods("GET")
	r.Handle("/bank-transaction/{id:[A-Za-z0-9_-]+}/{action:link|unlink}", apisOK(web.handleBankTransactionDetail())).Methods("GET")

	// Donation linking/unlinking.
	r.Handle("/donations/{type:(?:invoice|bank-transaction)}/{id}/{action}", apisOK(web.handleDonationsLinkUnlink())).Methods("POST")

	// Todo: logout
	// Logout -- delete the api connection tokens and redirect to /connect

	// Chain the desired middleware. Todo: add recover handler.
	logging := handlers.LoggingHandler(os.Stdout, r)
	sessionMiddleWare := web.sessions.LoadAndSave(logging)
	csrfMiddlware := enforceCSRF(sessionMiddleWare)
	return csrfMiddlware
}

// apisConnectedOK checks whether the user is connected to the API services as
// represented by having a valid token. If any service is not connected, the user is
// redirected to the /connect endpoint.
//
// Note that in development mode this handler makes no checks.
func (web *WebApp) apisConnectedOK(next http.Handler) http.Handler {

	if web.inDevelopment {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if _, err := web.getValidTokenFromSession(ctx, token.XeroToken); err != nil {
			web.log.Info("xero token is not valid, redirecting")
			http.Redirect(w, r, "/connect?status=xero_token_invalid", http.StatusSeeOther)
			return
		}
		if _, err := web.getValidTokenFromSession(ctx, token.SalesforceToken); err != nil {
			web.log.Info("saleforce token is not valid, redirecting")
			http.Redirect(w, r, "/connect?status=sf_token_invalid", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleRoot deals with http calls to "/" by redirecting to "/connect".
func (web *WebApp) handleRoot() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/connect", http.StatusFound)
	})
}

// handleConnect serves the /connect endpoint.
func (web *WebApp) handleConnect() http.Handler {

	name := "connect.html"
	tpls := []string{"base.html", "connect.html"}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()

		var xeroTokenValid, sfTokenValid bool
		_, err := web.getValidTokenFromSession(ctx, token.XeroToken)
		if err == nil {
			xeroTokenValid = true
		}
		sfTok, err := web.getValidTokenFromSession(ctx, token.SalesforceToken)
		if err == nil {
			sfTokenValid = true
		}
		if sfTokenValid {
			web.sessions.Put(ctx, "salesforce-instance-url", sfTok.InstanceURL)
		}

		data := map[string]any{
			"Organisation":     web.cfg.Organisation,
			"XeroTokenIsValid": xeroTokenValid,
			"SFTokenIsValid":   sfTokenValid,
		}
		web.render(w, r, templates, name, data)
	})
}

// handleLogout serves the /logout endpoint.
func (web *WebApp) handleLogout() http.Handler {

	name := "logout.html"
	tpls := []string{"base.html", "logout.html"}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))

	// Determine if an in-memory database is in use.
	hasMemoryDB := strings.Contains(web.cfg.DatabasePath, ":memory:")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := map[string]any{
			"MemoryDatabase": hasMemoryDB,
			"DBName":         web.cfg.DatabasePath,
		}
		web.render(w, r, templates, name, data)
	})
}

// handleLogoutConfirmed serves the /logout/confirmed endpoint, which clears the session
// and redirects to /connect.
func (web *WebApp) handleLogoutConfirmed() http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()

		// Clear the session.
		err := web.sessions.Clear(ctx)
		if err != nil {
			web.log.Error(fmt.Sprintf("Sesssion clear error: %v", err))
		} else {
			web.log.Info("Session cleared")
		}

		// Close the database and kill the session.
		_ = web.db.Close()
		time.Sleep(1 * time.Second)
		web.log.Info("Logout completed")
		if fl, ok := w.(http.Flusher); ok {
			_, _ = fmt.Fprint(w, "Logout completed. Please restart the program.")
			fl.Flush()
		}

		time.Sleep(1 * time.Second)
		os.Exit(0)
	})
}

// handleRefresh serves the /refresh page.
func (web *WebApp) handleRefresh() http.Handler {

	name := "refresh.html"
	tpls := []string{"base.html", "refresh.html"}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))

	// Configuration start date.
	dataStartDate := web.cfg.DataStartDate
	accountCodes := web.cfg.DonationAccountPrefixes

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Todo: Refresh data is best determined by database data freshness.
		var refreshed bool
		var lastRefresh time.Time
		xeroLastRefresh := web.sessions.GetTime(ctx, "xero-refreshed-datetime")
		sfLastRefresh := web.sessions.GetTime(ctx, "sf-refreshed-datetime")
		if !xeroLastRefresh.IsZero() && !sfLastRefresh.IsZero() {
			refreshed = true
			if xeroLastRefresh.Before(sfLastRefresh) {
				lastRefresh = xeroLastRefresh
			} else {
				lastRefresh = sfLastRefresh
			}
		}

		data := map[string]any{
			"DataStartDate":        dataStartDate,
			"Refreshed":            refreshed,
			"LastRefresh":          lastRefresh,
			"DonationAccountCodes": accountCodes,
		}
		web.render(w, r, templates, name, data)
	})
}

// handleRefreshUpdates serves the htmx partial /refresh/update info for refreshing data
// from the api platforms into the database. Note that Linking/Unlinking donations also
// calls sfClient.GetOpportunities in a window of 2 minutes.
//
// Todo: consider splitting Xero and Salesforce update timestamps, and consider making
// the time windows smaller.
func (web *WebApp) handleRefreshUpdates() http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()

		// Retrieve and upsert the Xero records.
		infoMap, err := web.refreshXeroRecords(ctx)
		if err != nil {
			// Todo: report errors to client.
			web.log.Error(fmt.Sprintf("failed to refresh Xero records: %v", err))
			http.Redirect(w, r, "/refresh", http.StatusFound)
			return
		}

		// Set the xero shortcode in the session if applicable.
		if shortCode, ok := infoMap["xero-shortcode"]; ok {
			web.sessions.Put(ctx, "xero-shortcode", shortCode)
		}

		// Retrieve and upsert the Salesforce records.
		err = web.refreshSalesforceRecords(ctx)
		if err != nil {
			// Todo: report errors to client.
			web.log.Error(fmt.Sprintf("failed to refresh Salesforce records: %v", err))
			http.Redirect(w, r, "/refresh", http.StatusFound)
			return
		}

		web.log.Info("Refresh successfully completed.")

		// Redirect to invoices
		w.Header().Set("HX-Redirect", "/invoices")
		w.WriteHeader(http.StatusOK)

	})
}

// handleHome redirects from /home.
func (web *WebApp) handleHome() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/invoices", http.StatusFound)
	})
}

// handleInvoices serves the /invoices list.
func (web *WebApp) handleInvoices() http.Handler {

	thisURL := "/invoices"
	name := "invoices.html"
	tpls := []string{"base.html", "nav.html", "partial-listingTabs.html", "invoices.html"}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))
	dataStartDate := web.cfg.DataStartDate

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()

		// Initialise url parameter form and derive url.
		form := NewSearchForm(&web.cfg.DataStartDate, nil)
		if err := DecodeURLParams(r, form); err != nil {
			web.ServerError(w, r, err)
			return
		}
		urlParams, err := form.AsURLParams()
		if err != nil {
			web.ServerError(w, r, err)
		}
		redirectURL := thisURL + "?" + urlParams

		// If the url is 'reset', clear the last saved url and redirect to default.
		if r.URL.Query().Get("reset") == "true" {
			_ = web.sessions.PopString(ctx, thisURL)
			http.Redirect(w, r, redirectURL, http.StatusSeeOther)
			return
		}

		// If the url is 'naked', redirect to the last saved url or default.
		if r.URL.RawQuery == "" {
			if savedURL := web.sessions.GetString(ctx, thisURL); savedURL != "" {
				http.Redirect(w, r, savedURL, http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, redirectURL, http.StatusSeeOther)
			return
		}

		// Create a validator and validate the form.
		validator := NewValidator()
		form.Validate(validator)

		// Determine the last refresh time of Xero data. Note this can be
		// time.Time.IsZero(), but it is unlikely since the user has already run a
		// refresh to get to this handler.
		lastRefresh := web.sessions.GetTime(ctx, "xero-refreshed-datetime")
		lastRefreshed := time.Since(lastRefresh).Truncate(refreshTruncation)

		// If refresh is called and form is valid, do a xero data refresh then redirect
		// back to the current page.
		if validator.Valid() && form.Refresh {

			_, err = web.refreshXeroRecords(ctx)
			if err != nil {
				web.log.Error(fmt.Sprintf("list invoices: refresh data error: %v", err))
			}
			http.Redirect(w, r, redirectURL, http.StatusSeeOther)
			return
		}

		// Initialise pagination for default state.
		pagination, _ := NewPagination(pageLen, 1, form.Page, r.URL.Query())

		// Prepare data for the template, allowing passing of validation
		// errors back to the template if necessary.
		data := struct {
			PageTitle     string
			Invoices      []db.Invoice
			Form          *SearchForm
			Validator     *Validator
			Pagination    *Pagination
			CurrentPage   string
			ShortCode     string
			DataStartDate time.Time
			LastRefreshed time.Duration
		}{
			PageTitle:     "Invoices",
			Form:          form,
			Validator:     validator,
			Pagination:    pagination,
			CurrentPage:   "invoices",
			ShortCode:     web.sessions.GetString(ctx, "xero-shortcode"),
			DataStartDate: dataStartDate,
			LastRefreshed: lastRefreshed,
		}

		// Render template with errors and return if the form is invalid.
		if !validator.Valid() {
			web.render(w, r, templates, name, data)
			return
		}

		invoices, err := web.db.InvoicesGet(
			ctx,
			form.ReconciliationStatus,
			form.DateFrom,
			form.DateTo,
			form.SearchString,
			pageLen,
			form.Offset(pageLen),
		)
		if err != nil && err != sql.ErrNoRows {
			web.ServerError(w, r, err)
			return
		}

		// Set valid data from successful database call.
		data.Invoices = invoices

		// Set pagination for number of invoices. In case of an error, log
		// and continue. Each invoice has the search query row count as a
		// field.
		var recordsNo int
		if len(data.Invoices) == 0 {
			recordsNo = 1
		} else {
			recordsNo = data.Invoices[0].RowCount
		}
		data.Pagination, err = NewPagination(pageLen, recordsNo, form.Page, r.URL.Query())
		if err != nil {
			web.ServerError(w, r, err)
		}

		// Save the url.
		web.sessions.Put(ctx, thisURL, redirectURL)

		web.render(w, r, templates, name, data)
	})
}

// handleBankTransactions serves the /bank-transactions bank transactions list.
func (web *WebApp) handleBankTransactions() http.Handler {

	thisURL := "/bank-transactions"
	name := "bank-transactions.html"
	tpls := []string{"base.html", "nav.html", "partial-listingTabs.html", "bank-transactions.html"}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))
	dataStartDate := web.cfg.DataStartDate

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()

		// Initialise url parameter form and derive url.
		form := NewSearchForm(&web.cfg.DataStartDate, nil)
		if err := DecodeURLParams(r, form); err != nil {
			web.ServerError(w, r, err)
			return
		}
		urlParams, err := form.AsURLParams()
		if err != nil {
			web.ServerError(w, r, err)
		}
		redirectURL := thisURL + "?" + urlParams

		// If the url is 'reset', clear the last saved url and redirect to default.
		if r.URL.Query().Get("reset") == "true" {
			_ = web.sessions.PopString(ctx, thisURL)
			http.Redirect(w, r, redirectURL, http.StatusSeeOther)
			return
		}

		// If the url is 'naked', redirect to the default.
		if r.URL.RawQuery == "" {
			if savedURL := web.sessions.GetString(ctx, thisURL); savedURL != "" {
				http.Redirect(w, r, savedURL, http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, redirectURL, http.StatusSeeOther)
			return
		}

		// Create a validator and validate the form.
		validator := NewValidator()
		form.Validate(validator)

		// Determine the last refresh time of Xero data. Note this can be
		// time.Time.IsZero(), but it is unlikely since the user has already run a
		// refresh to get to this handler.
		lastRefresh := web.sessions.GetTime(ctx, "xero-refreshed-datetime")
		lastRefreshed := time.Since(lastRefresh).Truncate(refreshTruncation)

		// If refresh is called and form is valid, do a xero data refresh then redirect
		// back to the current page.
		if validator.Valid() && form.Refresh {

			_, err = web.refreshXeroRecords(ctx)
			if err != nil {
				web.log.Error(fmt.Sprintf("list bank-transactions: refresh data error: %v", err))
			}
			http.Redirect(w, r, redirectURL, http.StatusSeeOther)
			return
		}

		// Initialise pagination for default state.
		pagination, _ := NewPagination(pageLen, 1, form.Page, r.URL.Query())

		// Prepare data for the template, allowing passing of validation
		// errors back to the template if necessary.
		data := struct {
			PageTitle        string
			BankTransactions []db.BankTransaction
			Form             *SearchForm
			Validator        *Validator
			Pagination       *Pagination
			CurrentPage      string
			DataStartDate    time.Time
			LastRefreshed    time.Duration
		}{
			PageTitle:     "Bank Transactions",
			Form:          form,
			Validator:     validator,
			Pagination:    pagination,
			CurrentPage:   "bank-transactions",
			DataStartDate: dataStartDate,
			LastRefreshed: lastRefreshed,
		}

		// Render template with errors and return if the form is invalid.
		if !validator.Valid() {
			web.render(w, r, templates, name, data)
			return
		}

		transactions, err := web.db.BankTransactionsGet(
			ctx,
			form.ReconciliationStatus,
			form.DateFrom,
			form.DateTo,
			form.SearchString,
			pageLen,
			form.Offset(pageLen),
		)
		if err != nil && err != sql.ErrNoRows {
			web.ServerError(w, r, err)
			return
		}

		// Set valid data from successful database call.
		data.BankTransactions = transactions

		// Set pagination for number of transactions. In case of an error, log
		// and continue. Each transaction has the search query row count as a
		// field.
		var recordsNo int
		if len(data.BankTransactions) == 0 {
			recordsNo = 1
		} else {
			recordsNo = data.BankTransactions[0].RowCount
		}
		data.Pagination, err = NewPagination(pageLen, recordsNo, form.Page, r.URL.Query())
		if err != nil {
			web.ServerError(w, r, err)
		}

		// Save the url.
		web.sessions.Put(ctx, thisURL, redirectURL)

		web.render(w, r, templates, name, data)
	})
}

// handleDonations serves the /donations list of donations.
func (web *WebApp) handleDonations() http.Handler {

	thisURL := "/donations"
	name := "donations.html"
	tpls := []string{"base.html", "nav.html", "partial-listingTabs.html", "partial-donations-searchform.html", "partial-donations-searchresults.html", "donations.html"}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))
	dataStartDate := web.cfg.DataStartDate

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()

		// Initialise url parameter form and derive url.
		form := NewSearchDonationsForm(&web.cfg.DataStartDate, nil)
		if err := DecodeURLParams(r, form); err != nil {
			web.ServerError(w, r, err)
			return
		}
		urlParams, err := form.AsURLParams()
		if err != nil {
			web.ServerError(w, r, err)
		}
		redirectURL := thisURL + "?" + urlParams

		// If the url is 'reset', clear the last saved url and redirect to default.
		if r.URL.Query().Get("reset") == "true" {
			_ = web.sessions.PopString(ctx, thisURL)
			http.Redirect(w, r, redirectURL, http.StatusSeeOther)
			return
		}

		// If the url is 'naked', redirect to the default.
		if r.URL.RawQuery == "" {
			if savedURL := web.sessions.GetString(ctx, thisURL); savedURL != "" {
				http.Redirect(w, r, savedURL, http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, redirectURL, http.StatusSeeOther)
			return
		}

		// Create a validator and validate the form.
		validator := NewValidator()
		form.Validate(validator)

		// Determine the last refresh time of Salesforce data. Note this can be
		// time.Time.IsZero(), but it is unlikely since the user has already run a
		// refresh to get to this handler.
		lastRefresh := web.sessions.GetTime(ctx, "sf-refreshed-datetime")
		lastRefreshed := time.Since(lastRefresh).Truncate(refreshTruncation)

		// If refresh is called and form is valid, do a salesforce data refresh then
		// redirect back to the current page.
		if validator.Valid() && form.Refresh {

			err = web.refreshSalesforceRecords(ctx)
			if err != nil {
				web.log.Error(fmt.Sprintf("list donations: refresh data error: %v", err))
			}
			http.Redirect(w, r, redirectURL, http.StatusSeeOther)
			return
		}

		// Initialise pagination for default state.
		pagination, _ := NewPagination(pageLen, 1, form.Page, r.URL.Query())

		// Get salesforce instance url from the session (via OAuth2 token)
		instanceURL := web.sessions.GetString(ctx, "salesforce-instance-url")

		// Prepare data for the template, allowing passing of validation
		// errors back to the template if necessary.
		data := struct {
			PageTitle     string
			ViewDonations []viewDonation
			Form          *SearchDonationsForm
			ID            string // needed to match the invoice/bank transaction struct
			Typer         string
			Validator     *Validator
			Pagination    *Pagination
			CurrentPage   string
			GetURL        string
			SFInstanceURL string
			DataStartDate time.Time
			LastRefreshed time.Duration
		}{
			PageTitle:     "Donations",
			Form:          form,
			ID:            "", // no data needed
			Typer:         "donations",
			Validator:     validator,
			Pagination:    pagination,
			CurrentPage:   "donations",
			GetURL:        "/donations",
			SFInstanceURL: instanceURL,
			DataStartDate: dataStartDate,
			LastRefreshed: lastRefreshed,
		}

		// Render template with errors and return if the form is invalid.
		if !validator.Valid() {
			web.render(w, r, templates, name, data)
			return
		}

		donations, err := web.db.DonationsGet(
			ctx,
			form.DateFrom,
			form.DateTo,
			form.LinkageStatus,
			form.PayoutReference,
			form.SearchString,
			pageLen,
			form.Offset(pageLen),
		)
		if err != nil && err != sql.ErrNoRows {
			web.ServerError(w, r, err)
			return
		}

		// Process donations into donationView type
		viewDonations := newViewDonations(donations)

		// Set valid data from successful database call.
		data.ViewDonations = viewDonations

		// Set pagination for number of donations. In case of an error, log
		// and continue. Each donation has the search query row count as a
		// field.
		var recordsNo int
		if len(data.ViewDonations) == 0 {
			recordsNo = 1
		} else {
			recordsNo = data.ViewDonations[0].RowCount
		}
		data.Pagination, err = NewPagination(pageLen, recordsNo, form.Page, r.URL.Query())
		if err != nil {
			web.log.Error(fmt.Sprintf("pagination error: %v", err))
		}

		// Save the url.
		web.sessions.Put(ctx, thisURL, redirectURL)

		web.render(w, r, templates, name, data)
	})
}

// handleInvoiceDetail serves the detail page at /invoice/<id> for a single invoice.
// Note that the func can also be invoked at `/invoice/<id>/<action>(link|unlink)`. The
// former redirect to the latter's 'link' form by default.
func (web *WebApp) handleInvoiceDetail() http.Handler {

	name := "invoice.html"
	tpls := []string{
		"base.html",
		"nav.html",
		"partial-listingTabs.html",
		"partial-donations-tabs.html",
		"partial-donations-linked.html",
		"partial-donations-searchform.html",
		"partial-donations-searchresults.html",
		"invoice.html",
	}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()

		// Extract route parameters.
		vars, err := validMuxVars(mux.Vars(r), "id")
		if err != nil {
			web.clientError(w, err.Error(), http.StatusBadRequest)
			return
		}
		invoiceID := vars["id"]

		// The 'action' route parameter is optional, but this handler reroutes to an
		// "action" version of the url if it is empty.
		action := "link"
		if a, ok := mux.Vars(r)["action"]; ok {
			action = a
		}
		baseURL := fmt.Sprintf("/invoice/%s/%s", invoiceID, action)

		// Get the invoice details.
		var invoice db.WRInvoice
		var lineItems []db.WRLineItem
		invoice, lineItems, err = web.db.InvoiceWRGet(ctx, invoiceID)
		if err != nil && err != sql.ErrNoRows {
			web.ServerError(w, r, err)
			return
		}
		if err == sql.ErrNoRows {
			web.notFound(w, r, fmt.Sprintf("invoice %q not found", invoiceID))
			return
		}

		// Process the line items.
		viewLineItems := newViewLineItems(lineItems)

		// Determine the dates for retrieving donations.
		startDate, endDate := donationSearchTimeSpan(invoice.Date)

		// Initialise url parameter form and derive default url.
		form := NewSearchDonationsForm(&startDate, &endDate)

		// Derive the default url.
		urlParams, err := form.AsURLParams()
		if err != nil { // unlikely
			web.log.Error(fmt.Sprintf("handleInvoiceDetail: default url error: %v", err))
			web.ServerError(w, r, err)
		}
		defaultURL := baseURL + "?" + urlParams

		// If the url has no 'action' path param or has the 'reset' query param, clear
		// the last saved url and redirect to default.
		if r.URL.Query().Get("reset") == "true" {
			_ = web.sessions.PopString(ctx, baseURL)
			http.Redirect(w, r, defaultURL, http.StatusSeeOther)
			return
		}

		// If the url is 'naked', redirect to either the saved or default.
		if r.URL.RawQuery == "" {
			if savedURL := web.sessions.GetString(ctx, baseURL); savedURL != "" {
				http.Redirect(w, r, savedURL, http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, defaultURL, http.StatusSeeOther)
			return
		}

		// Decode the url params and construct the current url, interjecting if the
		// action is "unlink" to get the related information.
		if err := DecodeURLParams(r, form); err != nil {
			web.ServerError(w, r, err)
		}
		if action == "unlink" {
			form.DateFrom = web.cfg.DataStartDate
			form.DateTo = time.Now().AddDate(1, 0, 0)
			form.LinkageStatus = "Linked"
			form.PayoutReference = invoice.InvoiceNumber
		}
		urlParams, err = form.AsURLParams()
		if err != nil {
			web.ServerError(w, r, err)
		}
		thisURL := baseURL + "?" + urlParams

		// Save the url.
		web.sessions.Put(ctx, baseURL, thisURL)

		// Create a validator and validate the form.
		validator := NewValidator()
		form.Validate(validator)

		// Get the donations if the form is valid
		var donations []db.Donation
		if validator.Valid() {
			donations, err = web.db.DonationsGet(
				ctx,
				form.DateFrom,
				form.DateTo,
				form.LinkageStatus,
				form.PayoutReference,
				form.SearchString,
				pageLen,
				form.Offset(pageLen),
			)
			if err != nil && err != sql.ErrNoRows {
				web.ServerError(w, r, err)
				return
			}
		}

		// Process donations into donationView type
		viewDonations := newViewDonations(donations)

		// Set pagination for number of donations. In case of an error, log
		// and continue. Each donation has the search query row count as a
		// field.
		var recordsNo int
		if len(viewDonations) == 0 {
			recordsNo = 1
		} else {
			recordsNo = viewDonations[0].RowCount
		}

		// Todo: fix page number (here 1)
		pagination, err := NewPagination(pageLen, recordsNo, form.Page, r.URL.Query())
		if err != nil {
			web.log.Error(fmt.Sprintf("pagination error: %v", err))
		}

		// Prepare data for the template.
		data := struct {
			PageTitle     string
			Invoice       db.WRInvoice
			LineItems     []viewLineItem
			ID            string
			DFK           string // for Invoices, this is the Invoice Number
			Typer         string
			ShortCode     string
			CurrentPage   string
			TabFocus      string
			SFInstanceURL string

			// Donation data
			ViewDonations []viewDonation
			Form          *SearchDonationsForm
			Validator     *Validator
			Pagination    *Pagination
		}{
			PageTitle:     fmt.Sprintf("Invoice %s", invoiceID),
			Invoice:       invoice,
			LineItems:     viewLineItems,
			ID:            invoice.ID,
			DFK:           invoice.InvoiceNumber,
			Typer:         "invoice",
			ShortCode:     web.sessions.GetString(ctx, "xero-shortcode"),
			CurrentPage:   "invoice-detail",
			TabFocus:      action,
			SFInstanceURL: web.sessions.GetString(ctx, "salesforce-instance-url"),

			ViewDonations: viewDonations,
			Form:          form,
			Validator:     validator,
			Pagination:    pagination,
		}

		web.log.Debug(fmt.Sprintf("invoiceDetail: about to complete: %s", thisURL))

		web.render(w, r, templates, name, data)
	})
}

// handleBankTransactionDetail serves the page at /bank-transaction/<id>, showing
// details for a single bank transaction.
func (web *WebApp) handleBankTransactionDetail() http.Handler {

	name := "bank-transaction.html"
	tpls := []string{
		"base.html",
		"nav.html",
		"partial-listingTabs.html",
		"partial-donations-tabs.html",
		"partial-donations-linked.html",
		"partial-donations-searchform.html",
		"partial-donations-searchresults.html",
		"bank-transaction.html",
	}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()

		// Extract route parameters.
		vars, err := validMuxVars(mux.Vars(r), "id")
		if err != nil {
			web.clientError(w, err.Error(), http.StatusBadRequest)
			return
		}
		transactionID := vars["id"]

		// The 'action' route parameter is optional, but this handler reroutes to an
		// "action" version of the url if it is empty.
		action := "link"
		if a, ok := mux.Vars(r)["action"]; ok {
			action = a
		}
		baseURL := fmt.Sprintf("/bank-transaction/%s/%s", transactionID, action)

		// Get the transaction details.
		var transaction db.WRTransaction
		var lineItems []db.WRLineItem
		transaction, lineItems, err = web.db.BankTransactionWRGet(ctx, transactionID)
		if err != nil && err != sql.ErrNoRows {
			web.ServerError(w, r, err)
			return
		}
		if err == sql.ErrNoRows {
			web.notFound(w, r, fmt.Sprintf("transaction %q not found", transactionID))
			return
		}

		// Process the line items.
		viewLineItems := newViewLineItems(lineItems)

		// Determine the dates for retrieving donations.
		startDate, endDate := donationSearchTimeSpan(transaction.Date)

		// Initialise url parameter form and derive default url.
		form := NewSearchDonationsForm(&startDate, &endDate)

		// Derive the default url.
		urlParams, err := form.AsURLParams()
		if err != nil { // unlikely
			web.log.Error(fmt.Sprintf("handleTransactionDetail: default url error: %v", err))
			web.ServerError(w, r, err)
		}
		defaultURL := baseURL + "?" + urlParams

		// If the url has no 'action' path param or has the 'reset' query param, clear
		// the last saved url and redirect to default.
		if r.URL.Query().Get("reset") == "true" {
			_ = web.sessions.PopString(ctx, baseURL)
			http.Redirect(w, r, defaultURL, http.StatusSeeOther)
			return
		}

		// If the url is 'naked', redirect to either the saved or default.
		if r.URL.RawQuery == "" {
			if savedURL := web.sessions.GetString(ctx, baseURL); savedURL != "" {
				http.Redirect(w, r, savedURL, http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, defaultURL, http.StatusSeeOther)
			return
		}

		// Decode the url params and construct the current url, interjecting if the
		// action is "unlink" to get the related information.
		if err := DecodeURLParams(r, form); err != nil {
			web.ServerError(w, r, err)
		}

		DFK := *transaction.Reference
		if DFK == "" {
			DFK = missingTransactionReference
		}

		if action == "unlink" {
			form.DateFrom = web.cfg.DataStartDate
			form.DateTo = time.Now().AddDate(1, 0, 0)
			form.LinkageStatus = "Linked"
			form.PayoutReference = DFK
		}

		urlParams, err = form.AsURLParams()
		if err != nil {
			web.ServerError(w, r, err)
		}
		thisURL := baseURL + "?" + urlParams

		// Save the url.
		web.sessions.Put(ctx, baseURL, thisURL)

		// Create a validator and validate the form.
		validator := NewValidator()
		form.Validate(validator)

		// Get the donations if the form is valid
		var donations []db.Donation
		if validator.Valid() {
			donations, err = web.db.DonationsGet(
				ctx,
				form.DateFrom,
				form.DateTo,
				form.LinkageStatus,
				form.PayoutReference,
				form.SearchString,
				pageLen,
				form.Offset(pageLen),
			)
			if err != nil && err != sql.ErrNoRows {
				web.ServerError(w, r, err)
				return
			}
		}

		// Process donations into donationView type
		viewDonations := newViewDonations(donations)

		// Set pagination for number of donations. In case of an error, log
		// and continue. Each donation has the search query row count as a
		// field.
		var recordsNo int
		if len(viewDonations) == 0 {
			recordsNo = 1
		} else {
			recordsNo = viewDonations[0].RowCount
		}

		// Todo: fix page number (here 1)
		pagination, err := NewPagination(pageLen, recordsNo, form.Page, r.URL.Query())
		if err != nil {
			web.log.Error(fmt.Sprintf("pagination error: %v", err))
		}

		// Prepare data for the template.
		data := struct {
			PageTitle     string
			Transaction   db.WRTransaction
			LineItems     []viewLineItem
			ID            string
			DFK           string // for transactions, this is the Reference
			Typer         string
			ShortCode     string
			CurrentPage   string
			TabFocus      string
			SFInstanceURL string

			// Donation data
			ViewDonations []viewDonation
			Form          *SearchDonationsForm
			Validator     *Validator
			Pagination    *Pagination
		}{
			PageTitle:     fmt.Sprintf("Bank Transaction %s", transaction.ID),
			Transaction:   transaction,
			LineItems:     viewLineItems,
			ID:            transaction.ID,
			DFK:           DFK,
			Typer:         "bank-transaction",
			ShortCode:     web.sessions.GetString(ctx, "xero-shortcode"),
			CurrentPage:   "transaction-detail",
			TabFocus:      action,
			SFInstanceURL: web.sessions.GetString(ctx, "salesforce-instance-url"),

			ViewDonations: viewDonations,
			Form:          form,
			Validator:     validator,
			Pagination:    pagination,
		}

		web.log.Debug(fmt.Sprintf("transactionDetail: about to complete: %s", thisURL))

		web.render(w, r, templates, name, data)
	})
}

// handleDonationsLinkUnlink links or unlinks donations to either Xero invoices or bank
// transactions.
//
// The target here is hx-post="/donations/{{ .Typer }}/{{ .ID }}/(link|unlink)"
// However .ID is the bank-transaction or invoice UUID and the linking DFK info is
// the bank-transaction *reference* or invoice *invoice-number*. The DFK (and record
// date) is therefore retrieved using the `getInvoiceOrBankTransactionDetails` method.
func (web *WebApp) handleDonationsLinkUnlink() http.Handler {

	dataStartDate := web.cfg.DataStartDate

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()
		if r.Method != "POST" {
			web.htmxClientError(w, "only POST requests allowed")
			return
		}

		// Extract url parameters.
		vars, err := validMuxVars(mux.Vars(r), "type", "id", "action")
		if err != nil {
			web.log.Error(fmt.Sprintf("link/unlink error: invalid mux vars: %v", err))
			web.htmxClientError(w, err.Error())
			return
		}

		// Extract the form data.
		err = r.ParseForm()
		if err != nil {
			fmt.Printf("invalid POST request: %v", err)
			web.htmxClientError(
				w,
				fmt.Sprintf("%s form error: invalid POST request: %v", vars["action"], err),
			)
			return
		}

		// Validate the form data
		form, err := CheckLinkOrUnlinkForm(r.PostForm, vars)
		if err != nil {
			fmt.Printf("invalid form data: %v", err)
			web.htmxClientError(
				w,
				fmt.Sprintf("%s form error: invalid form data: %v", vars["action"], err),
			)
			return
		}
		validator := NewValidator()
		form.Validate(validator)
		if !validator.Valid() {
			web.log.Error(fmt.Sprintf("invalid data was received: %v", validator.Errors))
			web.htmxClientError(
				w,
				fmt.Sprintf("%s form error: invalid data was received: %v", vars["action"], validator.Errors))
			return
		}

		web.log.Info(fmt.Sprintf("donationLinkUnlink %s action called for %s : %s (%d donations)",
			form.Action,
			form.Typer,
			form.ID,
			len(form.DonationIDs),
		))

		// In link mode, retrieve the details of the invoice or bank transaction.
		// retrieve the related invoice or bank transaction dfk and date
		var dfk string
		if form.Action == "link" {
			dfk, _, err = web.getInvoiceOrBankTransactionDetails(ctx, form.Typer, form.ID)
			if err != nil {
				web.ServerError(w, r, fmt.Errorf("could not get invoice or bank transaction info: %w", err))
				web.htmxClientError(
					w,
					fmt.Sprintf("%s id: %s error: could get invoice/transaction info: %v", form.Typer, form.ID, err))
				return
			}
			if dfk == "" || dfk == missingTransactionReference {
				web.ServerError(w, r, fmt.Errorf("%s id %s had empty or invalid dfk and cannot be linked", form.Typer, form.ID))
				web.htmxClientError(
					w,
					fmt.Sprintf("%s id %s has an empty dfk and cannot be linked", form.Typer, form.ID))
				return
			}
		}

		// Retrieve the oauth2 tokens from the session
		sfToken, err := web.getValidTokenFromSession(ctx, token.SalesforceToken)
		if err != nil {
			web.log.Info("sfToken empty, redirecting to connect")
			w.Header().Set("HX-Redirect", "/connect")
			w.WriteHeader(http.StatusOK)
			return
		}

		// Create the salesforce client and run a batch update.
		sfClient, err := salesforce.NewClient(ctx, web.cfg, web.log, sfToken)
		if err != nil {
			web.ServerError(w, r, fmt.Errorf("failed to create salesforce client for linking/unlinking: %w", err))
			return
		}

		sfLastRefresh := web.sessions.GetTime(ctx, "sf-refreshed-datetime")

		// Update the donations. If it is an unlink action, update the dfk with "", else
		// the actual dfk from the bank transaction or invoice.
		_, err = sfClient.BatchUpdateOpportunityRefs(ctx, dfk, form.DonationIDs, false)
		if err != nil {
			web.ServerError(w, r, fmt.Errorf("failed to batch update salesforce records for linking/unlinking: %w", err))
			return
		}
		web.log.Info(fmt.Sprintf("Successfully linked %d donations.", len(form.DonationIDs)))

		// Upsert the updated opportunities.
		// The refresh window is rough; double upserts shouldn't be a major issue.
		updatedDonations, err := sfClient.GetOpportunities(ctx, dataStartDate, sfLastRefresh.Add(refreshDurationWindow))
		if err != nil {
			web.ServerError(w, r, fmt.Errorf("failed to upsert the linked opportunities: %v", err))
			return
		}
		if err := web.db.UpsertDonations(ctx, updatedDonations); err != nil {
			web.ServerError(w, r, fmt.Errorf("failed to save updated donations to local DB: %v", err))
			return
		}

		// Redirect to the originator.
		// Todo: set focus to either the "find" or "linked" donations tab.
		redirectURL := fmt.Sprintf("/%s/%s/%s", form.Typer, form.ID, form.Action)
		w.Header().Set("HX-Redirect", redirectURL)
		w.WriteHeader(http.StatusOK)

	})
}

/* -------------------------------------------------------------------------- */
// Helpers
/* -------------------------------------------------------------------------- */

// render renders the specified template.
func (web *WebApp) render(w http.ResponseWriter, r *http.Request, template *template.Template, filename string, data any) {
	buf := new(bytes.Buffer)
	err := template.ExecuteTemplate(buf, filename, data)
	if err != nil {
		web.log.Error(fmt.Sprintf("template %q rendering error %v", filename, err))
		web.ServerError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = buf.WriteTo(w)
}

// ServerError logs and return an internal server 500 error. The error should contain
// the information needed for logging.
func (web *WebApp) ServerError(w http.ResponseWriter, r *http.Request, errs ...error) {
	err := errors.Join(errs...)
	web.log.Error(err.Error(), "method", r.Method, "uri", r.URL.RequestURI())
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

// clientError returns a client error.
func (web *WebApp) clientError(w http.ResponseWriter, message string, status int) {
	if message == "" {
		web.log.Warn(fmt.Sprintf("client error: status %d", status))
		message = http.StatusText(status)
	}
	web.log.Warn(fmt.Sprintf("client error: %s (status %d)", message, status))
	http.Error(w, message, status)
}

// htmxClientError returns an htmx client error.
func (web *WebApp) htmxClientError(w http.ResponseWriter, message string) {
	web.log.Warn(fmt.Sprintf("client htmx error: %s", message))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// htmx won't normally process a non-200 response.
	w.WriteHeader(http.StatusOK)
	errorString := fmt.Sprintf(
		`<div class="text-sm text-red px-4 pb-2">%s</div>`,
		message,
	)
	_, _ = w.Write([]byte(errorString))
}

// donationSearchTimeSpan uses a simple heuristic for determining the dates for a
// donation search, typically -6 weeks and +2 weeks around an invoice or bank
// transaction date. Most donations are prior to the date recorded for an invoice or bank transaction.
func donationSearchTimeSpan(dt time.Time) (time.Time, time.Time) {
	if dt.IsZero() {
		return dt, dt
	}
	start := dt.Add(time.Duration(-6 * 7 * 24 * time.Hour))
	end := dt.Add(time.Duration(+2 * 7 * 24 * time.Hour))
	return start, end
}

// notfound raises a 404 clientError.
func (web *WebApp) notFound(w http.ResponseWriter, r *http.Request, message string) {
	web.clientError(w, message, http.StatusNotFound)
}

// getInvoiceOrBankTransactionDetails returns the DFK and Date from an invoice or Bank
// Transaction identified by id (a uuid).
func (web *WebApp) getInvoiceOrBankTransactionDetails(ctx context.Context, typer string, id string) (string, time.Time, error) {
	var rt time.Time
	if typer == "invoice" {
		invoice, _, err := web.db.InvoiceWRGet(ctx, id)
		if err != nil {
			return "", rt, fmt.Errorf("could not get invoice details for %q: %w", id, err)
		}
		return invoice.InvoiceNumber, invoice.Date, nil
	}
	transaction, _, err := web.db.BankTransactionWRGet(ctx, id)
	if err != nil {
		return "", rt, fmt.Errorf("could not get transaction details for %q: %w", id, err)
	}
	ref := *transaction.Reference
	return ref, transaction.Date, nil
}

var ErrTokenMissingOrInvalid = errors.New("token missing or invalid")

// getValidTokenFromSession gets a Xero or Salesforce token from the session.
func (web *WebApp) getValidTokenFromSession(ctx context.Context, typer token.TokenType) (*token.ExtendedToken, error) {

	// Get token from session.
	et, ok := web.sessions.Get(ctx, typer.SessionName()).(token.ExtendedToken)
	if !ok {
		web.log.Info(fmt.Sprintf("%s token not found in session", typer))
		return nil, ErrTokenMissingOrInvalid
	}

	// Try and refresh the token.
	var cfg *oauth2.Config
	switch typer {
	case token.SalesforceToken:
		cfg = web.cfg.Salesforce.OAuth2Config
	case token.XeroToken:
		cfg = web.cfg.Xero.OAuth2Config
	}
	refreshed, err := et.ReuseOrRefresh(ctx, cfg)
	if err != nil {
		web.log.Warn(fmt.Sprintf("%s token error on refresh: %v", typer, err))
		return nil, ErrTokenMissingOrInvalid
	}

	// Update the session with the new token.
	web.sessions.Put(ctx, typer.SessionName(), et)
	if refreshed {
		web.log.Info(fmt.Sprintf("%s token refreshed in session", typer))
	} else {
		web.log.Info(fmt.Sprintf("%s token reused", typer))
	}

	return &et, nil
}
