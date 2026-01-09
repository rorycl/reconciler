package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"

	// linked subapp via go.work
	"dbquery"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

type Invoice struct {
	UUID            string
	InvoiceNumber   string
	Date            time.Time
	ContactName     string
	Description     string
	Total           float64
	DonationsTotal  float64
	IsReconciled    bool
	LinkedDonations []Donation
	LineItems       []LineItem // faked for the moment.
}

type LineItem struct {
	Description string
	UnitAmount  float64
	AccountCode string
	LineItemID  string
	Quantity    float64
	TaxAmount   float64
	LineAmount  float64
}

func (l *LineItem) IsDonation() bool {
	if len(l.AccountCode) > 0 && string(l.AccountCode[0]) == "4" {
		return true
	}
	return false
}

type Donation struct {
	UUID        string
	Date        time.Time
	ContactName string
	Campaign    string
	Description string
	Amount      float64
	PayoutRef   string
}

func rebuildTailwind() error {
	log.Println("rebulding tailwind")
	cmdArgs := strings.Split(`tailwindcss-linux-x64-v4.0.7 -i static/css/input.css -o static/css/output.css`, " ")
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	out, err := cmd.CombinedOutput()
	log.Println(string(out))
	return err
}

// line item account codes
var accountCodes = "^(53|55|57)"

// database connection
var db *dbquery.DB

// pageLen is the number of items listed on a page
var pageLen = 20

func main() {

	var server *http.Server
	var err error

	db, err = dbquery.New("../dbquery/testdata/test.db", "../dbquery/sql", accountCodes)
	if err != nil {
		fmt.Println("database setup error", err)
		os.Exit(1)
	}

	// The filewatcher watches for file changes.
	filewatcher, err := NewFileChangeNotifier(
		[]DirFilesDescriptor{
			DirFilesDescriptor{"templates", []string{"html"}},
			DirFilesDescriptor{"static/css", []string{"css"}},
			DirFilesDescriptor{"static/js", []string{"js"}},
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		err := filewatcher.Watch(context.Background())
		if err != nil {
			log.Fatal("watcher error", err)
		}
	})
	go func() {
		wg.Wait()
	}()
	updater := filewatcher.Update()

	for {

		// rebuild tailwind
		err := rebuildTailwind()
		if err != nil {
			log.Fatalf("tailwind rebuild failed: %s", err)
		}

		server = &http.Server{Addr: "127.0.0.1:8080"}
		r := mux.NewRouter()

		// Serve static files (CSS, JS) from the 'static' directory.
		fs := http.FileServer(http.Dir("./static"))
		r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", fs))

		// Register page handlers.
		r.HandleFunc("/connect", handleConnect)
		r.HandleFunc("/refresh", handleRefresh)
		r.HandleFunc("/home", handleHome) // will also serve /invoices
		r.HandleFunc("/invoice", handleInvoiceDetail)

		r.HandleFunc("/", handleRedirectToConnect)

		logging := func(handler http.Handler) http.Handler {
			return handlers.CombinedLoggingHandler(os.Stdout, handler)
		}
		r.Use(logging)

		// Attach router to server handler.
		server.Handler = r

		go func() {
			log.Println("Starting server on 127.0.0.1:8080")
			err := server.ListenAndServe()
			if err != nil {
				if !errors.Is(err, http.ErrServerClosed) {
					log.Fatalf("server error: %s\n", err)
				}
				fmt.Println("shutting down server")
				return
			}
		}()
		select {
		case <-updater:
			log.Println("file update detected")
			_ = server.Shutdown(context.Background())
			break
		}
		log.Println("restarting server")
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
	templates := []string{"templates/base.html", "templates/connect.html"}
	renderTemplate(w, "connect", templates, nil)
}

// handleRefresh serves the data refresh page.
func handleRefresh(w http.ResponseWriter, r *http.Request) {
	templates := []string{"templates/base.html", "templates/refresh.html"}
	renderTemplate(w, "refresh", templates, nil)
}

// handleHome serves the main dashboard view, which is the invoice list.
func handleHome(w http.ResponseWriter, r *http.Request) {
	// This data would come from a database query based on filters.

	ctx := r.Context()

	type Args struct {
		ReconciliationStatus string
		DateFrom, DateTo     time.Time
		SearchString         string
		Page, Limit, Offset  int
	}

	stringRepr := func(a Args) string {
		return fmt.Sprintf("%#v\n", a)
	}

	a := validation(r)
	log.Printf("ARGS: %#v", stringRepr(*a))

	invoices, err := db.GetInvoices(ctx, a.ReconciliationStatus, a.DateFrom, a.DateTo, a.SearchString, a.Limit, a.Offset)
	if err != nil {
		log.Printf("invoices get error %v", err)
	}

	log.Println("invoices len:", len(invoices))

	data := struct {
		PageTitle string
		Invoices  []dbquery.Invoice
		Args      Args
	}{
		PageTitle: "Home",
		Invoices:  invoices,
		Args:      *a,
	}

	templates := []string{"templates/base.html", "templates/home.html"}
	renderTemplate(w, "home", templates, data)
}

// handleInvoiceDetail serves the detail page for a single invoice.
func handleInvoiceDetail(w http.ResponseWriter, r *http.Request) {
	// Simple mock routing based on the URL path segment.
	invoiceID := path.Base(r.URL.Path)

	var invoice Invoice
	if invoiceID == "reconciled-example" {
		invoice = getDummyReconciledInvoice()
	} else {
		// Default to the unreconciled view for any other ID.
		invoice = getDummyUnreconciledInvoice()
	}

	data := struct {
		PageTitle string
		Invoice   Invoice
		Donations []Donation // For the "find donations" search result mock
		LineItems []Invoice
	}{
		PageTitle: fmt.Sprintf("Invoice %s", invoice.InvoiceNumber),
		Invoice:   invoice,
		Donations: getDummySearchDonations(),
		LineItems: getDummyInvoices(),
	}
	templates := []string{"templates/base.html", "templates/invoice.html"}
	renderTemplate(w, "invoice", templates, data)
}

// helpers

// renderTemplate is a helper to execute templates and handle errors.
func renderTemplate(w http.ResponseWriter, name string, templates []string, data any) {
	tpl := template.Must(template.ParseFiles(templates...))
	err := tpl.ExecuteTemplate(w, name+".html", data)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error rendering template %s: %v", name, err), http.StatusInternalServerError)
		log.Printf("Error rendering template %s: %v", name, err)
	}
}

