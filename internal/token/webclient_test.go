package token

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"reconciler/config"

	"github.com/alexedwards/scs/v2"
)

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
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  mockAccessToken,
			"token_type":    "Bearer",
			"refresh_token": "mock-refresh-token",
			"instance_url":  mockInstanceURL,
			// "expires_in":    3600, // not that salesforce does this...
			"issued_at": strconv.FormatInt(time.Now().UnixMilli(), 10),
		})
	})

	cfg := &config.Config{
		Salesforce: createSFConfig(
			t,
			"/salesforce/callback",
			sfServer.URL,
		),
	}

	// Setup in-memory session manager.
	gob.Register(ExtendedToken{})
	sessionManager := scs.New()
	sessionManager.Lifetime = 1 * time.Hour

	// Attach the handlers with the session middleware.
	localMux := http.NewServeMux()

	// Initialise a TokenWebHandler for the two OAuth2 "local" handlers.
	twc, err := NewTokenWebClient(
		SalesforceToken,
		cfg.Salesforce.OAuth2Config,
		sessionManager,
		mockErrorLogger{t},
		"/connect",
	)
	if err != nil {
		t.Fatalf("NewTokenWebHander error: %v", err)
	}

	localMux.Handle("/salesforce/init", sessionManager.LoadAndSave(
		twc.InitiateWebLogin(),
	))
	localMux.Handle("/salesforce/callback", sessionManager.LoadAndSave(
		twc.WebLoginCallBack(),
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
	defer func() {
		_ = resp.Body.Close()
	}()

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
		t.Fatalf("Failed to call /callback: %v", err) // failure (EOF)
	}
	defer func() {
		_ = respCallback.Body.Close()
	}()

	if respCallback.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 Redirect from /callback (success), got %d", respCallback.StatusCode)
	}

	expectedRedirect := "/connect"
	if got := respCallback.Header.Get("Location"); got != expectedRedirect {
		t.Errorf("expected redirect to %q, got %q", expectedRedirect, got)
	}

	// Extract and verify callback cookie value.
	callbackCookieVal := phase1SessionCookie.Value
	callbackCookies := respCallback.Cookies()
	if len(callbackCookies) > 0 {
		callbackCookieVal = callbackCookies[0].Value
	}

	// Manually load the session data from the store into a context, keyed by the
	//
	// scs.Load takes a background context and the token string, and returns
	// a context populated with the session data.
	verifyCtx, err := sessionManager.Load(context.Background(), callbackCookieVal)
	if err != nil {
		t.Fatalf("failed to load session for verification: %v", err)
	}

	// Query the session manager using this populated context.
	val := sessionManager.Get(verifyCtx, twc.SessionKey())
	if val == nil {
		t.Fatal("session value was nil")
	}

	sessionToken, ok := val.(ExtendedToken)
	if !ok {
		t.Fatalf("could not type assert extended token from session. Got type: %T", val)
	}

	if got, want := sessionToken.InstanceURL, mockInstanceURL; got != want {
		t.Errorf("saved instanceURL: got %q, want %q", got, want)
	}
	if got, want := sessionToken.Token.AccessToken, mockAccessToken; got != want {
		t.Errorf("saved accessToken: got %q want %q", got, want)
	}

}
