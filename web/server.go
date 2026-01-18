package main

import (
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	// linked subapp via go.work
	"dbquery"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

// all vars to be moved to settings.
var (

	// line item account codes
	// var accountCodes = "^4[1234].*"
	accountCodes = "^(53|55|57)"

	// database connection
	db *dbquery.DB

	// pageLen is the number of items listed on a page
	pageLen = 15
)

// pageNo calculates the number of pages in a database result set.
func pageNo(recNo int) int {
	return ((recNo - 1) / pageLen) + 1
}

// default start and end date for searches
// Todo: it's better to use nill values to map to SQL NULLS.
var defaultStartDate = time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC)
var defaultEndDate = time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC)

const inDevelopment bool = true

// horrible global to remove
var mounts *fileMounts

func main() {

	if len(os.Args) != 2 {
		fmt.Println("database needed as an argument")
		os.Exit(1)
	}
	dbFile := os.Args[1]

	var err error
	mounts, err = makeMounts(inDevelopment)
	if err != nil {
		fmt.Printf("mounts err: %v\n", err)
		os.Exit(1)
	}

	var server *http.Server

	db, err = dbquery.New(dbFile, mounts.sqlDir, accountCodes)
	if err != nil {
		fmt.Println("database setup error", err)
		os.Exit(1)
	}

	for {

		server = &http.Server{Addr: "127.0.0.1:8080"}
		r := mux.NewRouter()

		// Serve static files (css, js) from the 'static' directory.
		fs := http.FileServer(http.FS(mounts.static))
		r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", fs))

		// Intro pages.
		r.HandleFunc("/", handleRedirectToConnect)
		r.HandleFunc("/connect", handleConnect)
		r.HandleFunc("/refresh", handleRefresh)
		r.HandleFunc("/home", handleHome) // will also serve /invoices

		// Main listing pages.
		r.HandleFunc("/invoices", handleInvoices)
		r.HandleFunc("/bank-transactions", handleBankTransactions)
		r.HandleFunc("/donations", handleDonations)

		// An invoice is identified by a uuid in Xero, a simple word in test data.
		r.HandleFunc("/invoice/{id:[A-Za-z0-9_-]+}", handleInvoiceDetail)
		r.HandleFunc("/bank-transaction/{id:[A-Za-z0-9_-]+}", handleBankTransactionDetail)

		// partials for the invoice and bank-transaction detail pages.
		// Linked donations deals with both invoices and
		// bank-transactions. (?:...) is a non capturing group.
		r.HandleFunc("/partials/donations-linked/{type:(?:invoice|bank-transaction)}/{id}", handlePartialDonationsLinked)
		r.HandleFunc("/partials/donations-find/{type:(?:invoice|bank-transaction)}/{id}", handlePartialDonationsFind)

		logging := func(handler http.Handler) http.Handler {
			return handlers.CombinedLoggingHandler(os.Stdout, handler)
		}
		r.Use(logging)

		// Attach router to server handler.
		server.Handler = r

		log.Println("Starting server on 127.0.0.1:8080")
		err = server.ListenAndServe()
		if err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("server error: %s\n", err)
			}
			fmt.Println("shutting down server")
			return
		}
	}
}

// http handlers

// handleRedirectToConnect provides a default route to the start page.
func handleRedirectToConnect(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/connect", http.StatusFound)
}

// handleConnect serves the initial OAuth connection page.
func handleConnect(w http.ResponseWriter, r *http.Request) {
	templates := []string{"base.html", "connect.html"}
	renderTemplate(w, "connect", templates, nil)
}

// handleRefresh serves the data refresh page.
func handleRefresh(w http.ResponseWriter, r *http.Request) {
	templates := []string{"base.html", "refresh.html"}
	renderTemplate(w, "refresh", templates, nil)
}

// handleHome serves the main dashboard view, which by default is the invoice list.
func handleHome(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/invoices", http.StatusFound)
}

// handleInvoices serves the invoices list.
func handleInvoices(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()
	templates := []string{"base.html", "partial-listingTabs.html", "invoices.html"}

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
		Invoices    []dbquery.Invoice
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
		renderTemplate(w, "invoices", templates, data)
		return
	}

	invoices, err := db.GetInvoices(
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

	renderTemplate(w, "invoices", templates, data)
}

