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
	"time"

	"github.com/rorycl/reconciler/config"
	"github.com/rorycl/reconciler/db"
	"github.com/rorycl/reconciler/domain"
	"github.com/rorycl/reconciler/internal/token"

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
// its records.
var refreshDurationWindow = -10 * time.Second

// refreshTrucation is the user-oriented refresh rounding duration.
var refreshTruncation = 10 * time.Second

// Exiter is the local Exit func
var Exiter func(code int) = os.Exit

// WebApp is the configuration object for the web server.
type WebApp struct {
	cfg            *config.Config
	log            *slog.Logger
	reconciler     reconcilerer // interface to domain.Reconciler
	staticFS       fs.FS        // the fs holding the static web resources.
	templateFS     fs.FS        // the fs holding the web templates.
	server         *http.Server
	sessions       *scs.SessionManager
	accountsRegexp *regexp.Regexp
	logoutDuration time.Duration // time to pause when logging out.

	// Xero and Salesforce client factory funcs allow the passing in of funcs that make a client that meets
	// the domain.XeroClient and domain.SalesforceClient interfaces.
	newXeroClient xeroClientMaker
	newSFClient   sfClientMaker

	// web clients for oauth2
	xeroWebClient *token.TokenWebClient
	sfWebClient   *token.TokenWebClient

	// in development mode
	inDevelopment bool
}

// New initialises a WebApp. An error type is returned for future use.
func New(
	config *config.Config,
	reconciler reconcilerer,
	logger *slog.Logger,
	staticFS fs.FS,
	templateFS fs.FS,
	xeroClientFunc xeroClientMaker,
	sfClientFunc sfClientMaker,
) (*WebApp, error) {

	// Add settings for the http server.
	// The timeout settings are intended to allow the API data refresh handler to run
	// without interruption.
	server := &http.Server{
		Addr:              config.Web.ListenAddress,
		ReadHeaderTimeout: time.Duration(300 * time.Second),
		WriteTimeout:      time.Duration(300 * time.Second),
		MaxHeaderBytes:    1 << 17, // 125k ish
	}

	// Initialise in-memory session store and related custom gob types.
	// Sessions have an absolute validity limit of 8 hours.
	// Sessions time out after 2 hours of inactivity.
	gob.Register(time.Time{})
	gob.Register(token.ExtendedToken{})

	scsSessionStore := scs.New()
	scsSessionStore.Lifetime = 8 * time.Hour
	scsSessionStore.IdleTimeout = 2 * time.Hour

	// Set the duration of the logout pause before closing the app.
	logoutDuration := time.Duration(1 * time.Second)

	// Compile the donation accounts filtering regexp
	// (this is a safe assignment, checked at config ingestion).
	accountsRegexp := config.DonationAccountCodesAsRegex()

	webApp := &WebApp{
		cfg:            config,
		reconciler:     reconciler,
		log:            logger,
		staticFS:       staticFS,
		templateFS:     templateFS,
		server:         server,
		sessions:       scsSessionStore,
		accountsRegexp: accountsRegexp,
		logoutDuration: logoutDuration,
	}

	// Client factory funcs. The default is to attach the full API clients.
	if xeroClientFunc == nil {
		webApp.newXeroClient = newDefaultXeroClient
	} else {
		webApp.newXeroClient = xeroClientFunc
	}
	if sfClientFunc == nil {
		webApp.newSFClient = newDefaultSalesforceClient
	} else {
		webApp.newSFClient = sfClientFunc
	}

	// Attach the salesforce and xero OAuth2 web client handler constructors.
	sfWebClient, err := token.NewTokenWebClient(
		token.SalesforceToken,
		config.Salesforce.OAuth2Config,
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
		config.Xero.OAuth2Config,
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
	if config.InDevelopmentMode {
		webApp.inDevelopment = true
		webApp.log.Warn("******************************************")
		webApp.log.Warn("       Warning: IN DEVELOPMENT mode       ")
		webApp.log.Warn("    ** API connection check disabled **   ")
		webApp.log.Warn("       Warning: IN DEVELOPMENT mode       ")
		webApp.log.Warn("******************************************")
	}

	return webApp, nil
}

// SetInDevelopment is a development-mode switch for setting the web app in development
// mode. This has no effect if called after the server has started.
func (web *WebApp) SetInDevelopment() {
	web.inDevelopment = true
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
	r.Handle("/", web.ErrorChecker(web.handleRoot())).Methods("GET") // synonym for /connect
	r.Handle("/connect", web.ErrorChecker(web.handleConnect())).Methods("GET")
	r.Handle("/logout", web.ErrorChecker(web.handleLogout())).Methods("GET")
	r.Handle("/logout/confirmed", web.ErrorChecker(web.handleLogoutConfirmed())).Methods("GET")

	// Xero OAuth2 init and callback.
	r.Handle("/xero/init", web.xeroWebClient.InitiateWebLogin()).Methods("GET")
	r.Handle(web.cfg.Web.XeroCallBack, web.xeroWebClient.WebLoginCallBack()).Methods("GET")

	// Salesforce OAuth2 init and callback.
	r.Handle("/salesforce/init", web.sfWebClient.InitiateWebLogin()).Methods("GET")
	r.Handle(web.cfg.Web.SalesforceCallBack, web.sfWebClient.WebLoginCallBack()).Methods("GET")

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
func (web *WebApp) handleRoot() appHandler {
	return func(w http.ResponseWriter, r *http.Request) error {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return nil
		}
		http.Redirect(w, r, "/connect", http.StatusFound)
		return nil
	}
}

// handleConnect serves the /connect endpoint.
func (web *WebApp) handleConnect() appHandler {

	name := "connect.html"
	tpls := []string{
		"base.html",
		"connect.html",
	}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))

	return func(w http.ResponseWriter, r *http.Request) error {

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
		return web.render(w, r, templates, name, data)
	}
}

