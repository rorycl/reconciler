package web

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"reconciler/config"
	"reconciler/db"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

// pageLen is the number of items to show in a page listing.
const pageLen = 15

//go:embed static
var staticEmbeddedFS embed.FS

//go:embed templates
var templatesEmbeddedFS embed.FS

// WebApp is the configuration object for the web server.
type WebApp struct {
	log              *log.Logger
	cfg              *config.Config
	db               *db.DB
	staticFS         fs.FS // the fs holding the static web resources.
	templateFS       fs.FS // the fs holding the web templates.
	defaultStartDate time.Time
	defaultEndDate   time.Time
	server           *http.Server
}

// New initialises a WebApp. An error type is returned for future use.
func New(logger *log.Logger, cfg *config.Config, db *db.DB, staticFS, templateFS fs.FS, start, end time.Time) (*WebApp, error) {
	if start.After(end) {
		return nil, fmt.Errorf("start date %s after end %s", start.Format("2006-01-2"), end.Format("2006-01-02"))
	}

	// Add settings for the http server. Consider adding logging here.
	server := &http.Server{
		Addr:              cfg.Web.ListenAddress,
		ReadHeaderTimeout: time.Duration(30 * time.Second),
		WriteTimeout:      time.Duration(30 * time.Second),
		MaxHeaderBytes:    1 << 19, // 100k ish
	}

	webApp := &WebApp{
		log:              logger, // this conflicts with the gorilla logging middleware; also how about slog?
		cfg:              cfg,
		db:               db,
		staticFS:         staticFS,
		templateFS:       templateFS,
		defaultStartDate: start,
		defaultEndDate:   end,
		server:           server,
	}
	return webApp, nil
}

// StartServer starts a WebApp.
func (web *WebApp) StartServer() error {
	web.server.Handler = web.routes()
	web.log.Printf("Starting server on %s", web.cfg.Web.ListenAddress)
	return web.server.ListenAndServe()
}

// routes connects all of the endpoints and provides middleware.
func (web *WebApp) routes() http.Handler {

	r := mux.NewRouter()

	fs := http.FileServerFS(web.staticFS)
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", fs))

	r.Handle("/", web.handleRoot())
	r.Handle("/connect", web.handleConnect())
	r.Handle("/refresh", web.handleRefresh())
	r.Handle("/home", web.handleHome()) // redirect to handleInvoices.

	// Main listing pages.
	r.Handle("/invoices", web.handleInvoices())
	r.Handle("/bank-transactions", web.handleBankTransactions())
	r.Handle("/donations", web.handleDonations())

	// Detail pages.
	// Note that the regexp works for uuids and the system test data.
	r.Handle("/invoice/{id:[A-Za-z0-9_-]+}", web.handleInvoiceDetail())
	r.Handle("/bank-transaction/{id:[A-Za-z0-9_-]+}", web.handleBankTransactionDetail())

	// Partial pages.
	// These are HTMX partials showing donation listings in "linked" and "find to link" modes.
	r.Handle("/partials/donations-linked/{type:(?:invoice|bank-transaction)}/{id}", web.handlePartialDonationsLinked())
	r.Handle("/partials/donations-find/{type:(?:invoice|bank-transaction)}/{id}", web.handlePartialDonationsFind())

	logging := handlers.LoggingHandler(os.Stdout, r)
	return logging
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
		err := templates.ExecuteTemplate(w, name, nil)
		if err != nil {
			web.log.Printf("Error rendering template %s: %v", name, err)
			http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
		}
	})
}

