package web

// This file describes the web server for this project.
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

import (
	"bytes"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"reconciler/apiclients/salesforce"
	"reconciler/apiclients/xero"
	"reconciler/config"
	"reconciler/db"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

// pageLen is the number of items to show in a page listing.
const pageLen = 15

//go:embed static
var StaticEmbeddedFS embed.FS

//go:embed templates
var TemplatesEmbeddedFS embed.FS

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
func New(
	logger *log.Logger,
	cfg *config.Config,
	db *db.DB,
	staticFS fs.FS,
	templateFS fs.FS,
	start time.Time,
	end time.Time,
) (*WebApp, error) {
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

	r.Handle(
		"/",
		web.handleRoot(), // synonym for /connect
	)
	r.Handle(
		"/connect",
		web.handleConnect(),
	)
	r.Handle(
		"/refresh",
		web.apisConnectedOK(web.handleRefresh()),
	)
	r.Handle(
		"/home",
		web.apisConnectedOK(web.handleHome()),
	) // redirect to handleInvoices.

	// Main listing pages.
	r.Handle(
		"/invoices",
		web.apisConnectedOK(web.handleInvoices()),
	)
	r.Handle(
		"/bank-transactions",
		web.apisConnectedOK(web.handleBankTransactions()),
	)
	r.Handle(
		"/donations",
		web.apisConnectedOK(web.handleDonations()),
	)
	// Todo: consider adding campaigns page

	// Detail pages.
	// Note that the regexp works for uuids and the system test data.
	r.Handle(
		"/invoice/{id:[A-Za-z0-9_-]+}",
		web.apisConnectedOK(web.handleInvoiceDetail()),
	)
	r.Handle(
		"/bank-transaction/{id:[A-Za-z0-9_-]+}",
		web.apisConnectedOK(web.handleBankTransactionDetail()),
	)
	// Todo: donation detail page.

	// Partial pages.
	// These are HTMX partials showing donation listings in "linked" and "find to link" modes.
	r.Handle(
		"/partials/donations-linked/{type:(?:invoice|bank-transaction)}/{id}",
		web.apisConnectedOK(web.handlePartialDonationsLinked()),
	)
	r.Handle(
		"/partials/donations-find/{type:(?:invoice|bank-transaction)}/{id}",
		web.apisConnectedOK(web.handlePartialDonationsFind()),
	)

	// Donation linking/unlinking.
	r.Handle(
		"/donations/{type:(?:invoice|bank-transaction)}/{id}/{action}",
		web.apisConnectedOK(web.handleDonationsLinkUnlink()),
	)

	logging := handlers.LoggingHandler(os.Stdout, r)
	return logging
}

// apisConnectedOK checks whether the user is connected to the api services. If not, the user is
// redirected to the /connect endpoint.
func (web *WebApp) apisConnectedOK(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xeroConnected := xero.TokenIsValid(web.cfg.Xero.TokenFilePath)
		sfConnected := salesforce.TokenIsValid(web.cfg.Salesforce.TokenFilePath)
		if !xeroConnected || !sfConnected {
			http.Redirect(w, r, "/connect", http.StatusSeeOther)
			return
		}
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
		web.render(w, r, templates, name, nil)
	})
}

// handleRefresh serves the /refresh page.
func (web *WebApp) handleRefresh() http.Handler {

	name := "refresh.html"
	tpls := []string{"base.html", "refresh.html"}
	templates := template.Must(template.ParseFS(web.templateFS, tpls...))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		web.render(w, r, templates, name, nil)
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
			web.log.Printf("pagination error: %v", err)
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

		var lineItems []db.WRLineItem
		data.Invoice, lineItems, err = web.db.InvoiceWRGet(ctx, invoiceID)
		if err != nil && err != sql.ErrNoRows {
			web.ServerError(w, r, err)
			return
		}

		data.LineItems = newViewLineItems(lineItems)

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

		var lineItems []db.WRLineItem
		data.Transaction, lineItems, err = web.db.BankTransactionWRGet(ctx, transactionReference)
		if err != nil && err != sql.ErrNoRows {
			web.ServerError(w, r, err)
			return
		}

		data.LineItems = newViewLineItems(lineItems)

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
			// web.ServerError(w, r, err)
			web.log.Printf("pagination error: %v", err)
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
			log.Printf("pagination error: %v", err)
			// web.ServerError(w, r, err)
		}

		web.render(w, r, templates, name, data)

	})
}

// handleDonationsLinkUnlink links or unlinks donations to either Xero invoices or bank
// transactions.
func (web *WebApp) handleDonationsLinkUnlink() http.Handler {

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
			// Consider reporting the errors better here.
			fmt.Printf("invalid data was received: %v", validator.Errors)
			web.htmxClientError(
				w,
				fmt.Sprintf("%s form error: invalid data was received: %v", vars["action"], validator.Errors))
			return
		}

		// Create the salesforce client and run a batch update.
		sfClient, err := salesforce.NewClient(ctx, web.cfg)
		if err != nil {
			fmt.Printf("failed to create salesforce client for linking/unlinking: %v", err)
			web.ServerError(w, r, fmt.Errorf("failed to create salesforce client for linking/unlinking: %w", err))
			return
		}
		_, err = sfClient.BatchUpdateOpportunityRefs(ctx, form.ID, form.DonationIDs, false)
		if err != nil {
			log.Printf("failed to batch update salesforce records for linking/unlinking: %v", err)
			web.ServerError(w, r, fmt.Errorf("failed to batch update salesforce records for linking/unlinking: %w", err))
			return
		}

		log.Printf("Successfully linked %d donations.", len(form.DonationIDs))

		web.htmxClientError(w, fmt.Sprintf("Successfully linked %d donations.", len(form.DonationIDs)))
		return

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
		web.log.Printf("template %q rendering error %v", filename, err)
		web.ServerError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
	buf.WriteTo(w)
}

// ServerError logs and return an internal server error. The error should contain the
// information needed for logging.
func (web *WebApp) ServerError(w http.ResponseWriter, r *http.Request, errs ...error) {
	err := errors.Join(errs...)
	web.log.Printf(err.Error(), "method", r.Method, "uri", r.URL.RequestURI())
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

// clientError returns a client error.
func (web *WebApp) clientError(w http.ResponseWriter, message string, status int) {
	if message == "" {
		message = http.StatusText(status)
	}
	http.Error(w, message, status)
}

// htmxClientError returns an htmx client error.
func (web *WebApp) htmxClientError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// htmx won't normally process a non-200 response.
	w.WriteHeader(http.StatusOK)
	errorString := fmt.Sprintf(
		`<div class="text-sm text-red px-4 pb-2">%s</div>`,
		message,
	)
	w.Write([]byte(errorString))
}

// notfound raises a 404 clientError.
func (web *WebApp) notFound(w http.ResponseWriter, r *http.Request, message string) {
	web.clientError(w, message, http.StatusNotFound)
}