// handleLogout serves the /logout endpoint.
func (web *WebApp) handleLogout() appHandler {

	name := "logout.html"
	tpls := []string{
		"base.html",
		"logout.html",
	}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))

	return func(w http.ResponseWriter, r *http.Request) error {
		data := map[string]any{
			"MemoryDatabase": web.reconciler.DBIsInMemory(),
			"DBName":         web.reconciler.DBPath(),
		}
		return web.render(w, r, templates, name, data)
	}
}

// handleLogoutConfirmed serves the /logout/confirmed endpoint, which clears the session
// and redirects to /connect.
func (web *WebApp) handleLogoutConfirmed() appHandler {

	return func(w http.ResponseWriter, r *http.Request) error {

		ctx := r.Context()

		// Clear the session.
		err := web.sessions.Clear(ctx)
		if err != nil {
			web.log.Error(fmt.Sprintf("Sesssion clear error: %v", err))
		} else {
			web.log.Info("Session cleared")
		}

		// Close the database and kill the session.
		_ = web.reconciler.Close()
		time.Sleep(web.logoutDuration)
		web.log.Info("Logout completed")
		if fl, ok := w.(http.Flusher); ok {
			_, _ = fmt.Fprint(w, "Logout completed. Please restart the program.")
			fl.Flush()
		}

		time.Sleep(web.logoutDuration)
		Exiter(0)
		return nil
	}
}

// handleRefresh serves the /refresh page.
func (web *WebApp) handleRefresh() appHandler {

	name := "refresh.html"
	tpls := []string{
		"base.html",
		"refresh.html",
	}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))

	// Configuration start date.
	dataStartDate := web.cfg.DataStartDate
	accountCodes := web.cfg.DonationAccountPrefixes

	return func(w http.ResponseWriter, r *http.Request) error {
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
			"Message":              web.sessions.PopString(ctx, "message"),
		}
		return web.render(w, r, templates, name, data)
	}
}

