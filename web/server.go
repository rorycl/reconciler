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
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"reconciler/apiclients/salesforce"
	"reconciler/apiclients/xero"
	"reconciler/config"
	"reconciler/db"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
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

// WebApp is the configuration object for the web server.
type WebApp struct {
	log              *slog.Logger
	cfg              *config.Config
	db               *db.DB
	staticFS         fs.FS // the fs holding the static web resources.
	templateFS       fs.FS // the fs holding the web templates.
	defaultStartDate time.Time
	defaultEndDate   time.Time
	server           *http.Server
	sessions         *scs.SessionManager
	inDevelopment    bool // in development mode
}

// New initialises a WebApp. An error type is returned for future use.
func New(
	logger *slog.Logger,
	cfg *config.Config,
	db *db.DB,
	staticFS fs.FS,
	templateFS fs.FS,
	start time.Time,
	end time.Time,
) (*WebApp, error) {

	if !end.IsZero() && start.After(end) {
		return nil, fmt.Errorf("start date %s after end %s", start.Format("2006-01-2"), end.Format("2006-01-02"))
	}

	// Add settings for the http server.
	server := &http.Server{
		Addr:              cfg.Web.ListenAddress,
		ReadHeaderTimeout: time.Duration(240 * time.Second),
		WriteTimeout:      time.Duration(240 * time.Second),
		MaxHeaderBytes:    1 << 19, // 100k ish
	}

	// Initialise in-memory session store.
	scsSessionStore := scs.NewSession()
	scsSessionStore.Lifetime = 12 * time.Hour

	webApp := &WebApp{
		log:              logger,
		cfg:              cfg,
		db:               db,
		staticFS:         staticFS,
		templateFS:       templateFS,
		defaultStartDate: start,
		defaultEndDate:   end,
		server:           server,
		sessions:         scsSessionStore,
	}

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

// StartServer starts a WebApp.
func (web *WebApp) StartServer() error {
	web.server.Handler = web.routes()
	web.log.Info(fmt.Sprintf("Starting server on %s", web.cfg.Web.ListenAddress))
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

	// Xero OAuth2 init and callback.
	r.Handle("/xero/init", xero.InitiateWebLogin(web.cfg, web.sessions)).Methods("GET")
	r.Handle(web.cfg.Web.XeroCallBack, xero.WebLoginCallBack(web.cfg, web.sessions, web)).Methods("GET")

	// Salesforce OAuth2 init and callback.
	r.Handle("/salesforce/init", salesforce.InitiateWebLogin(web.cfg, web.sessions)).Methods("GET")
	r.Handle(web.cfg.Web.SalesforceCallBack, salesforce.WebLoginCallBack(web.cfg, web.sessions, web)).Methods("GET")

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
	r.Handle("/bank-transaction/{id:[A-Za-z0-9_-]+}", apisOK(web.handleBankTransactionDetail())).Methods("GET")

	// Partial pages.
	// These are HTMX partials showing donation listings in "linked" and "find to link" modes.
	r.Handle("/partials/donations-linked/{type:(?:invoice|bank-transaction)}/{id}", apisOK(web.handlePartialDonationsLinked())).Methods("GET")
	r.Handle("/partials/donations-find/{type:(?:invoice|bank-transaction)}/{id}", apisOK(web.handlePartialDonationsFind())).Methods("GET")

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

// apisConnectedOK checks whether the user is connected to the api services. If not, the user is
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
		if !xero.TokenIsValid(web.cfg.Xero.TokenFilePath, web.cfg.Xero.TokenTimeoutDuration) {
			web.log.Info("xero token is not valid, redirecting")
			http.Redirect(w, r, "/connect?status=xero_token_invalid", http.StatusSeeOther)
			return
		}
		if !salesforce.TokenIsValid(web.cfg.Salesforce.TokenFilePath, web.cfg.Salesforce.TokenTimeoutDuration) {
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

		xeroTokenIsValid := xero.TokenIsValid(web.cfg.Xero.TokenFilePath, web.cfg.Xero.TokenTimeoutDuration)
		sfTokenIsValid := salesforce.TokenIsValid(web.cfg.Salesforce.TokenFilePath, web.cfg.Salesforce.TokenTimeoutDuration)

		data := map[string]any{
			"SFTokenIsValid":   sfTokenIsValid,
			"XeroTokenIsValid": xeroTokenIsValid,
		}
		web.render(w, r, templates, name, data)
	})
}

