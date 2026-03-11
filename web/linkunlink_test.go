package web

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/gorilla/mux"
	"github.com/rorycl/reconciler/config"
	"github.com/rorycl/reconciler/internal/token"
	"golang.org/x/oauth2"
)

// Re-utilise mock salesforce client in refresh_test.go
func TestLinkUnlink(t *testing.T) {

	// Register types for scs.
	gob.Register(time.Time{})
	gob.Register(token.ExtendedToken{})

	testDB, closeDB := setupRefreshTestDB(t)
	t.Cleanup(closeDB)

	logger := slog.Default()

	// Register session store.
	sessionStore := scs.New()
	sessionStore.Lifetime = 1 * time.Hour

	ctx, err := sessionStore.Load(context.Background(), "")
	if err != nil {
		t.Fatalf("could not load session store: %v", err)
	}

	webApp := &WebApp{
		log:            logger,
		db:             testDB,
		sessions:       sessionStore,
		accountsRegexp: regexp.MustCompile(".*"),
		cfg: &config.Config{
			DataStartDate:           time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			DonationAccountPrefixes: []string{"53", "55", "57"},
		},

		// client factory funcs
		newSFClient: NewMockSFClient,
	}

	// Add salesforce token.
	validToken := token.ExtendedToken{
		Type:        token.SalesforceToken,
		InstanceURL: "https://example.com",
		Token: &oauth2.Token{
			AccessToken: "valid-token-234",
			Expiry:      time.Now().Add(1 * time.Hour), // not expired
		},
	}
	webApp.sessions.Put(ctx, token.SalesforceToken.SessionName(), validToken)

	tests := []struct {
		name         string
		rq           *http.Request
		expectedCode int
		expectedBody string
	}{
		{
			name: "3 donations ok",
			rq: httptest.NewRequestWithContext(
				ctx,
				http.MethodPost,
				"/donations/invoice/inv-002/link",
				strings.NewReader("donation-ids=12&donation-ids=33&donation-ids=44"),
			),
			expectedCode: 200,
			expectedBody: "",
		},
		{
			name: "no donations",
			rq: httptest.NewRequestWithContext(
				ctx,
				http.MethodPost,
				"/donations/invoice/inv-002/link",
				strings.NewReader("donation-ids="),
			),
			expectedCode: 200,
			expectedBody: "invalid data was received",
		},
		{
			name: "invalid method",
			rq: httptest.NewRequestWithContext(
				ctx,
				http.MethodGet,
				"/donations/invoice/inv-002/link",
				strings.NewReader("donation-ids=12&donation-ids=33&donation-ids=44"),
			),
			expectedCode: 200,
			expectedBody: "only POST requests allowed",
		},
		{
			name: "no invoice or bank transaction",
			rq: httptest.NewRequestWithContext(
				ctx,
				http.MethodPost,
				"/donations/invoice/inv-99999/link",
				strings.NewReader("donation-ids=12"),
			),
			expectedCode: 200,
			expectedBody: "could not get invoice/transaction info",
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("%d_%s", ii, tt.name), func(t *testing.T) {

			writer := httptest.NewRecorder()
			tt.rq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			r := mux.NewRouter()
			r.Handle("/donations/{type:(?:invoice|bank-transaction)}/{id}/{action}", webApp.handleDonationsLinkUnlink())

			r.ServeHTTP(writer, tt.rq)

			body, err := io.ReadAll(writer.Body)
			if err != nil {
				t.Fatalf("body read error: %v", err)
			}
			// htmx always expects a 200 response.
			if got, want := writer.Code, tt.expectedCode; got != want {
				t.Errorf("got code %d want %d", got, want)
			}
			if got, want := string(body), tt.expectedBody; !strings.Contains(got, want) {
				t.Errorf("got body %q should contain %q", got, want)
			}
		})
	}

}
