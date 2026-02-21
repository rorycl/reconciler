package token

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"reconciler/config"

	"golang.org/x/oauth2"
)

// duration generates a time.Duration for test purposes.
func duration(t *testing.T, s string) time.Duration {
	t.Helper()
	d, err := time.ParseDuration(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// tokenPrinter prints out a token, helpful for debugging.
func tokenPrinter(t *oauth2.Token) string {
	if t == nil {
		return ""
	}
	return fmt.Sprintf("type: %s\nexpiry: %v\nrefresh: %v\n",
		t.Type(),
		t.Expiry,
		t.RefreshToken,
	)
}

// createFSConfig creates a salesforce configuration for tests.
func createSFConfig(t *testing.T, callbackURL, serverURL string) config.SalesforceConfig {
	t.Helper()
	return config.SalesforceConfig{
		LoginDomain:          serverURL,
		ClientID:             "my-client-id",
		ClientSecret:         "my-client-secret",
		TokenTimeoutDuration: duration(t, "8h"),
		OAuth2Config: &oauth2.Config{
			RedirectURL: callbackURL,
			Endpoint: oauth2.Endpoint{
				AuthURL:  fmt.Sprintf("%s/oauth2/authorize", serverURL),
				TokenURL: fmt.Sprintf("%s/oauth2/token", serverURL),
			},
			Scopes: []string{"api", "refresh_token"},
		},
	}
}

// TestTokenNotExpired tests that a (Salesforce) valid, non-expired token does not
// trigger a token refresh.
func TestTokenNotExpired(t *testing.T) {

	const instanceURL = "https://instance-url-example"

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	// Salesforce API authorization endpoint.
	mux.HandleFunc("/oauth2/authorize", func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if got, want := authHeader, "Bearer valid-token-123"; got != want {
			t.Errorf("Incorrect Authorization header got %q want %q", got, want)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = fmt.Fprintf(w, `[{"instance_url": "%s"}]`, instanceURL)
	})

	// Salesforce API /oauth2/token refresh endpoint (which should not
	// be called in this test).
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("token refresh endpoint was called unexpectedly")
	})

	validToken := &ExtendedToken{
		Type:        SalesforceToken,
		InstanceURL: instanceURL,
		Token: &oauth2.Token{
			AccessToken: "valid-token-123",
			Expiry:      time.Now().Add(1 * time.Hour), // not expired
		},
	}

	if !validToken.IsValid(duration(t, "8h")) {
		t.Fatalf("token in %#v should be valid", validToken)
	}

	cfg := &config.Config{
		Salesforce: createSFConfig(t, "/callback/sf", server.URL),
	}

	refreshed, err := validToken.ReuseOrRefresh(context.Background(), cfg.Salesforce.OAuth2Config)
	if err != nil {
		t.Fatalf("ReuseOrRefresh returned an error: %v", err)
	}
	if refreshed == true {
		t.Fatal("refresh unexpectedly returned true")
	}
	if got, want := validToken.InstanceURL, instanceURL; got != want {
		t.Errorf("instance url got %q want %q", got, want)
	}
	if got, want := validToken.Type.String(), "salesforce"; got != want {
		t.Errorf("type got %s want %s", got, want)
	}

}