// handleRefresh serves the /refresh page.
func (web *WebApp) handleRefresh() http.Handler {

	name := "refresh.html"
	tpls := []string{"base.html", "refresh.html"}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))

	// The start date from which data will be downloaded is set in the configuration file.
	dataStartDate := web.cfg.DataStartDate

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		refreshed := web.sessions.GetBool(ctx, "refreshed")
		lastRefresh := web.sessions.GetTime(ctx, "refreshed-datetime")

		data := map[string]any{
			"DataStartDate": dataStartDate,
			"Refreshed":     refreshed,
			"LastRefresh":   lastRefresh,
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

	dataStartDate := web.cfg.DataStartDate

	logAndPrintErrorToWeb := func(w io.Writer, format string, a ...any) {
		web.log.Error(fmt.Sprintf(format, a...))
	}

	logAndPrintToWeb := func(w io.Writer, format string, a ...any) {
		web.log.Info(fmt.Sprintf(format, a...))
		// Writing output here doesn't work because it will be buffered.
		// Todo: consider SSE solution.
		// _, _ = fmt.Fprintf(w, format, a)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()

		lastRefresh := web.sessions.GetTime(ctx, "refreshed-datetime")

		// Determine if refreshing needs to occur. If it does, set the refresh time to 5
		// minutes before the last refresh to deal with any slow updates on the remote
		// platform.
		refreshDeadline := time.Now().Add(-3 * time.Minute)
		if refreshDeadline.After(lastRefresh) {
			web.sessions.Put(ctx, "refreshed", false)
		} else {
			lastRefresh.Add(-5 * time.Minute)
		}
		isRefreshed := web.sessions.GetBool(ctx, "refreshed")
		web.log.Info(fmt.Sprintf("Refresh status: %t", isRefreshed))
		web.log.Info(fmt.Sprintf("Refresh last refresh: %s", lastRefresh.Format(time.DateTime)))

		// Connect the Xero client.
		xeroClient, err := xero.NewClient(ctx, web.cfg, web.log)
		if err != nil {
			// Todo: consider redirect to /connect.
			web.ServerError(w, r, fmt.Errorf("failed to create xero client: %w", err))
			return
		}
		web.log.Info("Xero client authenticated successfully.")

		// Retrieve the Xero accounts, bank transactions and invoices.
		// The following steps ideally would update the content at the end of the
		// templates/refresh #data-refresh-updates div.

		// Accounts
		accounts, err := xeroClient.GetAccounts(ctx, dataStartDate)
		if err != nil {
			logAndPrintErrorToWeb(w, "accounts retrieval error: %v", err)
			return
		}
		logAndPrintToWeb(w, "retrieved %d account records", len(accounts))

		if err := web.db.AccountsUpsert(ctx, accounts); err != nil {
			logAndPrintErrorToWeb(w, "failed to upsert account records: %v", err)
			return
		}
		logAndPrintToWeb(w, "Successfully upserted accounts to database.")

		// Bank Transactions
		transactions, err := xeroClient.GetBankTransactions(ctx, dataStartDate, lastRefresh)
		if err != nil {
			logAndPrintErrorToWeb(w, "bank transaction retrieval error: %v", err)
			return
		}
		logAndPrintToWeb(w, "retrieved %d bank transactions", len(transactions))

		if err = web.db.BankTransactionsUpsert(ctx, transactions); err != nil {
			logAndPrintErrorToWeb(w, "failed to upsert bank transactions: %v", err)
			return
		}
		logAndPrintToWeb(w, "Successfully upserted bank transactions to database.")

		// Invoices
		invoices, err := xeroClient.GetInvoices(ctx, dataStartDate, lastRefresh)
		if err != nil {
			logAndPrintErrorToWeb(w, "invoices retrieval error: %v", err)
			return
		}
		logAndPrintToWeb(w, "retrieved %d invoices", len(invoices))

		if err := web.db.InvoicesUpsert(ctx, invoices); err != nil {
			logAndPrintErrorToWeb(w, "failed to upsert invoices", err)
			return
		}
		logAndPrintToWeb(w, "Successfully upserted invoices to database.")

		// Retrieve the Salesforce donations.
		sfClient, err := salesforce.NewClient(ctx, web.cfg, web.log)
		if err != nil {
			// consider redirect to /connect
			logAndPrintErrorToWeb(w, "donations new client error: %v", err)
			return
		}
		donations, err := sfClient.GetOpportunities(ctx, dataStartDate, lastRefresh)
		if err != nil {
			logAndPrintErrorToWeb(w, "failed to retrieve donations: %v", err)
			return
		}
		logAndPrintToWeb(w, "retrieved %d donations", len(donations))
		if err := web.db.UpsertDonations(ctx, donations); err != nil {
			logAndPrintErrorToWeb(w, "failed to upsert donations", err)
			return
		}
		logAndPrintToWeb(w, "successfully upserted donations to database")

		// Update session information
		web.sessions.Put(ctx, "refreshed", true)
		web.sessions.Put(ctx, "refreshed-datetime", time.Now())

		// Redirect to invoices
		w.Header().Set("HX-Redirect", "/invoices")
		w.WriteHeader(http.StatusOK)
		return

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

	name := "invoices.html"
	tpls := []string{"base.html", "partial-listingTabs.html", "invoices.html"}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()

		form := NewSearchForm()
		if err := DecodeURLParams(r, form); err != nil {
			web.ServerError(w, r, err)
			return
		}

		// Create a validator and validate the form.
		validator := NewValidator()
		form.Validate(validator)

		// Initialise pagination for default state.
		pagination, _ := NewPagination(pageLen, 1, form.Page, r.URL.Query())

		// Prepare data for the template, allowing passing of validation
		// errors back to the template if necessary.
		data := struct {
			PageTitle   string
			Invoices    []db.Invoice
			Form        *SearchForm
			Validator   *Validator
			Pagination  *Pagination
			CurrentPage string
		}{
			PageTitle:   "Invoices",
			Form:        form,
			Validator:   validator,
			Pagination:  pagination,
			CurrentPage: "invoices",
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
			form.Offset(),
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

		web.render(w, r, templates, name, data)
	})
}

