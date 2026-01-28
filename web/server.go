package web

import (
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"reconciler/config"
	"reconciler/db"

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
	log        *log.Logger
	cfg        *config.Config
	db         *db.DB
	staticFS   fs.FS // the fs holding the static web resources.
	templateFS fs.FS // the fs holding the web templates.
	templates  map[string]*template.Template
}

// New initialises a WebApp. An error type is returned for future use.
func New(logger *log.Logger, cfg *config.Config, db *db.DB, staticFS, templateFS fs.FS) (*WebApp, error) {
	webApp := &WebApp{
		log:        logger,
		cfg:        cfg,
		db:         db,
		staticFS:   staticFS,
		templateFS: templateFS,
	}
	return webApp, nil
}

// StartServer starts a WebApp.
func (web *WebApp) StartServer() error {
	srv := &http.Server{
		Addr:    web.cfg.Web.ListenAddress,
		Handler: web.routes(),
	}
	log.Println(fmt.Sprintf("Starting server on %s", web.cfg.Web.ListenAddress))
	return srv.ListenAndServe()
}

// routes connects all of the WebApp routes and provides middleware.
func (web *WebApp) routes() http.Handler {

	r := mux.NewRouter()

	fs := http.FileServerFS(web.staticFS)
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", fs))

	r.Handle("/", web.handleRoot())
	r.Handle("/connect", web.handleConnect())
	r.Handle("/refresh", web.handleRefresh())

	r.Handle("/invoices", web.handleInvoices())
	r.Handle("/bank-transactions", web.handleBankTransactions())
	/*
		r.Handle("/invoice/{id:[A-Za-z0-9_-]+}", web.handleInvoiceDetail)

		r.Handle("/bank-transactions", web.handleBankTransactions)
		r.Handle("/bank-transaction/{id:[A-Za-z0-9_-]+}", web.handleBankTransactionDetail)

		r.Handle("/donations", web.handleDonations)

		r.Handle("/partials/donations-linked/{type:(?:invoice|bank-transaction)}/{id}", web.handlePartialDonationsLinked)
		r.Handle("/partials/donations-find/{type:(?:invoice|bank-transaction)}/{id}", web.handlePartialDonationsFind)
	*/

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
			http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
			log.Printf("Error rendering template %s: %v", name, err)
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
			http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
			log.Printf("Error rendering template %s: %v", name, err)
		}
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
			log.Printf("error: could not decode the url parameters: %v", err)
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
				http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
				log.Printf("Error rendering template %s: %v", name, err)
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
			log.Printf("error: database GetInvoices failed: %v", err)
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
			log.Printf("pagination error: %v", err)
			http.Redirect(w, r, "/invoices", http.StatusFound)
		}

		err = templates.ExecuteTemplate(w, name, data)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
			log.Printf("Error rendering template %s: %v", name, err)
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
			log.Printf("error: could not decode the url parameters: %v", err)
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
				http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
				log.Printf("Error rendering template %s: %v", name, err)
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
			log.Printf("error: database GetBankTransactions failed: %v", err)
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
			log.Printf("pagination error: %v", err)
			http.Redirect(w, r, "/banktransactions", http.StatusFound)
		}

		err = templates.ExecuteTemplate(w, name, data)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
			log.Printf("Error rendering template %s: %v", name, err)
		}
	})
}