// handleRefresh serves the /refresh page.
func (web *WebApp) handleRefresh() http.Handler {

	name := "refresh.html"
	tpls := []string{"base.html", "refresh.html"}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := templates.ExecuteTemplate(w, name, nil)
		if err != nil {
			web.log.Printf("Error rendering template %s: %v", name, err)
			http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
		}
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
			web.log.Printf("error: could not decode the url parameters: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
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
			err := templates.ExecuteTemplate(w, name, nil)
			if err != nil {
				web.log.Printf("Error rendering template %s: %v", name, err)
				http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
			}
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
			web.log.Printf("error: database GetInvoices failed: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
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
			web.log.Printf("pagination error: %v", err)
			http.Redirect(w, r, "/invoices", http.StatusFound)
		}

		err = templates.ExecuteTemplate(w, name, data)
		if err != nil {
			web.log.Printf("Error rendering template %s: %v", name, err)
			http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
		}
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
			web.log.Printf("error: could not decode the url parameters: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
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
			err := templates.ExecuteTemplate(w, name, data)
			if err != nil {
				web.log.Printf("Error rendering template %s: %v", name, err)
				http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
			}
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
			web.log.Printf("error: database GetBankTransactions failed: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
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
			web.log.Printf("pagination error: %v", err)
			http.Redirect(w, r, "/banktransactions", http.StatusFound)
		}

		err = templates.ExecuteTemplate(w, name, data)
		if err != nil {
			web.log.Printf("Error rendering template %s: %v", name, err)
			http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
		}
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
			web.log.Printf("error: could not decode the url parameters: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
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
			PageTitle     string
			ViewDonations []viewDonation
			Form          *SearchDonationsForm
			Validator     *Validator
			Pagination    *Pagination
			CurrentPage   string
			PageType      string
			GetURL        string
		}{
			PageTitle:   "Donations",
			Form:        form,
			Validator:   validator,
			Pagination:  pagination,
			CurrentPage: "donations",
			PageType:    "direct", // indirect pages are htmx pages that have an hx target
			GetURL:      "/donations",
		}

		// Render template with errors and return if the form is invalid.
		if !validator.Valid() {
			err := templates.ExecuteTemplate(w, name, data)
			if err != nil {
				web.log.Printf("Error rendering template %s: %v", name, err)
				http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
			}
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
			web.log.Printf("error: database GetDonations failed: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
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
			web.log.Printf("pagination error: %v", err)
			http.Redirect(w, r, "/donations", http.StatusFound)
		}

		err = templates.ExecuteTemplate(w, name, data)
		if err != nil {
			web.log.Printf("Error rendering template %s: %v", name, err)
			http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
		}
	})
}

// handleInvoiceDetail serves the detail page at /invoice/<id> for a single invoice.
func (web *WebApp) handleInvoiceDetail() http.Handler {

	name := "invoice.html"
	tpls := []string{"base.html", "partial-listingTabs.html", "partial-donations-tabs.html", "invoice.html"}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()

		vars := mux.Vars(r)
		if vars == nil {
			web.log.Printf("error: invoiceID capture (vars: %v)", mux.Vars(r))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		invoiceID, ok := vars["id"]
		if !ok {
			web.log.Printf("error: id not in mux.Vars (vars: %v)", mux.Vars(r))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}

		data := struct {
			PageTitle string
			Invoice   db.WRInvoice
			LineItems []viewLineItem
			ID        string
			TabType   string
		}{
			PageTitle: fmt.Sprintf("Invoice %s", invoiceID),
			ID:        invoiceID,
			TabType:   "link", // by default the donations tab type is "link", not "find"
		}

		var err error
		var lineItems []db.WRLineItem
		data.Invoice, lineItems, err = web.db.InvoiceWRGet(ctx, invoiceID)
		if err != nil && err != sql.ErrNoRows {
			web.log.Printf("error: database GetInvoices failed: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		data.LineItems = newViewLineItems(lineItems)

		// Return a 404 if no invoice was found.
		if errors.Is(err, sql.ErrNoRows) {
			web.log.Printf("Invoice: %q not found", invoiceID)
			http.Error(w, fmt.Sprintf("Invoice %q not found", invoiceID), http.StatusNotFound)
			return
		}

		err = templates.ExecuteTemplate(w, name, data)
		if err != nil {
			web.log.Printf("Error rendering template %s: %v", name, err)
			http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
		}
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

		vars := mux.Vars(r)
		if vars == nil {
			web.log.Printf("error: bank transaction capture (vars: %v)", mux.Vars(r))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		transactionReference, ok := vars["id"]
		if !ok {
			web.log.Printf("error: id not in mux.Vars (vars: %v)", mux.Vars(r))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}

		data := struct {
			PageTitle   string
			Transaction db.WRTransaction
			LineItems   []viewLineItem
			ID          string
			TabType     string
		}{
			PageTitle: fmt.Sprintf("Bank Transaction %s", transactionReference),
			ID:        transactionReference,
			TabType:   "link", // by default the donations tab type is "link", not "find"
		}

		var err error
		var lineItems []db.WRLineItem
		data.Transaction, lineItems, err = web.db.BankTransactionWRGet(ctx, transactionReference)
		if err != nil && err != sql.ErrNoRows {
			web.log.Printf("error: database GetTransaction failed: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		data.LineItems = newViewLineItems(lineItems)

		// Return a 404 if no bank transaction was found.
		if errors.Is(err, sql.ErrNoRows) {
			web.log.Printf("Bank Transaction: %q not found", transactionReference)
			http.Error(w, fmt.Sprintf("Bank Transaction %q not found", transactionReference), http.StatusNotFound)
			return
		}

		err = templates.ExecuteTemplate(w, name, data)
		if err != nil {
			web.log.Printf("Error rendering template %s: %v", name, err)
			http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
		}
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

		vars := mux.Vars(r)
		if vars == nil {
			web.log.Printf("error: linked donations vars capture (vars: %v)", mux.Vars(r))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		typer, ok := vars["type"]
		if !ok {
			web.log.Printf("error: type not in mux.Vars (vars: %v)", mux.Vars(r))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		id, ok := vars["id"]
		if !ok {
			web.log.Printf("error: id not in mux.Vars (vars: %v)", mux.Vars(r))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}

		// Initialise pagination for default state.
		// Todo: fix page number (here 1)
		pagination, _ := NewPagination(pageLen, 1, 1, r.URL.Query())

		// Prepare data for the template.
		data := struct {
			ID            string
			Typer         string
			ViewDonations []viewDonation
			Pagination    *Pagination
			TabType       string
		}{
			ID:         id,
			Typer:      typer,
			Pagination: pagination,
			TabType:    "link",
		}

		donations, err := web.db.DonationsGet(
			ctx,
			web.defaultStartDate,
			web.defaultEndDate,
			"Linked", // linkage status
			id,       // payout reference
			"",       // searchstring
			pageLen,
			0, // form offset
		)
		if err != nil && err != sql.ErrNoRows {
			web.log.Printf("error: database GetDonations failed: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
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
			web.log.Printf("pagination error: %v", err)
		}

		err = templates.ExecuteTemplate(w, name, data)
		if err != nil {
			web.log.Printf("Error rendering template %s: %v", name, err)
			http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
		}
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

		vars := mux.Vars(r)
		if vars == nil {
			web.log.Printf("error: linked donations vars capture (vars: %v)", mux.Vars(r))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		typer, ok := vars["type"]
		if !ok {
			web.log.Printf("error: type not in mux.Vars (vars: %v)", mux.Vars(r))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		id, ok := vars["id"]
		if !ok {
			web.log.Printf("error: id not in mux.Vars (vars: %v)", mux.Vars(r))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}

		form := NewSearchDonationsForm()
		if err := DecodeURLParams(r, form); err != nil {
			web.log.Printf("error: could not decode the url parameters: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
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
			PageTitle     string
			ID            string
			Typer         string
			ViewDonations []viewDonation
			Form          *SearchDonationsForm
			Validator     *Validator
			Pagination    *Pagination

			PageType string
			GetURL   string
			TabType  string
		}{
			PageTitle:  "Donations",
			ID:         id,
			Typer:      typer,
			Form:       form,
			Validator:  validator,
			Pagination: pagination,
			PageType:   "indirect", // indirect pages are htmx pages that have an hx target
			GetURL:     fmt.Sprintf("/partials/donations-find/%s/%s", typer, id),
			TabType:    "find",
		}

		// Render template with errors and return if the form is invalid.
		if !validator.Valid() {
			err := templates.ExecuteTemplate(w, name, data)
			if err != nil {
				web.log.Printf("Error rendering template %s: %v", name, err)
				http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
			}
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
			log.Printf("error: database GetDonations failed: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
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
			log.Printf("pagination error: %v", err)
		}

		err = templates.ExecuteTemplate(w, name, data)
		if err != nil {
			web.log.Printf("Error rendering template %s: %v", name, err)
			http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
		}
	})
}
