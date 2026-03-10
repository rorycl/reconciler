package web

import (
	"context"
	"encoding/gob"
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

	key := validToken.Type.SessionName()
	webApp.sessions.Put(ctx, key, validToken)
	webApp.sessions.Put(ctx, token.SalesforceToken.SessionName(), validToken)

	// Setup request.
	request := httptest.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"/donations/invoice/inv-002/link",
		strings.NewReader("donation-ids=12&donation-ids=33&donation-ids=44"),
	)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Setup writer.
	writer := httptest.NewRecorder()

	r := mux.NewRouter()
	r.Handle("/donations/{type:(?:invoice|bank-transaction)}/{id}/{action}", webApp.handleDonationsLinkUnlink())
	r.ServeHTTP(writer, request)

	if writer.Code != 200 {
		t.Errorf("code %d received, not 200", writer.Code)
	}
}