// handleBankTransactions serves the transactions list.
func handleBankTransactions(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()
	templates := []string{"base.html", "partial-listingTabs.html", "bank_transactions.html"}

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
		BankTransactions []dbquery.BankTransaction
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
		renderTemplate(w, "transactions", templates, data)
		return
	}

	transactions, err := db.GetBankTransactions(
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

	renderTemplate(w, "bank_transactions", templates, data)
}

// handleDonations serves the transactions list.
func handleDonations(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()
	templates := []string{"base.html", "partial-listingTabs.html", "partial-donations-searchform.html", "partial-donations-searchresults.html", "donations.html"}

	form := NewSearchDonationsForm()
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
		renderTemplate(w, "donations", templates, data)
		return
	}

	donations, err := db.GetDonations(
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
		http.Redirect(w, r, "/donations", http.StatusFound)
	}

	renderTemplate(w, "donations", templates, data)
}

// handleInvoiceDetail serves the detail page for a single invoice.
func handleInvoiceDetail(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()
	templates := []string{"base.html", "partial-listingTabs.html", "partial-donations-tabs.html", "invoice.html"}

	vars := mux.Vars(r)
	if vars == nil {
		log.Printf("error: invoiceID capture (vars: %v)", mux.Vars(r))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
	invoiceID, ok := vars["id"]
	if !ok {
		log.Printf("error: id not in mux.Vars (vars: %v)", mux.Vars(r))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}

	data := struct {
		PageTitle string
		Invoice   dbquery.WRInvoice
		LineItems []viewLineItem
		ID        string
		TabType   string
	}{
		PageTitle: fmt.Sprintf("Invoice %s", invoiceID),
		ID:        invoiceID,
		TabType:   "link", // by default the donations tab type is "link", not "find"
	}

	var err error
	var lineItems []dbquery.WRLineItem
	data.Invoice, lineItems, err = db.GetInvoiceWR(ctx, invoiceID)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("error: database GetInvoices failed: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data.LineItems = newViewLineItems(lineItems)

	// Return a 404 if no invoice was found.
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, fmt.Sprintf("Invoice %q not found", invoiceID), http.StatusNotFound)
		return
	}

	renderTemplate(w, "invoice", templates, data)
}

// handleBankTransactionDetail serves the detail page for a single bank
// transaction.
func handleBankTransactionDetail(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()
	templates := []string{"base.html", "partial-listingTabs.html", "partial-donations-tabs.html", "bank-transaction.html"}

	vars := mux.Vars(r)
	if vars == nil {
		log.Printf("error: bank transaction capture (vars: %v)", mux.Vars(r))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
	transactionReference, ok := vars["id"]
	if !ok {
		log.Printf("error: id not in mux.Vars (vars: %v)", mux.Vars(r))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}

	data := struct {
		PageTitle   string
		Transaction dbquery.WRTransaction
		LineItems   []viewLineItem
		ID          string
		TabType     string
	}{
		PageTitle: fmt.Sprintf("Bank Transaction %s", transactionReference),
		ID:        transactionReference,
		TabType:   "link", // by default the donations tab type is "link", not "find"
	}

	var err error
	var lineItems []dbquery.WRLineItem
	data.Transaction, lineItems, err = db.GetTransactionWR(ctx, transactionReference)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("error: database GetTransaction failed: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data.LineItems = newViewLineItems(lineItems)

	// Return a 404 if no bank transaction was found.
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, fmt.Sprintf("Bank Transaction %q not found", transactionReference), http.StatusNotFound)
		return
	}

	renderTemplate(w, "bank-transaction", templates, data)
}

