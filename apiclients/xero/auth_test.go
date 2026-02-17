package xero

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reconciler/config"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/oauth2"
)

func duration(t *testing.T, s string) time.Duration {
	t.Helper()
	d, err := time.ParseDuration(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

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

func createXeroConfig(t *testing.T, callbackURL, serverURL, tokenPath string) config.XeroConfig {
	t.Helper()
	return config.XeroConfig{
		ClientID:             "my-client-id",
		ClientSecret:         "my-client-secret",
		TokenFilePath:        tokenPath,
		TokenTimeoutDuration: duration(t, "8h"),
		PKCEEnabled:          true,
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

// TestTokenFileFuncs tests the token file saving, reading and deletion
// functions.
func TestTokenFileFuncs(t *testing.T) {
	dir := t.TempDir()
	fileName := `token.json`
	filePath := filepath.Join(dir, fileName)

	// Save a token.
	tok := &oauth2.Token{AccessToken: "xyz-123-abc"}
	err := SaveTokenToFile(tok, filePath)
	if err != nil {
		t.Fatalf("save token failed: %v", err)
	}

	// Load the token from file.
	tok2, err := LoadTokenFromFile(filePath)
	if err != nil {
		t.Fatalf("load token failed %v", err)
	}
	if got, want := tok2.AccessToken, tok.AccessToken; got != want {
		t.Errorf("got access token %q want %q", got, want)
	}

	// Delete the token.
	if err = DeleteToken(filePath); err != nil {
		t.Fatalf("deletetoken failed: %v", err)
	}

	_, err = os.Stat(filePath)
	if !os.IsNotExist(err) {
		t.Fatal("token file still exists on disk after delete called")
	}
}

// TestTokenValid tests if a slightly modified real xero token is valid.
func TestTokenValid(t *testing.T) {

	file := "testdata/xero_token.json"
	tok, err := LoadTokenFromFile(file)
	if err != nil {
		t.Fatal("error loading test json file", err)
	}
	if !TokenIsValid(file, duration(t, "99999h")) {
		t.Errorf("token with expiry was not classed as valid:\n%v", tokenPrinter(tok))
	}
}

// setupTestServer creates a test environment for OAuth2 web server calls.
func setupTestServer(t *testing.T) (mux *http.ServeMux, teardown func(), addr string) {
	t.Helper()
	mux = http.NewServeMux()
	server := httptest.NewServer(mux)
	teardown = func() {
		server.Close()
	}
	return mux, teardown, server.URL
}

// TestNewClient_ValidToken tests that a client with a valid, non-expired token
// is created successfully without triggering a token refresh.
func TestToken_ValidToken(t *testing.T) {
	const expectedTenantID = "tenant-abc-123"

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	// Override auth connection url for this test.
	origURL := connectionsURL
	connectionsURL = server.URL + "/connections"
	t.Cleanup(func() {
		connectionsURL = origURL
	})

	// Xero API /connections endpoint.
	mux.HandleFunc("/connections", func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if got, want := authHeader, "Bearer valid-token-123"; got != want {
			t.Errorf("Incorrect Authorization header got %q want %q", got, want)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		fmt.Fprintf(w, `[{"tenantId": "%s"}]`, expectedTenantID)
	})

	// Xero API /oauth/token endpoint (which should not be called in
	// this test).
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("token refresh endpoint was called unexpectedly")
	})

	// Create temporary token and save to file.
	tokenPath := filepath.Join(t.TempDir(), "token.json")
	validToken := &oauth2.Token{
		AccessToken: "valid-token-123",             // todo: fixme
		Expiry:      time.Now().Add(1 * time.Hour), // not expired
	}
	if err := SaveTokenToFile(validToken, tokenPath); err != nil {
		t.Fatal(err)
	}

	if !TokenIsValid(tokenPath, duration(t, "8h")) {
		t.Fatalf("token in %q should be valid", tokenPath)
	}

	cfg := &config.Config{
		Xero: config.XeroConfig{
			TokenFilePath:        tokenPath,
			TokenTimeoutDuration: duration(t, "8h"),
			OAuth2Config: &oauth2.Config{
				Endpoint: oauth2.Endpoint{
					TokenURL: server.URL + "/oauth/token",
				},
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(
		os.Stdout,
		&slog.HandlerOptions{Level: slog.LevelDebug},
	))

	client, err := NewClient(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("NewClient returned an error: %v", err)
	}
	if got, want := client.tenantID, expectedTenantID; got != want {
		t.Errorf("got tenantID %q want %q", got, want)
	}

}

// TestToken_RefreshExpiredToken tests that an expiredtoken is
// automatically refreshed and the new token is saved.
func TestToken_RefreshExpiredToken(t *testing.T) {
	const expectedTenantID = "tenant-abc-123"
	const newAccessToken = "new-token-456"

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	// Override auth connection url for this test.
	origURL := connectionsURL
	connectionsURL = server.URL + "/connections"
	t.Cleanup(func() {
		connectionsURL = origURL
	})

	// Xero API /connections endpoint.
	mux.HandleFunc("/connections", func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if got, want := authHeader, "Bearer "+newAccessToken; got != want {
			t.Errorf("Incorrect Authorization header got %q want %q", got, want)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		fmt.Fprintf(w, `[{"tenantId": "%s"}]`, expectedTenantID)
	})

	// Xero API /oauth/token endpoint
	var refreshCalled bool
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		refreshCalled = true

		// Check that the oauth2 library is requesting a refresh correctly
		if got := r.FormValue("grant_type"); got != "refresh_token" {
			t.Errorf("expected grant_type refresh_token, got %s", got)
		}
		if got := r.FormValue("refresh_token"); got != "my-refresh-token" {
			t.Errorf("expected refresh_token my-refresh-token, got %s", got)
		}

		// Respond with a new, valid token as JSON
		w.Header().Set("Content-Type", "application/json")
		newToken := oauth2.Token{
			AccessToken: newAccessToken,
			Expiry:      time.Now().Add(1 * time.Hour),
		}
		json.NewEncoder(w).Encode(newToken)

	})

	// Create temporary token and save to file.
	tokenPath := filepath.Join(t.TempDir(), "token.json")

	expiredToken := &oauth2.Token{
		AccessToken:  "expired-token-000",
		RefreshToken: "my-refresh-token",
		Expiry:       time.Now().Add(-1 * time.Hour), // expired
	}
	if err := SaveTokenToFile(expiredToken, tokenPath); err != nil {
		t.Fatalf("could not save expired token to temp file %q: %v", tokenPath, err)
	}

	// This test succeeds because it has a refresh token and is within the
	// TokenTimeoutDuration period.
	if !TokenIsValid(tokenPath, duration(t, "2h")) {
		t.Fatalf("token in %q should be valid", tokenPath)
	}

	cfg := &config.Config{
		Xero: config.XeroConfig{
			TokenFilePath:        tokenPath,
			TokenTimeoutDuration: duration(t, "8h"),
			PKCEEnabled:          true,
			OAuth2Config: &oauth2.Config{
				Endpoint: oauth2.Endpoint{
					TokenURL: server.URL + "/oauth/token",
				},
			},
		},
	}

	// Run NewClient.
	logger := slog.New(slog.NewTextHandler(
		os.Stdout,
		&slog.HandlerOptions{Level: slog.LevelDebug},
	))
	client, err := NewClient(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("NewClient returned an error: %v", err)
	}
	if !refreshCalled {
		t.Error("expected token refresh endpoint to be called, but it wasn't")
	}
	if got, want := client.tenantID, expectedTenantID; got != want {
		t.Errorf("got tenantID %q want %q", got, want)
	}

	// Save the token.
	savedToken, err := LoadTokenFromFile(tokenPath)
	if err != nil {
		t.Fatalf("failed to load token from disk: %v", err)
	}
	if got, want := savedToken.AccessToken, newAccessToken; got != want {
		t.Errorf("token file was not updated: got %q, want %q", got, want)
	}

}

// mock the WebServerError interface.
type mockErrorLogger struct {
	t *testing.T
}

// ServerError meets the WebServerError interface for raising web server errors.
func (m mockErrorLogger) ServerError(w http.ResponseWriter, r *http.Request, errs ...error) {
	m.t.Helper()
	for _, err := range errs {
		m.t.Logf("[WebServerError] %v", err)
	}
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}

// TestAuthWebLoginAndCallbacktests the local web OAuth2 login and callback
// handlers with and without PKCE verification.
func TestAuthWebLoginAndCallbackPKCE(t *testing.T) {

	const mockInstanceURL = "https://mock-xero-instance.com"
	const mockAccessToken = "mock-access-token-zyx"
	const testOAuth2Code = "08b2c1d"

	// Create a web server to act for the xero platform.
	xeroMux := http.NewServeMux()
	xeroServer := httptest.NewServer(xeroMux)
	defer xeroServer.Close()

	// Create config.
	tokenPath := filepath.Join(t.TempDir(), "web_handler_token.json")
	cfg := &config.Config{
		Xero: createXeroConfig(
			t,
			"/xero/callback",
			xeroServer.URL,
			tokenPath,
		),
	}

	// Mock the xero token endpoint.
	xeroMux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		// Check this is an authorization_code exchange.
		if got, want := r.FormValue("grant_type"), "authorization_code"; got != want {
			t.Errorf("expected grant_type %q, got %q", got, want)
		}
		if got, want := r.FormValue("code"), testOAuth2Code; got != want {
			t.Errorf("expected code %q, got %q", got, want)
		}
		if cfg.Xero.PKCEEnabled {
			if r.FormValue("code_verifier") == "" {
				t.Errorf("expected a PKCE code_verifier in the token request")
			}
		}
		// Return a token.
		w.Header().Set("Content-type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  mockAccessToken,
			"token_type":    "Bearer",
			"refresh_token": "mock-refresh-token",
			"instance_url":  mockInstanceURL,
			"expires_in":    3600,
		})
	})

	// Setup in-memory session manager.
	sessionManager := scs.New()
	sessionManager.Lifetime = 1 * time.Hour

	// Attach the handlers with the session middleware.
	localMux := http.NewServeMux()

	localMux.Handle("/xero/init", sessionManager.LoadAndSave(
		InitiateWebLogin(cfg, sessionManager),
	))

	localMux.Handle("/xero/callback", sessionManager.LoadAndSave(
		WebLoginCallBack(cfg, sessionManager, mockErrorLogger{t: t}),
	))

	localServer := httptest.NewServer(localMux)
	defer localServer.Close()

	// ************************************************************************
	// client testing actions
	// ************************************************************************

	// Run two clients: one in PKCE mode, one without.
	for ii, tt := range []bool{true, false} {
		t.Run(fmt.Sprintf("test_%d", ii), func(t *testing.T) {

			cfg.Xero.PKCEEnabled = tt

			// Test client has redirect disabled.
			client := &http.Client{
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}

			// Test phase 1 (init).
			initURL, _ := url.JoinPath(localServer.URL, "/xero/init")
			resp, err := client.Get(initURL)
			if err != nil {
				t.Fatalf("Failed to call /init: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusSeeOther {
				t.Fatalf("expected 303 redirect from /init; got %d", resp.StatusCode)
			}

			cookies := resp.Cookies()
			if len(cookies) == 0 {
				t.Fatal("exected session cookie from /init, got none")
			}
			phase1SessionCookie := cookies[0]

			locationURL, err := url.Parse(resp.Header.Get("Location"))
			if err != nil {
				t.Fatalf("failed to parse redirect location: %v", err)
			}
			phase1State := locationURL.Query().Get("state")
			if phase1State == "" {
				t.Fatal("redirect URL did not contain 'state' parameter")
			}

			// Test phase 2 (callback).
			callbackURL, _ := url.Parse(localServer.URL)
			callbackURL.Path = "/xero/callback"

			q := callbackURL.Query()
			q.Set("state", phase1State)
			q.Set("code", testOAuth2Code)
			callbackURL.RawQuery = q.Encode()

			// Setup the request to the callback url; attaching the session cookie.
			req, _ := http.NewRequest("GET", callbackURL.String(), nil)
			req.AddCookie(phase1SessionCookie)

			respCallback, err := client.Do(req)
			if err != nil {
				t.Fatalf("Failed to call /callback: %v", err)
			}
			defer respCallback.Body.Close()

			if respCallback.StatusCode != http.StatusSeeOther {
				t.Fatalf("expected 303 Redirect from /callback (success), got %d", respCallback.StatusCode)
			}

			expectedRedirect := "/connect"
			if got := respCallback.Header.Get("Location"); got != expectedRedirect {
				t.Errorf("expected redirect to %q, got %q", expectedRedirect, got)
			}

			// Verify persistence.
			savedToken, err := LoadTokenFromFile(tokenPath)
			if err != nil {
				t.Fatalf("could not load generated token file: %v", err)
			}
			if got, want := savedToken.AccessToken, mockAccessToken; got != want {
				t.Errorf("saved Token: got %q want %q", got, want)
			}
		})
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

	tempFile := filepath.Join(t.TempDir(), "token_validity_test.json")

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("%d_%s", ii, tt.accessToken), func(t *testing.T) {
			tok := &oauth2.Token{
				AccessToken:  tt.accessToken,
				RefreshToken: tt.refreshToken,
				Expiry:       tt.expiry,
			}
			err := SaveTokenToFile(tok, tempFile)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := TokenIsValid(tempFile, duration(t, "1h")), tt.expectedOK; got != want {
				t.Errorf("validity check expected got %t want %t\ntoken details %v", got, want, tokenPrinter(tok))
			}
		})
	}
}
