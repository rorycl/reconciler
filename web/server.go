package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"log/slog"
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
	log        *slog.Logger
	cfg        *config.Config
	db         *db.DB
	staticFS   fs.FS // the fs holding the static web resources.
	templateFS fs.FS // the fs holding the web templates.
	templates  map[string]*template.Template
}

// New initialises a WebApp. An error type is returned for future use.
func New(logger *slog.Logger, cfg *config.Config, db *db.DB, staticFS, templateFS fs.FS) (*WebApp, error) {
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
	web.log.Info(fmt.Sprintf("Starting server on %s", web.cfg.Web.ListenAddress))
	return srv.ListenAndServe()
}

// routes connects all of the WebApp routes and provides middleware.
func (web *WebApp) routes() http.Handler {

	r := mux.NewRouter()

	fs := http.FileServerFS(web.staticFS)
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", fs))

	r.Handle("/", web.handleRoot())
	r.Handle("/connect", web.handleConnect())
	/*
		r.Handle("/refresh", web.handleRefresh)

		r.Handle("/invoices", web.handleInvoices)
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

// handleConnect deals with the /connect endpoint.
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