// TestTokenRefresh tests that an expired token is automatically refreshed and the new
// token returned.
func TestTokenRefresh(t *testing.T) {

	const instanceURL = "https://instance-url-example"
	const newAccessToken = "new-token-456"

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	// Salesforce API authorization endpoint.
	mux.HandleFunc("/oauth2/authorize", func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if got, want := authHeader, "Bearer "+newAccessToken; got != want {
			t.Errorf("Incorrect Authorization header got %q want %q", got, want)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = fmt.Fprintf(w, `[{"instance_url": "%s"}]`, instanceURL)
	})

	// Salesforce API /oauth2/token refresh endpoint
	var refreshCalled bool
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		refreshCalled = true

		// Check that the oauth2 library is requesting a refresh correctly
		if err := r.ParseForm(); err != nil {
			t.Fatalf("Failed to parse form: %v", err)
		}
		if got := r.FormValue("grant_type"); got != "refresh_token" {
			t.Errorf("expected grant_type refresh_token, got %s", got)
		}
		if got := r.FormValue("refresh_token"); got != "my-refresh-token" {
			t.Errorf("expected refresh_token my-refresh-token, got %s", got)
		}

		// Respond with a new, valid token as JSON
		w.Header().Set("Content-Type", "application/json")

		resp := map[string]interface{}{
			"access_token":  newAccessToken,
			"token_type":    "Bearer",
			"refresh_token": "new-refresh-token-if-returned",
			// Salesforce specific fields.
			"issued_at":    strconv.FormatInt(time.Now().UnixMilli(), 10),
			"instance_url": instanceURL,
			"signature":    "some-signature",
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	thisToken := &ExtendedToken{
		Type:        SalesforceToken,
		InstanceURL: instanceURL,
		Token: &oauth2.Token{
			AccessToken:  "expired-token-000",
			RefreshToken: "my-refresh-token",
			Expiry:       time.Now().Add(-1 * time.Hour), // expired
		},
	}

	if thisToken.IsValid(duration(t, "30m")) {
		t.Fatalf("token %#v should be invalid", thisToken)
	}

	cfg := &config.Config{
		Salesforce: createSFConfig(t, "/callback/sf", server.URL),
	}

	// Run NewClient.
	refreshed, err := thisToken.ReuseOrRefresh(context.Background(), cfg.Salesforce.OAuth2Config)
	if err != nil {
		t.Fatalf("ReuseOrRefresh returned an error: %v", err)
	}
	if refreshed != true {
		t.Error("expected refresh to be true")
	}
	if !refreshCalled {
		t.Error("expected token refresh endpoint to be called, but it wasn't")
	}
	if got, want := thisToken.InstanceURL, instanceURL; got != want {
		t.Errorf("instance url got %q want %q", got, want)
	}
	if !thisToken.IsValid(duration(t, "1m")) {
		t.Errorf("token in %#v should be valid", thisToken)
	}
}

// TokenIsValid checks if the token (with or without refresh token) is still valid based
// on the specified validity period. All tests run with an expected 1 hour token
// validity.
func TestOAuth2TokenValidity(t *testing.T) {

	tests := []struct {
		accessToken  string
		refreshToken string
		expiry       time.Time
		expectedOK   bool
	}{
		{"001-ok-token", "", time.Now().UTC().Add(1 * time.Minute), true},
		{"002-expired-token", "", time.Now().UTC().Add(-1 * time.Minute), false},
		{"003-ok-token-refresh", "refresh-token", time.Now().UTC().Add(1 * time.Minute), true},
		{"004-ok-expired-with-refresh", "refresh-token", time.Now().UTC().Add(-59 * time.Minute), true},
		{"005-expired-with-refresh", "refresh-token", time.Now().UTC().Add(-61 * time.Minute), false},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("%d_%s", ii, tt.accessToken), func(t *testing.T) {
			thisToken := &ExtendedToken{
				InstanceURL: "https://an-instance.url",
				Token: &oauth2.Token{
					AccessToken:  tt.accessToken,
					RefreshToken: tt.refreshToken,
					Expiry:       tt.expiry,
				},
			}
			if got, want := thisToken.IsValid(duration(t, "1h")), tt.expectedOK; got != want {
				t.Errorf("validity check expected got %t want %t\ntoken details %v",
					got,
					want,
					tokenPrinter(thisToken.Token))
			}
		})
	}
}

// TestFixSalesforceTokenExpiry tests fixing the invalid Salesforce token expiry time.
func TestFixSalesforceTokenExpiry(t *testing.T) {
	// ms, err := strconv.ParseInt("1278448384000", 10, 64)
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// fmt.Println(time.UnixMilli(ms)) // 2010-07-06 23:33:04 +0100 BST

	tok := ExtendedToken{
		Token: &oauth2.Token{
			AccessToken: "a-token",
		},
	}
	tok.Token = tok.Token.WithExtra(map[string]any{"issued_at": "1278448384000"})
	err := tok.fixSalesforceTokenExpiry()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := tok.Token.Expiry.UTC(), time.Date(2010, 7, 6, 22, 33, 04, 0, time.UTC); got != want {
		t.Errorf("got incorrectly fixed expiry %v want %v", got, want)
	}
}

func TestTokenNames(t *testing.T) {

	tok := SalesforceToken
	if got, want := tok.String(), "salesforce"; got != want {
		t.Errorf("unexepected token name got %q want %q", got, want)
	}
	if got, want := tok.SessionName(), "salesforce-session"; got != want {
		t.Errorf("unexepected token session name got %q want %q", got, want)
	}

}