// handleRefreshUpdates serves the htmx partial /refresh/update info for refreshing data
// from the api platforms into the database.
func (web *WebApp) handleRefreshUpdates() appHandler {

	return func(w http.ResponseWriter, r *http.Request) error {

		ctx := r.Context()

		// Retrieve and upsert the Xero records.
		results, err := web.refreshXeroRecords(ctx)
		if err != nil {
			// Todo: report errors to client.
			msg := fmt.Sprintf("failed to refresh Xero records: %v", err)
			web.log.Error(msg)
			web.sessions.Put(ctx, "message", msg)
			http.Redirect(w, r, "/refresh", http.StatusFound)
			return nil
		}

		// Set the xero shortcode in the session if a full refresh occurred.
		if results.FullRefresh && results.ShortCode != "" {
			web.sessions.Put(ctx, "xero-shortcode", results.ShortCode)
		}

		// Retrieve and upsert the Salesforce records.
		_, err = web.refreshSalesforceRecords(ctx)
		if err != nil {
			// Todo: report errors to client.
			web.log.Error(fmt.Sprintf("failed to refresh Salesforce records: %v", err))
			http.Redirect(w, r, "/refresh", http.StatusFound)
			return nil
		}
		web.log.Info("Refresh successfully completed.")

		// Redirect to invoices
		w.Header().Set("HX-Redirect", "/invoices")
		w.WriteHeader(http.StatusOK)
		return nil

	}
}

// handleHome redirects from /home.
func (web *WebApp) handleHome() appHandler {
	return func(w http.ResponseWriter, r *http.Request) error {
		http.Redirect(w, r, "/invoices", http.StatusFound)
		return nil
	}
}

// handleInvoices serves the /invoices list.
func (web *WebApp) handleInvoices() appHandler {

	thisURL := "/invoices"
	name := "invoices.html"
	tpls := []string{
		"base.html",
		"nav.html",
		"partial-listingTabs.html",
		"invoices.html",
	}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))
	dataStartDate := web.cfg.DataStartDate

	return func(w http.ResponseWriter, r *http.Request) error {

		ctx := r.Context()

		// Initialise url parameter form and derive url.
		form := NewSearchForm(&web.cfg.DataStartDate, nil)

		// Check if a redirection is needed.
		derivedURL, redirect, err := redirectCheck(ctx, form, web.sessions, r, thisURL)
		if err != nil {
			return errInternal{"redirectCheck", err}
		}
		if redirect {
			web.log.Info(fmt.Sprintf("redirecting to %s", derivedURL))
			http.Redirect(w, r, derivedURL, http.StatusSeeOther)
			return nil
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
				return err
			}
			http.Redirect(w, r, derivedURL, http.StatusSeeOther)
			return nil
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
			return web.render(w, r, templates, name, data)
		}

		invoices, err := web.reconciler.InvoicesGet(
			ctx,
			form.ReconciliationStatus,
			form.DateFrom,
			form.DateTo,
			form.SearchString,
			pageLen,
			form.Offset(pageLen),
		)
		if err != nil && err != sql.ErrNoRows {
			return err
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
		web.sessions.Put(ctx, thisURL, derivedURL)

		return web.render(w, r, templates, name, data)
	}
}

