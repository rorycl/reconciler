package web

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rorycl/reconciler/config"
	"github.com/rorycl/reconciler/db"
	mounts "github.com/rorycl/reconciler/internal/mounts"

	"golang.org/x/oauth2"
)

// TestWebAppAndShutdown tests bringing up the web server and then stopping it.
func TestWebAppAndShutdown(t *testing.T) {

	serverURL := "127.0.0.1:8000"

	cfg := &config.Config{
		Web: config.WebConfig{
			ListenAddress: serverURL,
		},
		Xero: config.XeroConfig{
			OAuth2Config: &oauth2.Config{
				RedirectURL: "/xero/callback",
				Endpoint: oauth2.Endpoint{
					AuthURL:  fmt.Sprintf("%s/oauth2/authorize", serverURL),
					TokenURL: fmt.Sprintf("%s/oauth2/token", serverURL),
				},
				Scopes: []string{"accounting.transactions", "accounting.settings.read", "offline_access"},
			},
		},
		Salesforce: config.SalesforceConfig{
			OAuth2Config: &oauth2.Config{
				RedirectURL: "/sf/callback",
				Endpoint: oauth2.Endpoint{
					AuthURL:  fmt.Sprintf("%s/oauth2/authorize", serverURL),
					TokenURL: fmt.Sprintf("%s/oauth2/token", serverURL),
				},
				Scopes: []string{"api", "refresh_token"},
			},
		},
		DataStartDate: time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC),
	}

	logger := slog.New(slog.NewTextHandler(
		t.Output(),
		&slog.HandlerOptions{Level: slog.LevelDebug},
	))
	t.Log("logger initialised")

	staticFS, err := mounts.NewFileMount("static", StaticEmbeddedFS, "")
	if err != nil {
		t.Fatal(err)
	}
	templatesFS, err := mounts.NewFileMount("templates", TemplatesEmbeddedFS, "")
	if err != nil {
		t.Fatal(err)
	}
	sqlFS, err := mounts.NewFileMount("sql", db.SQLEmbeddedFS, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("mounts initialised")

	accountCodes := "^(53|55|57)"
	dbPath := "file:memdb_srv?mode=memory&cache=shared&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	db, err := db.NewConnectionInTestMode(dbPath, sqlFS, accountCodes, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("db initialised")

	webApp, err := New(logger, cfg, db, staticFS, templatesFS)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("web app initialised")

	// Check route restarting works (this is useful for live reloading, for example for
	// testing template and static file changes.
	webApp.RestartRoutes()
	t.Log("routes restarted")

	go func() {
		<-time.After(50 * time.Millisecond)
		t.Log("shutdown called")
		_ = webApp.server.Shutdown(context.Background())
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

/*
github.com/rorycl/reconciler/web/server.go:1494:	getInvoiceOrBankTransactionDetails	0.0%
github.com/rorycl/reconciler/web/types.go:27:		newXeroClienter				0.0%
github.com/rorycl/reconciler/web/types.go:42:		newSalesforceClienter			0.0%
*/

func TestServerComponents(t *testing.T) {

	logger := slog.New(slog.NewTextHandler(
		t.Output(),
		&slog.HandlerOptions{Level: slog.LevelDebug},
	))

	webApp := &WebApp{
		inDevelopment: false,
		log:           logger,
	}
	webApp.SetInDevelopment()
	if got, want := webApp.inDevelopment, true; got != want {
		t.Errorf("inDevelopment got %t want %t", got, want)
	}

	// ServerError.
	r := httptest.NewRequest("GET", "/error", nil)
	w := httptest.NewRecorder()

	webApp.ServerError(w, r, errors.New("error1"))

	result := w.Result()
	body, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("result body reading error: %v", err)
	}
	if got, want := result.StatusCode, http.StatusInternalServerError; got != want {
		t.Errorf("got status %d expected %d", got, want)
	}
	if got, want := string(body), "Internal Server Error"; !strings.Contains(got, want) {
		t.Errorf("body: expected %s in %s", want, got)
	}

	// clientError with message.
	w = httptest.NewRecorder()
	webApp.clientError(w, "a bad request message", http.StatusBadRequest)

	result = w.Result()
	body, err = io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("result body reading error: %v", err)
	}
	if got, want := result.StatusCode, http.StatusBadRequest; got != want {
		t.Errorf("got status %d expected %d", got, want)
	}
	if got, want := string(body), "a bad request message"; !strings.Contains(got, want) {
		t.Errorf("body: expected %s in %s", want, got)
	}

	// clientError without message.
	w = httptest.NewRecorder()
	webApp.clientError(w, "", http.StatusBadRequest)

	result = w.Result()
	body, err = io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("result body reading error: %v", err)
	}
	if got, want := result.StatusCode, http.StatusBadRequest; got != want {
		t.Errorf("got status %d expected %d", got, want)
	}
	if got, want := string(body), http.StatusText(result.StatusCode); !strings.Contains(got, want) {
		t.Errorf("body: expected %s in %s", want, got)
	}

	// htmxClientError.
	w = httptest.NewRecorder()
	webApp.htmxClientError(w, "htmx client error")

	result = w.Result()
	body, err = io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("result body reading error: %v", err)
	}
	if got, want := result.StatusCode, http.StatusOK; got != want { // 200 status needed for htmx.
		t.Errorf("got status %d expected %d", got, want)
	}
	if got, want := string(body), "htmx client error"; !strings.Contains(got, want) {
		t.Errorf("body: expected %s in %s", want, got)
	}

	// notFound.
	r = httptest.NewRequest("GET", "/not/found", nil)
	w = httptest.NewRecorder()
	webApp.notFound(w, r, fmt.Sprintf("%q was not found", r.URL.Path))

	result = w.Result()
	body, err = io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("result body reading error: %v", err)
	}
	if got, want := result.StatusCode, http.StatusNotFound; got != want { // 404
		t.Errorf("got status %d expected %d", got, want)
	}
	if got, want := string(body), `was not found`; !strings.Contains(got, want) {
		t.Errorf("body: expected %q in %q", want, got)
	}

	// render ok.
	r = httptest.NewRequest("GET", "/hi/name", nil)
	w = httptest.NewRecorder()
	tpl := template.Must(template.New("t").Parse("hi {{ .Name }}"))

	webApp.render(w, r, tpl, "t", map[string]string{"Name": "emaN"})

	result = w.Result()
	body, err = io.ReadAll(result.Body)
	if got, want := result.StatusCode, http.StatusOK; got != want {
		t.Errorf("got status %d expected %d", got, want)
	}
	if got, want := string(body), "hi emaN"; !strings.Contains(got, want) {
		t.Errorf("body: expected %s in %s", want, got)
	}

	// render error.
	r = httptest.NewRequest("GET", "/hi/error", nil)
	w = httptest.NewRecorder()
	tpl = template.Must(template.New("t").Parse("hi {{ .XXX }}"))

	webApp.render(w, r, tpl, "t", struct{}{}) // trigger a template error

	result = w.Result()
	// body, err = io.ReadAll(result.Body)
	// fmt.Println(string(body))
	if got, want := result.StatusCode, http.StatusInternalServerError; got != want {
		t.Errorf("got status %d expected %d", got, want)
	}

}