// handleBankTransactions serves the /bank-transactions bank transactions list.
func (web *WebApp) handleBankTransactions() http.Handler {

	name := "bank_transactions.html"
	tpls := []string{"base.html", "partial-listingTabs.html", "bank_transactions.html"}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()

		form := NewSearchForm()
		if err := DecodeURLParams(r, form); err != nil {
			web.ServerError(w, r, err)
			return
		}

		// Create a validator and validate the form.
		validator := NewValidator()
		form.Validate(validator)

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
		}{
			PageTitle:   "Bank Transactions",
			Form:        form,
			Validator:   validator,
			Pagination:  pagination,
			CurrentPage: "bank-transactions",
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
			form.Offset(),
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

		web.render(w, r, templates, name, data)
	})
}

// handleDonations serves the /donations list of donations.
func (web *WebApp) handleDonations() http.Handler {

	name := "donations.html"
	tpls := []string{"base.html", "partial-listingTabs.html", "partial-donations-searchform.html", "partial-donations-searchresults.html", "donations.html"}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()

		form := NewSearchDonationsForm()
		if err := DecodeURLParams(r, form); err != nil {
			web.ServerError(w, r, err)
			return
		}

		// Create a validator and validate the form.
		validator := NewValidator()
		form.Validate(validator)

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
			Typer         string // needed for the partial search results
			Validator     *Validator
			Pagination    *Pagination
			CurrentPage   string
			PageType      string
			GetURL        string
			SFInstanceURL string
		}{
			PageTitle:     "Donations",
			Form:          form,
			Typer:         "direct",
			Validator:     validator,
			Pagination:    pagination,
			CurrentPage:   "donations",
			PageType:      "direct", // indirect pages are htmx pages that have an hx target
			GetURL:        "/donations",
			SFInstanceURL: instanceURL,
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
			form.Offset(),
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
			http.Redirect(w, r, "/donations", http.StatusFound)
		}

		web.render(w, r, templates, name, data)
	})
}