// Dummy data providers
// These functions simulate fetching data from a database.

func getDummyInvoices() []Invoice {
	return []Invoice{
		{
			UUID:           "unreconciled-example",
			InvoiceNumber:  "INV-0234",
			Date:           time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
			ContactName:    "James Galway",
			Description:    "James Galway (family) Donation",
			Total:          117.48,
			DonationsTotal: 0.00,
			IsReconciled:   false,
		},
		{
			UUID:           "reconciled-example",
			InvoiceNumber:  "INV-0236",
			Date:           time.Date(2025, 7, 3, 0, 0, 0, 0, time.UTC),
			ContactName:    "Julie Joyce",
			Description:    "STO J. Joyce Standing Order",
			Total:          50.00,
			DonationsTotal: 50.00,
			IsReconciled:   true,
		},
	}
}

func getDummyUnreconciledInvoice() Invoice {
	return Invoice{
		UUID:            "d4754673-da9f-11f0-b492-8c16455f785b",
		InvoiceNumber:   "INV-0234",
		Date:            time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
		ContactName:     "James Galway",
		Description:     "James Galway (family) Donation as set out in the agreement...",
		Total:           117.20,
		DonationsTotal:  0.00,
		IsReconciled:    false,
		LinkedDonations: []Donation{}, // Empty slice
		LineItems: []LineItem{
			LineItem{
				Description: "Stripe donation",
				UnitAmount:  200.00,
				AccountCode: "412",
				LineItemID:  "eab3ce9f-dd31-11f0-8de1-8c16455f785b",
				Quantity:    1,
				TaxAmount:   0.00,
				LineAmount:  200.00,
			},
			LineItem{
				Description: "Stripe platform fees",
				UnitAmount:  -2.80,
				AccountCode: "500",
				LineItemID:  "1bf7ffcf-dd32-11f0-b2d3-8c16455f785b",
				Quantity:    1,
				TaxAmount:   0.00,
				LineAmount:  -2.80,
			},
		},
	}
}

func getDummyReconciledInvoice() Invoice {
	linkedDonations := []Donation{
		{
			UUID:        "sf-don-001",
			Date:        time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
			ContactName: "James Galway",
			Campaign:    "Recurring Donor",
			Description: "Recurring monthly gift",
			Amount:      117.20,
			PayoutRef:   "d4754673-da9f-11f0-b492-8c16455f785b",
		},
	}
	return Invoice{
		UUID:            "d4754673-da9f-11f0-b492-8c16455f785b",
		InvoiceNumber:   "INV-0234",
		Date:            time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
		ContactName:     "James Galway",
		Description:     "James Galway (family) Donation as set out in the agreement...",
		Total:           117.20,
		DonationsTotal:  117.20,
		IsReconciled:    true,
		LinkedDonations: linkedDonations,
		LineItems: []LineItem{
			LineItem{
				Description: "Stripe donation",
				UnitAmount:  200.00,
				AccountCode: "412",
				LineItemID:  "eab3ce9f-dd31-11f0-8de1-8c16455f785b",
				Quantity:    1,
				TaxAmount:   0.00,
				LineAmount:  200.00,
			},
			LineItem{
				Description: "Stripe platform fees",
				UnitAmount:  -2.80,
				AccountCode: "500",
				LineItemID:  "1bf7ffcf-dd32-11f0-b2d3-8c16455f785b",
				Quantity:    1,
				TaxAmount:   0.00,
				LineAmount:  -2.80,
			},
		},
	}
}

func getDummySearchDonations() []Donation {
	return []Donation{
		{
			UUID:        "sf-don-001",
			Date:        time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
			ContactName: "James Galway",
			Campaign:    "Recurring Donor",
			Description: "James Galway (family) Donation",
			Amount:      117.20,
		},
		{
			UUID:        "sf-don-002",
			Date:        time.Date(2025, 7, 3, 0, 0, 0, 0, time.UTC),
			ContactName: "Galway Taxis",
			Campaign:    "Property Dinner 2025",
			Description: "Galway Taxi Services: one off donation for the...",
			Amount:      1250.00,
		},
	}
}