// handlePartialDonationsLinked is the partial htmx endpoint for
// rendering the list of donations linked to an Invoice or Bank
// Transaction.
func handlePartialDonationsLinked(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()
	templates := []string{"partial-donations-tabs.html", "partial-donations-linked.html"}

	vars := mux.Vars(r)
	if vars == nil {
		log.Printf("error: linked donations vars capture (vars: %v)", mux.Vars(r))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
	typer, ok := vars["type"]
	if !ok {
		log.Printf("error: type not in mux.Vars (vars: %v)", mux.Vars(r))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
	id, ok := vars["id"]
	if !ok {
		log.Printf("error: id not in mux.Vars (vars: %v)", mux.Vars(r))
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

	donations, err := db.GetDonations(
		ctx,
		defaultStartDate,
		defaultEndDate,
		"Linked", // linkage status
		id,       // payout reference
		"",       // searchstring
		pageLen,
		0, // form offset
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
	// Todo: fix url.query() if it doesn't have the "tab"
	// Todo: fix page number (here 1)
	data.Pagination, err = NewPagination(pageLen, recordsNo, 1, r.URL.Query())
	if err != nil {
		log.Printf("pagination error: %v", err)
	}

	renderTemplate(w, "partial-donations-linked", templates, data)
}

// handlePartialDonationsFind is the partial htmx endpoint for finding
// donations to link to an Invoice or Bank Transaction.
func handlePartialDonationsFind(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()
	templates := []string{"partial-donations-tabs.html", "partial-donations-searchform.html", "partial-donations-searchresults.html", "partial-donations.html"}

	vars := mux.Vars(r)
	if vars == nil {
		log.Printf("error: linked donations vars capture (vars: %v)", mux.Vars(r))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
	typer, ok := vars["type"]
	if !ok {
		log.Printf("error: type not in mux.Vars (vars: %v)", mux.Vars(r))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
	id, ok := vars["id"]
	if !ok {
		log.Printf("error: id not in mux.Vars (vars: %v)", mux.Vars(r))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}

	form := NewSearchDonationsForm()
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
		renderTemplate(w, "partial-donations", templates, data)
		return
	}

	donations, err := db.GetDonations(
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

	renderTemplate(w, "partial-donations", templates, data)

}

// viewDonation  is a view version of the dbquery.Donations type,
// with non-pointer fields.
type viewDonation struct {
	ID              string
	Name            string
	Amount          float64
	CloseDateStr    string
	PayoutReference any // string or specific web-safe template.HTML
	CreatedDateStr  string
	CreatedName     string
	ModifiedDateStr string
	ModifiedName    string
	IsLinked        bool
	RowCount        int
}

func newViewDonations(donations []dbquery.Donation) []viewDonation {
	dv := make([]viewDonation, len(donations))
	for i, d := range donations {
		dv[i].ID = d.ID
		dv[i].Name = d.Name
		dv[i].Amount = d.Amount
		dv[i].IsLinked = d.IsLinked
		dv[i].RowCount = d.RowCount
		// de-pointer
		if d.PayoutReference == nil {
			dv[i].PayoutReference = template.HTML("&mdash;")
		} else {
			dv[i].PayoutReference = *d.PayoutReference
		}
		if d.CloseDate != nil {
			dv[i].CloseDateStr = d.CloseDate.Format("02/01/2006")
		}
		if d.CreatedDate != nil {
			dv[i].CreatedDateStr = d.CreatedDate.Format("02/01/2006")
		}
		if d.ModifiedDate != nil {
			dv[i].ModifiedDateStr = d.ModifiedDate.Format("02/01/2006")
		}
		if d.CreatedName != nil {
			dv[i].CreatedName = *d.CreatedName
		}
		if d.ModifiedName != nil {
			dv[i].ModifiedName = *d.ModifiedName
		}
	}
	return dv
}

// viewLineItems is a view version of the dbquery.WRLineItem with
// non-pointer fields.
type viewLineItem struct {
	AccountCode    string
	AccountName    string
	Description    string
	TaxAmount      float64
	LineAmount     float64
	DonationAmount float64
}

// newViewLineItems converts a slice of WRLineItem to a slice of
// viewLineItem.
func newViewLineItems(lineItems []dbquery.WRLineItem) []viewLineItem {
	viewItems := make([]viewLineItem, len(lineItems))
	for i, li := range lineItems {
		if li.AccountCode != nil {
			viewItems[i].AccountCode = *li.AccountCode
		}
		if li.AccountName != nil {
			viewItems[i].AccountName = *li.AccountName
		}
		if li.Description != nil {
			viewItems[i].Description = *li.Description
		}
		if li.TaxAmount != nil {
			viewItems[i].TaxAmount = *li.TaxAmount
		}
		if li.LineAmount != nil {
			viewItems[i].LineAmount = *li.LineAmount
		}
		if li.DonationAmount != nil {
			viewItems[i].DonationAmount = *li.DonationAmount
		}
	}
	return viewItems
}

// helpers

// renderTemplate is a helper to execute templates and handle errors.
func renderTemplate(w http.ResponseWriter, name string, templates []string, data any) {
	templateFS := mounts.templates
	tpl := template.Must(template.ParseFS(templateFS, templates...))
	err := tpl.ExecuteTemplate(w, name+".html", data)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
		log.Printf("Error rendering template %s: %v", name, err)
	}
}