// handleInvoiceDetail serves the detail page at /invoice/<id> for a single invoice.
func (web *WebApp) handleInvoiceDetail() http.Handler {

	name := "invoice.html"
	tpls := []string{"base.html", "partial-listingTabs.html", "partial-donations-tabs.html", "invoice.html"}
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

		// Get salesforce instance url from the session (via OAuth2 token)
		instanceURL := web.sessions.GetString(ctx, "salesforce-instance-url")

		data := struct {
			PageTitle string
			Invoice   db.WRInvoice
			LineItems []viewLineItem
			ID        string
			DFK       string // for Invoices, this is the Invoice Number
			TabType   string
			Typer     string
			// Donation Search Dates
			DonationSearchStart, DonationSearchEnd time.Time
			SFInstanceURL                          string
		}{
			PageTitle:     fmt.Sprintf("Invoice %s", invoiceID),
			ID:            invoiceID,
			TabType:       "link", // by default the donations tab type is "link", not "find"
			Typer:         "invoice",
			SFInstanceURL: instanceURL,
		}

		var lineItems []db.WRLineItem
		data.Invoice, lineItems, err = web.db.InvoiceWRGet(ctx, invoiceID)
		if err != nil && err != sql.ErrNoRows {
			web.ServerError(w, r, err)
			return
		}
		data.DonationSearchStart, data.DonationSearchEnd = donationSearchTimeSpan(data.Invoice.Date)

		data.LineItems = newViewLineItems(lineItems)
		data.DFK = data.Invoice.InvoiceNumber

		// Return a 404 if no invoice was found.
		if errors.Is(err, sql.ErrNoRows) {
			web.notFound(w, r, fmt.Sprintf("Invoice: %q not found", invoiceID))
			return
		}

		web.render(w, r, templates, name, data)
	})
}

// handleBankTransactionDetail serves the page at /bank-transaction/<id>, showing
// details for a single bank transaction.
func (web *WebApp) handleBankTransactionDetail() http.Handler {

	name := "bank-transaction.html"
	tpls := []string{"base.html", "partial-listingTabs.html", "partial-donations-tabs.html", "bank-transaction.html"}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()

		// Extract route parameters.
		vars, err := validMuxVars(mux.Vars(r), "id")
		if err != nil {
			web.clientError(w, err.Error(), http.StatusBadRequest)
			return
		}
		transactionReference := vars["id"]

		// Get salesforce instance url from the session (via OAuth2 token)
		instanceURL := web.sessions.GetString(ctx, "salesforce-instance-url")

		data := struct {
			PageTitle   string
			Transaction db.WRTransaction
			LineItems   []viewLineItem
			ID          string
			DFK         string // for Transactions , this is the Reference
			TabType     string
			Typer       string
			// Donation Search Dates
			DonationSearchStart, DonationSearchEnd time.Time
			SFInstanceURL                          string
		}{
			PageTitle:     fmt.Sprintf("Bank Transaction %s", transactionReference),
			ID:            transactionReference,
			TabType:       "link", // by default the donations tab type is "link", not "find"
			Typer:         "bank-transaction",
			SFInstanceURL: instanceURL,
		}

		var lineItems []db.WRLineItem
		data.Transaction, lineItems, err = web.db.BankTransactionWRGet(ctx, transactionReference)
		if err != nil && err != sql.ErrNoRows {
			web.ServerError(w, r, err)
			return
		}
		data.DonationSearchStart, data.DonationSearchEnd = donationSearchTimeSpan(data.Transaction.Date)

		data.LineItems = newViewLineItems(lineItems)
		data.DFK = *data.Transaction.Reference
		if data.DFK == "" {
			data.DFK = missingTransactionReference
		}

		// Return a 404 if no bank transaction was found.
		if errors.Is(err, sql.ErrNoRows) {
			web.notFound(w, r, fmt.Sprintf("Bank Transaction: %q not found", transactionReference))
			return
		}

		web.render(w, r, templates, name, data)
	})
}

