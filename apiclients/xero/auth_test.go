package xero

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

	if !TokenIsValid(tokenPath) {
		t.Fatalf("token in %q should be valid", tokenPath)
	}

	cfg := &config.Config{
		Xero: config.XeroConfig{
			TokenFilePath: tokenPath,
			OAuth2Config: &oauth2.Config{
				Endpoint: oauth2.Endpoint{
					TokenURL: server.URL + "/oauth/token",
				},
			},
		},
	}

	client, err := NewClient(context.Background(), cfg)
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

	if TokenIsValid(tokenPath) {
		t.Fatalf("token in %q should be invalid", tokenPath)
	}

	cfg := &config.Config{
		Xero: config.XeroConfig{
			TokenFilePath: tokenPath,
			OAuth2Config: &oauth2.Config{
				Endpoint: oauth2.Endpoint{
					TokenURL: server.URL + "/oauth/token",
				},
			},
		},
	}

	// Run NewClient.
	client, err := NewClient(context.Background(), cfg)
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