// handleBankTransactions serves the /bank-transactions bank transactions list.
func (web *WebApp) handleBankTransactions() appHandler {

	thisURL := "/bank-transactions"
	name := "bank-transactions.html"
	tpls := []string{
		"base.html",
		"nav.html",
		"partial-listingTabs.html",
		"bank-transactions.html",
	}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))
	dataStartDate := web.cfg.DataStartDate

	return func(w http.ResponseWriter, r *http.Request) error {

		ctx := r.Context()

		// Initialise url parameter form.
		form := NewSearchForm(&web.cfg.DataStartDate, nil)

		// Check if a redirection is needed.
		derivedURL, redirect, err := redirectCheck(ctx, form, web.sessions, r, thisURL)
		if err != nil {
			web.ServerError(w, r, err)
		}
		if redirect {
			web.log.Info(fmt.Sprintf("redirecting to %s", derivedURL))
			http.Redirect(w, r, derivedURL, http.StatusSeeOther)
			return nil
		}

		// Create a validator and validate the form.
		validator := NewValidator()
		form.Validate(validator)

		// Determine the last refresh time of Xero data. Note this can be
		// time.Time.IsZero(), but this is unlikely since the user has already run a
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
			http.Redirect(w, r, derivedURL, http.StatusSeeOther)
			return nil
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
			return web.render(w, r, templates, name, data)
		}

		transactions, err := web.reconciler.TransactionsGet(
			ctx,
			form.ReconciliationStatus,
			form.DateFrom,
			form.DateTo,
			form.SearchString,
			pageLen,
			form.Offset(pageLen),
		)
		if err != nil && err != sql.ErrNoRows {
			return err
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
		web.sessions.Put(ctx, thisURL, derivedURL)

		return web.render(w, r, templates, name, data)
	}
}

// handleDonations serves the /donations list of donations.
func (web *WebApp) handleDonations() appHandler {

	thisURL := "/donations"
	name := "donations.html"
	tpls := []string{
		"base.html",
		"nav.html",
		"partial-listingTabs.html",
		"partial-donations-searchform.html",
		"partial-donations-searchresults.html",
		"donations.html",
	}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))
	dataStartDate := web.cfg.DataStartDate

	return func(w http.ResponseWriter, r *http.Request) error {

		ctx := r.Context()

		// Initialise url parameter form.
		form := NewSearchDonationsForm(&web.cfg.DataStartDate, nil)

		// Check if a redirection is needed.
		derivedURL, redirect, err := redirectCheck(ctx, form, web.sessions, r, thisURL)
		if err != nil {
			return err
		}
		if redirect {
			web.log.Info(fmt.Sprintf("redirecting to %s", derivedURL))
			http.Redirect(w, r, derivedURL, http.StatusSeeOther)
			return nil
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

			_, err = web.refreshSalesforceRecords(ctx)
			if err != nil {
				return err
			}
			http.Redirect(w, r, derivedURL, http.StatusSeeOther)
			return nil
		}

		// Initialise pagination for default state.
		pagination, _ := NewPagination(pageLen, 1, form.Page, r.URL.Query())

		// Get salesforce instance url from the session (via OAuth2 token)
		instanceURL := web.sessions.GetString(ctx, "salesforce-instance-url")

		// Prepare data for the template, allowing passing of validation
		// errors back to the template if necessary.
		data := struct {
			PageTitle     string
			ViewDonations []domain.ViewDonation
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
			return web.render(w, r, templates, name, data)
		}

		viewDonations, err := web.reconciler.DonationsGet(
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
			return err
		}

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
		web.sessions.Put(ctx, thisURL, derivedURL)

		return web.render(w, r, templates, name, data)
	}
}