// handlePartialDonationsLinked is the partial htmx endpoint for rendering the list of
// donations linked to an Invoice or Bank Transaction.
func (web *WebApp) handlePartialDonationsLinked() http.Handler {

	name := "partial-donations-linked.html"
	tpls := []string{"partial-donations-tabs.html", "partial-donations-linked.html"}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()

		// Extract url parameters.
		vars, err := validMuxVars(mux.Vars(r), "type", "id")
		if err != nil {
			web.clientError(w, err.Error(), http.StatusBadRequest)
			return
		}
		typer := vars["type"]
		id := vars["id"]

		// Initialise pagination for default state.
		pagination, _ := NewPagination(pageLen, 1, 1, r.URL.Query())

		// retrieve the related invoice or bank transaction dfk and date
		dfk, dater, err := web.getInvoiceOrBankTransactionDetails(ctx, typer, id)
		if err != nil {
			web.ServerError(w, r, fmt.Errorf("could not get invoice or bank transaction info: %w", err))
			return
		}
		donationSearchStart, donationSearchEnd := donationSearchTimeSpan(dater)

		// Get salesforce instance url from the session (via OAuth2 token)
		instanceURL := web.sessions.GetString(ctx, "salesforce-instance-url")

		web.log.Info(fmt.Sprintf("donations linked for %s (%s) : dfk %s start %s end %s",
			typer,
			id,
			dfk,
			donationSearchStart.Format("2006-01-02"),
			donationSearchEnd.Format("2006-01-02"),
		))

		// Prepare data for the template.
		data := struct {
			ID            string
			Typer         string
			DFK           string
			ViewDonations []viewDonation
			Pagination    *Pagination
			TabType       string
			SFInstanceURL string
			// Donation Date Search parameters
			DonationSearchStart, DonationSearchEnd time.Time
		}{
			ID:                  id,
			Typer:               typer,
			DFK:                 dfk,
			Pagination:          pagination,
			TabType:             "link",
			SFInstanceURL:       instanceURL,
			DonationSearchStart: donationSearchStart,
			DonationSearchEnd:   donationSearchEnd,
		}

		donations, err := web.db.DonationsGet(
			ctx,
			web.defaultStartDate,
			web.defaultEndDate,
			"Linked", // linkage status
			dfk,      // payout reference
			"",       // searchstring
			pageLen,
			0, // form offset
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
		// Todo: fix url.query() if it doesn't have the "tab"
		// Todo: fix page number (here 1)
		data.Pagination, err = NewPagination(pageLen, recordsNo, 1, r.URL.Query())
		if err != nil {
			web.log.Error(fmt.Sprintf("pagination error: %v", err))
		}

		web.render(w, r, templates, name, data)
	})
}

