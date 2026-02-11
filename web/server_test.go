package web

import (
	"context"
	"errors"
	"log"
	"net/http"
	"reconciler/config"
	"reconciler/db"
	"reconciler/internal"
	"testing"
	"time"
)

// TestWebAppToRun is for running the web server in development.
func TestWebAppToRun(t *testing.T) {

	t.Skip()

	logger := log.Default()

	cfg := &config.Config{
		Web: config.WebConfig{
			ListenAddress: "127.0.0.1:8000",
		},
		Xero: config.XeroConfig{
			TokenFilePath: "xero.json",
		},
		Salesforce: config.SalesforceConfig{
			TokenFilePath: "salesforce.json",
		},
	}
	accountCodes := "^(53|55|57)"
	db, err := db.NewConnectionInTestMode("file::memory:?cache=shared", "", accountCodes)
	if err != nil {
		t.Fatal(err)
	}

	staticFS, err := internal.NewFileMount("static", StaticEmbeddedFS, "")
	if err != nil {
		t.Fatal(err)
	}
	templatesFS, err := internal.NewFileMount("templates", TemplatesEmbeddedFS, "")
	if err != nil {
		t.Fatal(err)
	}

	// testing data start and end dates
	startDate := time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC)

	webApp, err := New(logger, cfg, db, staticFS, templatesFS, startDate, endDate)
	if err != nil {
		t.Fatal(err)
	}
	err = webApp.StartServer()
	if err != nil {
		t.Fatal(err)
	}
}

// TestWebAppAndShutdown tests bringing up the web server and then stopping it.
func TestWebAppAndShutdown(t *testing.T) {

	logger := log.Default()

	cfg := &config.Config{
		Web: config.WebConfig{
			ListenAddress: "127.0.0.1:8000",
		},
		Xero: config.XeroConfig{
			TokenFilePath: "xero.json",
		},
		Salesforce: config.SalesforceConfig{
			TokenFilePath: "sf.json",
		},
	}
	accountCodes := "^(53|55|57)"
	db, err := db.NewConnectionInTestMode("file::memory:?cache=shared", "", accountCodes)
	if err != nil {
		t.Fatal(err)
	}

	staticFS, err := internal.NewFileMount("static", StaticEmbeddedFS, "")
	if err != nil {
		t.Fatal(err)
	}
	templatesFS, err := internal.NewFileMount("templates", TemplatesEmbeddedFS, "")
	if err != nil {
		t.Fatal(err)
	}

	// testing data start and end dates
	startDate := time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC)

	webApp, err := New(logger, cfg, db, staticFS, templatesFS, startDate, endDate)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		select {
		case <-time.After(50 * time.Millisecond):
			_ = webApp.server.Shutdown(context.Background())
		}
	}()

	err = webApp.StartServer()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("server error: %T %v", err, err)
	}
}