// handleInvoiceDetail serves the detail page at /invoice/<id> for a single invoice.
// Note that the func can also be invoked at `/invoice/<id>/<action>(link|unlink)`. The
// former redirect to the latter's 'link' form by default.
func (web *WebApp) handleInvoiceDetail() appHandler {

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

	return func(w http.ResponseWriter, r *http.Request) error {

		ctx := r.Context()

		// Extract route parameters.
		vars, err := validMuxVars(mux.Vars(r), "id")
		if err != nil {
			return errUsage{err.Error(), http.StatusBadRequest}
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
		var viewLineItems []domain.ViewLineItem
		invoice, viewLineItems, err = web.reconciler.InvoiceDetailGet(ctx, invoiceID)
		if err != nil {
			return err
		}

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
			return nil
		}

		// If the url is 'naked', redirect to either the saved or default.
		if r.URL.RawQuery == "" {
			if savedURL := web.sessions.GetString(ctx, baseURL); savedURL != "" {
				http.Redirect(w, r, savedURL, http.StatusSeeOther)
				return nil
			}
			http.Redirect(w, r, defaultURL, http.StatusSeeOther)
			return nil
		}

		// Decode the url params and construct the current url, interjecting if the
		// action is "unlink" to get the related information.
		if err := form.DecodeURLParams(r.URL.Query()); err != nil {
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
		var viewDonations []domain.ViewDonation
		if validator.Valid() {
			viewDonations, err = web.reconciler.DonationsGet(
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
				return err
			}
		}

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
			LineItems     []domain.ViewLineItem
			ID            string
			DFK           string // for Invoices, this is the Invoice Number
			Typer         string
			ShortCode     string
			CurrentPage   string
			TabFocus      string
			SFInstanceURL string

			// Donation data
			ViewDonations []domain.ViewDonation
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

		return web.render(w, r, templates, name, data)
	}
}

// handleBankTransactionDetail serves the page at /bank-transaction/<id>, showing
// details for a single bank transaction.
func (web *WebApp) handleBankTransactionDetail() appHandler {

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

	return func(w http.ResponseWriter, r *http.Request) error {

		ctx := r.Context()

		// Extract route parameters.
		vars, err := validMuxVars(mux.Vars(r), "id")
		if err != nil {
			return errUsage{err.Error(), http.StatusBadRequest}
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
		var viewLineItems []domain.ViewLineItem
		transaction, viewLineItems, err = web.reconciler.TransactionDetailGet(ctx, transactionID)
		if err != nil {
			return err
		}

		// Determine the dates for retrieving donations.
		startDate, endDate := donationSearchTimeSpan(transaction.Date)

		// Initialise url parameter form and derive default url.
		form := NewSearchDonationsForm(&startDate, &endDate)

		// Derive the default url.
		urlParams, err := form.AsURLParams()
		if err != nil { // unlikely
			return errInternal{msg: "url paramater error", err: err}
		}
		defaultURL := baseURL + "?" + urlParams

		// If the url has no 'action' path param or has the 'reset' query param, clear
		// the last saved url and redirect to default.
		if r.URL.Query().Get("reset") == "true" {
			_ = web.sessions.PopString(ctx, baseURL)
			http.Redirect(w, r, defaultURL, http.StatusSeeOther)
			return nil
		}

		// If the url is 'naked', redirect to either the saved or default.
		if r.URL.RawQuery == "" {
			if savedURL := web.sessions.GetString(ctx, baseURL); savedURL != "" {
				http.Redirect(w, r, savedURL, http.StatusSeeOther)
				return nil
			}
			http.Redirect(w, r, defaultURL, http.StatusSeeOther)
			return nil
		}

		// Decode the url params and construct the current url, interjecting if the
		// action is "unlink" to get the related information.
		if err := form.DecodeURLParams(r.URL.Query()); err != nil {
			return errInternal{msg: "url decoding error", err: err}
		}

		var DFK string
		if transaction.Reference == nil {
			DFK = missingTransactionReference
		} else {
			DFK = *transaction.Reference
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
		var viewDonations []domain.ViewDonation
		if validator.Valid() {
			viewDonations, err = web.reconciler.DonationsGet(
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
				return err
			}
		}

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
			LineItems     []domain.ViewLineItem
			ID            string
			DFK           string // for transactions, this is the Reference
			Typer         string
			ShortCode     string
			CurrentPage   string
			TabFocus      string
			SFInstanceURL string

			// Donation data
			ViewDonations []domain.ViewDonation
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

		return web.render(w, r, templates, name, data)
	}
}

/* -------------------------------------------------------------------------- */
// Helpers
/* -------------------------------------------------------------------------- */

// render renders the specified template.
func (web *WebApp) render(w http.ResponseWriter, r *http.Request, template *template.Template, filename string, data any) error {
	buf := new(bytes.Buffer)
	err := template.ExecuteTemplate(buf, filename, data)
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusOK)
	_, _ = buf.WriteTo(w)
	return nil
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
