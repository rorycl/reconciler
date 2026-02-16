package salesforce

import (
	"context"
	"encoding/json"
	"fmt"
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

func createSFConfig(t *testing.T, callbackURL, serverURL, tokenPath string) config.SalesforceConfig {
	t.Helper()
	return config.SalesforceConfig{
		LoginDomain:          serverURL,
		ClientID:             "my-client-id",
		ClientSecret:         "my-client-secret",
		TokenFilePath:        tokenPath,
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

// TestTokenCacheFileFuncs tests the token file saving, reading and deletion
// functions.
func TestTokenCacheFileFuncs(t *testing.T) {
	dir := t.TempDir()
	fileName := `token.json`
	filePath := filepath.Join(dir, fileName)

	// Save a token.
	tc := &TokenCache{
		InstanceURL: "https://instance-url-example",
		Token:       &oauth2.Token{AccessToken: "xyz-123-abc"},
	}
	err := SaveTokenCacheToFile(tc, filePath)
	if err != nil {
		t.Fatalf("save token failed: %v", err)
	}

	// Load the token from file.
	tc2, err := LoadTokenCacheFromFile(filePath)
	if err != nil {
		t.Fatalf("load token failed %v", err)
	}
	if got, want := tc2.Token.AccessToken, tc.Token.AccessToken; got != want {
		t.Errorf("got access tcen %q want %q", got, want)
	}
	if got, want := tc2.InstanceURL, tc.InstanceURL; got != want {
		t.Errorf("got instance url %q want %q", got, want)
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
func TestTokenCache_ValidToken(t *testing.T) {

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
		fmt.Fprintf(w, `[{"instance_url": "%s"}]`, instanceURL)
	})

	// Salesforce API /oauth2/token refresh endpoint (which should not
	// be called in this test).
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("token refresh endpoint was called unexpectedly")
	})

	// Create temporary token and save to file.
	tokenPath := filepath.Join(t.TempDir(), "token.json")
	validTokenCache := &TokenCache{
		InstanceURL: instanceURL,
		Token: &oauth2.Token{
			AccessToken: "valid-token-123",
			Expiry:      time.Now().Add(1 * time.Hour), // not expired
		},
	}
	if err := SaveTokenCacheToFile(validTokenCache, tokenPath); err != nil {
		t.Fatal(err)
	}

	if !TokenIsValid(tokenPath, duration(t, "8h")) {
		t.Fatalf("token in %q should be valid", tokenPath)
	}

	cfg := &config.Config{
		Salesforce: createSFConfig(t, "/callback/sf", server.URL, tokenPath),
	}

	client, err := NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewClient returned an error: %v", err)
	}
	if got, want := client.instanceURL, instanceURL; got != want {
		t.Errorf("got instanceURL %q want %q", got, want)
	}

}

