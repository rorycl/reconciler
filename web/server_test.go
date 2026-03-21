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
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/rorycl/reconciler/apiclients/salesforce"
	"github.com/rorycl/reconciler/config"
	"github.com/rorycl/reconciler/db"
	"github.com/rorycl/reconciler/domain"
	mounts "github.com/rorycl/reconciler/internal/mounts"
	"github.com/rorycl/reconciler/internal/token"

	"golang.org/x/oauth2"
)

type reconciliationMock struct {
	donationsGet                    int
	donationsLinkUnlink             int
	invoiceDetailGet                int
	invoicesGet                     int
	transactionDetailGet            int
	transactionsGet                 int
	invoiceOrBankTransactionInfoGet int
	xeroRecordsRefresh              int
	salesforceRecordsRefresh        int
	dbIsInMemory                    int
	dbPath                          int
	closeCalled                     int
}

func (r *reconciliationMock) DonationsGet(context.Context, time.Time, time.Time, string, string, string, int, int) ([]domain.ViewDonation, error) {
	r.donationsGet++
	return nil, nil
}
func (r *reconciliationMock) DonationsLinkUnlink(context.Context, domain.SalesforceClient, []salesforce.IDRef, time.Time, time.Time) error {
	r.donationsLinkUnlink++
	return nil
}
func (r *reconciliationMock) InvoiceDetailGet(context.Context, string) (db.WRInvoice, []domain.ViewLineItem, error) {
	r.invoiceDetailGet++
	return db.WRInvoice{}, nil, nil
}
func (r *reconciliationMock) InvoicesGet(context.Context, string, time.Time, time.Time, string, int, int) ([]db.Invoice, error) {
	r.invoicesGet++
	return nil, nil
}
func (r *reconciliationMock) TransactionDetailGet(context.Context, string) (db.WRTransaction, []domain.ViewLineItem, error) {
	r.transactionDetailGet++
	return db.WRTransaction{}, nil, nil
}
func (r *reconciliationMock) TransactionsGet(context.Context, string, time.Time, time.Time, string, int, int) ([]db.BankTransaction, error) {
	r.transactionsGet++
	return nil, nil
}
func (r *reconciliationMock) InvoiceOrBankTransactionInfoGet(context.Context, string, string) (string, time.Time, error) {
	r.invoiceOrBankTransactionInfoGet++
	return "", time.Time{}, nil
}
func (r *reconciliationMock) SalesforceRecordsRefresh(context.Context, domain.SalesforceClient, time.Time, time.Time) (*domain.RefreshSalesforceResults, error) {
	r.salesforceRecordsRefresh++
	return nil, nil
}
func (r *reconciliationMock) XeroRecordsRefresh(context.Context, domain.XeroClient, time.Time, time.Time, *regexp.Regexp, bool) (*domain.RefreshXeroResults, error) {
	r.xeroRecordsRefresh++
	return nil, nil
}
func (r *reconciliationMock) DBIsInMemory() bool {
	r.dbIsInMemory++
	return true
}
func (r *reconciliationMock) DBPath() string {
	r.dbPath++
	return ""
}
func (r *reconciliationMock) Close() error {
	r.closeCalled++
	return nil
}

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
		DataStartDate:           time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC),
		DonationAccountPrefixes: []string{"51", "52", "53"},
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

	reconcilerMock := &reconciliationMock{}

	webApp, err := New(cfg, reconcilerMock, logger, staticFS, templatesFS, NewMockXeroClient, NewMockSFClient)
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
		DataStartDate:           time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC),
		DonationAccountPrefixes: []string{"53", "55", "57"},
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

	reconcilerMock := &reconciliationMock{}

	webApp, err := New(cfg, reconcilerMock, logger, staticFS, templatesFS, NewMockXeroClient, NewMockSFClient)
	if err != nil {
		t.Fatal(err)
	}

	// Set development and testing flags.
	webApp.SetInDevelopment()
	webApp.logoutDuration = time.Duration(10 * time.Millisecond)
	Exiter = func(int) {} // override default os.Exit

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
		"/logout/confirmed",
	}

	for _, path := range paths {

		resp, err := client.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("Failed to make request to %s: %v", path, err)
		}
		t.Cleanup(func() {
			_ = resp.Body.Close()
		})

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

// TestServerComponents tests some of the support components such as the error and
// render methods.
func TestServerComponents(t *testing.T) {

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name       string
		testFunc   appHandler
		wantStatus int
		wantBody   string
	}{
		{
			name: "Domain System Error",
			testFunc: func(w http.ResponseWriter, r *http.Request) error {
				return domain.ErrSystem{
					Detail: "some detail",
					Err:    errors.New("an error"),
					Msg:    "an internal error occurred",
				}
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "an internal error occurred",
		},
		{
			name: "Domain Usage Error",
			testFunc: func(w http.ResponseWriter, r *http.Request) error {
				return domain.ErrUsage{
					Detail: "domain usage",
					Msg:    "domain usage error",
				}
			},
			wantStatus: http.StatusBadRequest,
			wantBody:   "domain usage error",
		},
		{
			name: "Internal error",
			testFunc: func(w http.ResponseWriter, r *http.Request) error {
				return errInternal{
					msg: "an internal error",
					err: fmt.Errorf("an internal error"),
				}
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "an internal error",
		},
		{
			name: "Usage 404 error",
			testFunc: func(w http.ResponseWriter, r *http.Request) error {
				return errUsage{
					msg:    "example 404 error",
					status: http.StatusNotFound,
				}
			},
			wantStatus: http.StatusNotFound,
			wantBody:   "example 404 error",
		},
		{
			name: "htmx error",
			testFunc: func(w http.ResponseWriter, r *http.Request) error {
				return errHTMX{
					msg: "an htmx error",
					err: errors.New("an htmx error"),
				}
			},
			wantStatus: http.StatusOK,
			wantBody:   "an htmx error",
		},
		{
			name: "token error",
			testFunc: func(w http.ResponseWriter, r *http.Request) error {
				return token.ErrTokenWebClient{
					Context: "token",
					Err:     errors.New("an internal/token error"),
					Msg:     "a token error",
				}
			},
			wantStatus: http.StatusBadRequest, // not sure if this is appropriate
			wantBody:   "a token error",
		},
		{
			name: "render ok",
			testFunc: func(w http.ResponseWriter, r *http.Request) error {
				tpl := template.Must(template.New("t").Parse("hi {{ .Name }}"))
				a := &WebApp{inDevelopment: false, log: logger}
				return a.render(w, r, tpl, "t", map[string]string{"Name": "emaN"})
			},
			wantStatus: http.StatusOK,
			wantBody:   "hi emaN",
		},
		{
			name: "render fail",
			testFunc: func(w http.ResponseWriter, r *http.Request) error {
				tpl := template.Must(template.New("t").Parse("hi {{ .XXX }}"))
				a := &WebApp{inDevelopment: false, log: logger}
				return a.render(w, r, tpl, "t", struct{}{}) // trigger a template error
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

			wrappedHandler := a.ErrorChecker(tt.testFunc)
			wrappedHandler.ServeHTTP(w, r)

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