// handlePartialDonationsFind is the partial htmx endpoint for finding
// donations to link to an Invoice or Bank Transaction.
func (web *WebApp) handlePartialDonationsFind() http.Handler {

	name := "partial-donations.html"
	tpls := []string{"partial-donations-tabs.html", "partial-donations-searchform.html", "partial-donations-searchresults.html", "partial-donations.html"}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()

		// Extract route parameters.
		vars, err := validMuxVars(mux.Vars(r), "type", "id")
		if err != nil {
			web.clientError(w, err.Error(), http.StatusBadRequest)
			return
		}
		typer := vars["type"]
		id := vars["id"]

		form := NewSearchDonationsForm()
		if err := DecodeURLParams(r, form); err != nil {
			web.ServerError(w, r, err)
		}

		// Create a validator and validate the form.
		validator := NewValidator()
		form.Validate(validator)

		// Initialise pagination for default state.
		pagination, _ := NewPagination(pageLen, 1, form.Page, r.URL.Query())

		// retrieve the related invoice or bank transaction dfk and date
		dfk, dater, err := web.getInvoiceOrBankTransactionDetails(ctx, typer, id)
		if err != nil {
			web.ServerError(w, r, fmt.Errorf("could not get invoice or bank transaction info: %w", err))
			return
		}
		donationSearchStart, donationSearchEnd := donationSearchTimeSpan(dater)

		// Get salesforce instance url from the session (via OAuth2 token)
		instanceURL := web.sessions.GetString(ctx, "salesforce-instance-url")

		// Prepare data for the template, allowing passing of validation
		// errors back to the template if necessary.
		data := struct {
			PageTitle     string
			ID            string
			Typer         string
			DFK           string
			ViewDonations []viewDonation
			Form          *SearchDonationsForm
			Validator     *Validator
			Pagination    *Pagination
			SFInstanceURL string

			PageType string
			GetURL   string
			TabType  string

			// Donation Date Search parameters
			DonationSearchStart, DonationSearchEnd time.Time
		}{
			PageTitle:     "Donations",
			ID:            id,
			Typer:         typer,
			DFK:           dfk,
			Form:          form,
			Validator:     validator,
			Pagination:    pagination,
			SFInstanceURL: instanceURL,

			PageType: "indirect", // indirect pages are htmx pages that have an hx target
			GetURL:   fmt.Sprintf("/partials/donations-find/%s/%s", typer, id),
			TabType:  "find",

			// Donation Date search parameters.
			DonationSearchStart: donationSearchStart,
			DonationSearchEnd:   donationSearchEnd,
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
			form.Offset(),
		)
		if err != nil && err != sql.ErrNoRows {
			web.ServerError(w, r, err)
			return
		}

		// Process donations into donationView type.
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
			web.log.Warn(fmt.Sprintf("pagination error: %v", err))
		}

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
			fmt.Printf("link/unlink error: invalid mux vars: %v", err)
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

		// Create the salesforce client and run a batch update.
		sfClient, err := salesforce.NewClient(ctx, web.cfg, web.log)
		if err != nil {
			// Todo: decide if sfClient fails to always delete the current token.
			/*
				if !salesforce.TokenIsValid(web.cfg.Salesforce.TokenFilePath, web.cfg.Salesforce.Token....) {
					...os.Remove...
					web.log.Warn("handle Donations Link/Unlink detected an invalid token, redirecting to /connect")
					http.Redirect(w, r, "/connect?error=sf_expired", http.StatusSeeOther)
					return
				}
			*/
			web.ServerError(w, r, fmt.Errorf("failed to create salesforce client for linking/unlinking: %w", err))
			return
		}

		timeStamp := time.Now()

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
		updatedDonations, err := sfClient.GetOpportunities(ctx, dataStartDate, timeStamp.Add(-2*time.Minute))
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
		redirectURL := fmt.Sprintf("/%s/%s", form.Typer, form.ID)
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
	buf.WriteTo(w)
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
	web.log.Warn("client error: %s (status %d)", message, status)
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
	w.Write([]byte(errorString))
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
