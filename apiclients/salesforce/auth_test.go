package salesforce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reconciler/config"

	"golang.org/x/oauth2"
)

func createSFConfig(t *testing.T, callbackURL, serverURL, tokenPath string) config.SalesforceConfig {
	t.Helper()
	return config.SalesforceConfig{
		LoginDomain:   serverURL,
		ClientID:      "my-client-id",
		ClientSecret:  "my-client-secret",
		TokenFilePath: tokenPath,
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
	tc := &tokenCache{
		InstanceURL: "https://instance-url-example",
		Token:       &oauth2.Token{AccessToken: "xyz-123-abc"},
	}
	err := saveTokenCacheToFile(tc, filePath)
	if err != nil {
		t.Fatalf("save token failed: %v", err)
	}

	// Load the token from file.
	tc2, err := loadTokenCacheFromFile(filePath)
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
	validTokenCache := &tokenCache{
		InstanceURL: instanceURL,
		Token: &oauth2.Token{
			AccessToken: "valid-token-123",
			Expiry:      time.Now().Add(1 * time.Hour), // not expired
		},
	}
	if err := saveTokenCacheToFile(validTokenCache, tokenPath); err != nil {
		t.Fatal(err)
	}

	if !TokenIsValid(tokenPath) {
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

	expiredTokenCache := &tokenCache{
		InstanceURL: instanceURL,
		Token: &oauth2.Token{
			AccessToken:  "expired-token-000",
			RefreshToken: "my-refresh-token",
			Expiry:       time.Now().Add(-1 * time.Hour), // expired
		},
	}
	if err := saveTokenCacheToFile(expiredTokenCache, tokenPath); err != nil {
		t.Fatalf("could not save expired token cache to temp file %q: %v", tokenPath, err)
	}

	if TokenIsValid(tokenPath) {
		t.Fatalf("token in %q should be invalid", tokenPath)
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

	// Save the tokenCache.
	savedTokenCache, err := loadTokenCacheFromFile(tokenPath)
	if err != nil {
		t.Fatalf("failed to load token from disk: %v", err)
	}
	if got, want := savedTokenCache.Token.AccessToken, newAccessToken; got != want {
		t.Errorf("token file was not updated: got %q, want %q", got, want)
	}

}