// TestTokenCache_RefreshExpiredToken tests that an expiredtoken is
// automatically refreshed and the new token is saved.
func TestTokenCache_RefreshExpiredToken(t *testing.T) {
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
		fmt.Fprintf(w, `[{"instance_url": "%s"}]`, instanceURL)
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
		newToken := &oauth2.Token{
			AccessToken: newAccessToken,
			TokenType:   "Bearer",
			Expiry:      time.Now().Add(1 * time.Hour),
		}
		json.NewEncoder(w).Encode(newToken)
	})

	// Create temporary token and save to file.
	tokenPath := filepath.Join(t.TempDir(), "token.json")

	expiredTokenCache := &TokenCache{
		InstanceURL: instanceURL,
		Token: &oauth2.Token{
			AccessToken:  "expired-token-000",
			RefreshToken: "my-refresh-token",
			Expiry:       time.Now().Add(-1 * time.Hour), // expired
		},
	}
	if err := SaveTokenCacheToFile(expiredTokenCache, tokenPath); err != nil {
		t.Fatalf("could not save expired token cache to temp file %q: %v", tokenPath, err)
	}

	// This test succeeds because it has a refresh token and is within the
	// TokenTimeoutDuration period.
	if !TokenIsValid(tokenPath, duration(t, "2h")) {
		t.Fatalf("token in %q should be valid", tokenPath)
	}

	cfg := &config.Config{
		Salesforce: createSFConfig(t, "/callback/sf", server.URL, tokenPath),
	}

	// Run NewClient.
	client, err := NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewClient returned an error: %v", err)
	}
	if !refreshCalled {
		t.Error("expected token refresh endpoint to be called, but it wasn't")
	}
	if got, want := client.instanceURL, instanceURL; got != want {
		t.Errorf("got instanceURL %q want %q", got, want)
	}

	// Save the TokenCache.
	savedTokenCache, err := LoadTokenCacheFromFile(tokenPath)
	if err != nil {
		t.Fatalf("failed to load token from disk: %v", err)
	}
	if got, want := savedTokenCache.Token.AccessToken, newAccessToken; got != want {
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

// TestAuthWebLoginAndCallback tests the local web OAuth2 login and callback handlers.
func TestAuthWebLoginAndCallback(t *testing.T) {

	const mockInstanceURL = "https://mock-salesforce-instance.com"
	const mockAccessToken = "mock-access-token-zyx"
	const testOAuth2Code = "01c20e0b"

	// Create a web server to act for the salesforce platform.
	sfMux := http.NewServeMux()
	sfServer := httptest.NewServer(sfMux)
	defer sfServer.Close()

	// Mock the salesforce token endpoint.
	sfMux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
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
		if r.FormValue("code_verifier") == "" {
			t.Errorf("expected a PKCE code_verifier in the token request")
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

	// Setup a local server for attaching the handlers for testing.
	tokenPath := filepath.Join(t.TempDir(), "web_handler_token.json")

	cfg := &config.Config{
		Salesforce: createSFConfig(
			t,
			"/salesforce/callback",
			sfServer.URL,
			tokenPath,
		),
	}

	// Setup in-memory session manager.
	sessionManager := scs.New()
	sessionManager.Lifetime = 1 * time.Hour

	// Attach the handlers with the session middleware.
	localMux := http.NewServeMux()

	localMux.Handle("/salesforce/init", sessionManager.LoadAndSave(
		InitiateWebLogin(cfg, sessionManager),
	))

	localMux.Handle("/salesforce/callback", sessionManager.LoadAndSave(
		WebLoginCallBack(cfg, sessionManager, mockErrorLogger{t: t}),
	))

	localServer := httptest.NewServer(localMux)
	defer localServer.Close()

	// ************************************************************************
	// client testing actions
	// ************************************************************************

	// Test client has redirect disabled.
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Test phase 1 (init).
	initURL, _ := url.JoinPath(localServer.URL, "/salesforce/init")
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
	callbackURL.Path = "/salesforce/callback"

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
	savedCache, err := LoadTokenCacheFromFile(tokenPath)
	if err != nil {
		t.Fatalf("could not load generated token file: %v", err)
	}
	if got, want := savedCache.InstanceURL, mockInstanceURL; got != want {
		t.Errorf("saved instanceURL: got %q, want %q", got, want)
	}
	if got, want := savedCache.Token.AccessToken, mockAccessToken; got != want {
		t.Errorf("saved accessToken: got %q want %q", got, want)
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
			tokenCache := &TokenCache{
				InstanceURL: "https://an-instance.url",
				Token: &oauth2.Token{
					AccessToken:  tt.accessToken,
					RefreshToken: tt.refreshToken,
					Expiry:       tt.expiry,
				},
			}
			err := SaveTokenCacheToFile(tokenCache, tempFile)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := TokenIsValid(tempFile, duration(t, "1h")), tt.expectedOK; got != want {
				t.Errorf("validity check expected got %t want %t\ntoken details %v", got, want, tokenPrinter(tokenCache.Token))
			}
		})
	}
}
