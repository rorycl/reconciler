package web

import (
	"context"
	"errors"
	"fmt"
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
			ListenAddress: "127.0.0.1:8080",
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

// TestDonationSearchTimeSpan tests the dates around which a donation should be searched
// for in relation to an invoice or bank transaction.
func TestDonationSearchTimeSpan(t *testing.T) {
	tests := []struct {
		dt    time.Time
		start time.Time
		end   time.Time
	}{
		{
			dt:    time.Time{},
			start: time.Time{},
			end:   time.Time{},
		},
		{
			dt:    time.Date(2026, 07, 01, 12, 0, 0, 0, time.UTC),
			start: time.Date(2026, 05, 20, 12, 0, 0, 0, time.UTC),
			end:   time.Date(2026, 07, 15, 12, 0, 0, 0, time.UTC),
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("time_test_%d", ii), func(t *testing.T) {

			s, e := donationSearchTimeSpan(tt.dt)
			if got, want := s, tt.start; got != want {
				t.Errorf("start got %v want %v", got, want)
			}
			if got, want := e, tt.end; got != want {
				t.Errorf("end got %v want %v", got, want)
			}

		})
	}
}
