package web

import (
	"context"
	"encoding/gob"
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

	"github.com/alexedwards/scs/v2"
	"github.com/rorycl/reconciler/config"
	"github.com/rorycl/reconciler/db"
	mounts "github.com/rorycl/reconciler/internal/mounts"
	"github.com/rorycl/reconciler/internal/token"

	"golang.org/x/oauth2"
)

// TestWebAppAndShutdown tests bringing up the web server and then stopping it.
func TestWebAppAndShutdown(t *testing.T) {

	serverURL := "localhost:8000"

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

	webApp.SetInDevelopment()
	if !webApp.inDevelopment {
		t.Error("inDevelopment false after SetInDevelopment")
	}

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

// TestServerHandlers tests the main server handlers.
func TestServerHandlers(t *testing.T) {

	serverURL := "localhost:8000"

	// Register types for scs.
	gob.Register(time.Time{})
	gob.Register(token.ExtendedToken{})
	sessionStore := scs.New()
	sessionStore.Lifetime = 1 * time.Hour

	/*
		ctx, err := sessionStore.Load(context.Background(), "")
		if err != nil {
			t.Fatalf("could not load session store: %v", err)
		}
	*/

	cfg := &config.Config{
		Web: config.WebConfig{ListenAddress: serverURL},
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

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

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

	accountCodes := "^(53|55|57)"
	dbPath := "file:memdb_srv2?mode=memory&cache=shared&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	db, err := db.NewConnectionInTestMode(dbPath, sqlFS, accountCodes, logger)
	if err != nil {
		t.Fatal(err)
	}

	webApp, err := New(logger, cfg, db, staticFS, templatesFS)
	if err != nil {
		t.Fatal(err)
	}

	webApp.sessions = sessionStore
	webApp.SetInDevelopment()

	router := webApp.routes()
	ts := httptest.NewServer(router)
	defer ts.Close()

	client := ts.Client()

	paths := []string{
		"/connect",
		"/refresh",
		"/invoices",
		"/bank-transactions",
		"/donations",
		"/invoice/inv-001/link",
		"/bank-transaction/bt-001/unlink",
		"/logout",
	}

	for _, path := range paths {

		resp, err := client.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("Failed to make request to %s: %v", path, err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		if resp.StatusCode != 200 {
			t.Errorf("%s got unexpected status code %d", path, resp.StatusCode)
			fmt.Println(string(body))
		}
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

// TestServerComponents tests some of the support components such as error and render
// methods.
func TestServerComponents(t *testing.T) {

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name       string
		testFunc   func(a *WebApp, w http.ResponseWriter, r *http.Request)
		wantStatus int
		wantBody   string
	}{
		{
			name: "ServerError",
			testFunc: func(a *WebApp, w http.ResponseWriter, r *http.Request) {
				a.ServerError(w, r, errors.New("error1"))
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "Internal Server Error",
		},
		{
			name: "clientError",
			testFunc: func(a *WebApp, w http.ResponseWriter, r *http.Request) {
				a.clientError(w, "a bad request message", http.StatusBadRequest)
			},
			wantStatus: http.StatusBadRequest,
			wantBody:   "a bad request message",
		},
		{
			name: "clientError without message",
			testFunc: func(a *WebApp, w http.ResponseWriter, r *http.Request) {
				a.clientError(w, "", http.StatusBadRequest)
			},
			wantStatus: http.StatusBadRequest,
			wantBody:   http.StatusText(http.StatusBadRequest),
		},
		{
			name: "htmx client error",
			testFunc: func(a *WebApp, w http.ResponseWriter, r *http.Request) {
				a.htmxClientError(w, "htmx client error")
			},
			wantStatus: http.StatusOK,
			wantBody:   "htmx client error",
		},
		{
			name: "not found error",
			testFunc: func(a *WebApp, w http.ResponseWriter, r *http.Request) {
				a.notFound(w, r, fmt.Sprintf("%q was not found", r.URL.Path))
			},
			wantStatus: http.StatusNotFound,
			wantBody:   "was not found",
		},
		{
			name: "render ok",
			testFunc: func(a *WebApp, w http.ResponseWriter, r *http.Request) {
				tpl := template.Must(template.New("t").Parse("hi {{ .Name }}"))
				a.render(w, r, tpl, "t", map[string]string{"Name": "emaN"})
			},
			wantStatus: http.StatusOK,
			wantBody:   "hi emaN",
		},
		{
			name: "render fail",
			testFunc: func(a *WebApp, w http.ResponseWriter, r *http.Request) {
				tpl := template.Must(template.New("t").Parse("hi {{ .XXX }}"))
				a.render(w, r, tpl, "t", struct{}{}) // trigger a template error
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "",
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("%d_%s", ii, tt.name), func(t *testing.T) {

			a := &WebApp{
				inDevelopment: false,
				log:           logger,
			}

			r := httptest.NewRequest("GET", "/", nil)
			w := httptest.NewRecorder()

			tt.testFunc(a, w, r)

			result := w.Result()
			body, err := io.ReadAll(result.Body)
			if err != nil {
				t.Fatal(err)
			}

			if got, want := result.StatusCode, tt.wantStatus; got != want {
				t.Errorf("got status %d expected %d", got, want)
			}
			if got, want := string(body), tt.wantBody; !strings.Contains(got, want) {
				t.Errorf("body: expected %s in %s", want, got)
			}

		})
	}
}
