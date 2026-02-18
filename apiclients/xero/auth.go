package xero

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"reconciler/config"

	"golang.org/x/oauth2"
)

// connectionsURL is the Xero API endpoint for connection authorization and a list of
// tenants.
var connectionsURL = "https://api.xero.com/connections"

// ErrNewLoginRequired reports that a new login is required.
var ErrNewLoginRequired = errors.New("new login required")

// NewClient handles the OAuth2 flow to return an authenticated http.Client.
// It attempts to use a saved token first and will refresh it if necessary.
// If no token exists, it will fail, requiring the user to run the `login` command.
func NewClient(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*APIClient, error) {

	tok, err := LoadTokenFromFile(cfg.Xero.TokenFilePath)
	if err != nil {
		return nil, fmt.Errorf("no token file found at '%s'. Please run 'reconciler login xero' first", cfg.Xero.TokenFilePath)
	}

	// Check if the token (and refresh token) is still valid.
	if !TokenIsValid(cfg.Xero.TokenFilePath, cfg.Xero.TokenTimeoutDuration) {
		return nil, ErrNewLoginRequired
	}

	tokenSource := cfg.Xero.OAuth2Config.TokenSource(ctx, tok)

	// Check if the token was refreshed saving the new one if it was.
	refreshedToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	if refreshedToken.AccessToken != tok.AccessToken {
		log.Println("Access token was refreshed. Saving new token.")
		if err := SaveTokenToFile(refreshedToken, cfg.Xero.TokenFilePath); err != nil {
			return nil, fmt.Errorf("failed to save refreshed token: %w", err)
		}
	}

	oauthClient := oauth2.NewClient(ctx, tokenSource)

	tenantID, err := getTenantID(ctx, oauthClient)
	if err != nil {
		return nil, fmt.Errorf("failed to determine tenant ID: %w", err)
	}

	accountsRegexp, err := cfg.DonationAccountCodesAsRegex()
	if err != nil {
		return nil, fmt.Errorf("accounts regexp compliation failure: %w", err)
	}

	return NewAPIClient(tenantID, oauthClient, accountsRegexp, logger), nil
}

// InitiateLogin starts the interactive cli OAuth2 flow to get a new token from the web.
// It saves the new token to the specified path upon success.
func InitiateLogin(ctx context.Context, cfg *config.Config) error {
	tok, err := getNewTokenFromWeb(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to get new token: %w", err)
	}
	if err := SaveTokenToFile(tok, cfg.Xero.TokenFilePath); err != nil {
		return fmt.Errorf("failed to save new token: %w", err)
	}
	log.Println("Login successful. Token saved.")
	return nil
}

// getNewTokenFromWeb starts a temporary web server to handle the OAuth2 callback.
func getNewTokenFromWeb(ctx context.Context, cfg *config.Config) (*oauth2.Token, error) {
	codeChan := make(chan string)
	errChan := make(chan error)
	server := &http.Server{Addr: cfg.Web.ListenAddress}

	http.HandleFunc(cfg.Web.XeroCallBack, func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("did not receive authorization code in callback")
			return
		}
		_, _ = fmt.Fprintln(w, "Authorization successful! You can close this window.")
		codeChan <- code
	})

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			errChan <- fmt.Errorf("HTTP server failed: %w", err)
		}
	}()

	authURL := cfg.Xero.OAuth2Config.AuthCodeURL("state-string", oauth2.AccessTypeOffline)
	fmt.Printf("\nPlease open this URL in your browser to authorize the application:\n%s\n\n", authURL)

	var authCode string
	select {
	case code := <-codeChan:
		authCode = code
	case err := <-errChan:
		return nil, err
	case <-time.After(2 * time.Minute):
		return nil, fmt.Errorf("authentication timed out")
	}

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Failed to shut down server gracefully: %v", err)
	}

	tok, err := cfg.Xero.OAuth2Config.Exchange(ctx, authCode)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange authorization code for token: %w", err)
	}

	return tok, nil
}

// ValueStorer is an interface for storing values. Typically this will be implemented
// by a session store such as `github.com/alexedwards/scs/v2`.
type ValueStorer interface {
	Put(ctx context.Context, key string, val any)
	Remove(ctx context.Context, key string)
	GetString(ctx context.Context, key string) string
}

// WebServerError is an interface for raising web server errors.
type WebServerError interface {
	ServerError(w http.ResponseWriter, r *http.Request, errs ...error)
}

// InitiateWebLogin is an http.Handler for preparing a Xero OAuth2
// flow from a web interface. The Xero flow optionally does not use a PKCE verifier,
// although its use is strongly recommended.
func InitiateWebLogin(cfg *config.Config, vs ValueStorer) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Generate random state.
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			http.Error(w, "Failed to generate state", http.StatusInternalServerError)
			return
		}
		state := base64.URLEncoding.EncodeToString(b)
		vs.Put(ctx, "xero-state", state) // Save state to session

		// Generate verifier if applicable
		if !cfg.Xero.PKCEEnabled {
			authURL := cfg.Xero.OAuth2Config.AuthCodeURL(
				state,
				oauth2.AccessTypeOffline,
			)
			http.Redirect(w, r, authURL, http.StatusSeeOther)
			return
		}

		verifier := oauth2.GenerateVerifier()
		vs.Put(ctx, "xero-verifier", verifier) // Save verifier to session

		authURL := cfg.Xero.OAuth2Config.AuthCodeURL(
			state,
			oauth2.AccessTypeOffline,
			oauth2.S256ChallengeOption(verifier),
		)
		http.Redirect(w, r, authURL, http.StatusSeeOther)
	})
}

// WebLoginCallBack is an http.Handler for receiving a web callback initiated from a web
// interface.
func WebLoginCallBack(cfg *config.Config, vs ValueStorer, errLogger WebServerError) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Retrieve the state (CSRF protection) from the session and then check it
		// matches the state returned by the platform in the incoming url.
		state := vs.GetString(ctx, "xero-state")
		if state == "" {
			errLogger.ServerError(w, r, errors.New("missing 'state' in session"))
			return
		}
		vs.Remove(ctx, "xero-state") // Remove state from session.

		queryState := r.URL.Query().Get("state")
		if queryState == "" || queryState != state {
			errLogger.ServerError(w, r, errors.New("missing oauth 'state' in platform response"))
			return
		}

		var verifier string
		if cfg.Xero.PKCEEnabled {
			// Retrieve the PKCE verifier from the session.
			verifier = vs.GetString(ctx, "xero-verifier")
			if verifier == "" {
				errLogger.ServerError(w, r, errors.New("missing pkce 'verifier' in session"))
				return
			}
			vs.Remove(ctx, "xero-verifier") // Remove verifier from session.
		}

		// Extract the authorization code from the api platform's response.
		code := r.URL.Query().Get("code")
		if code == "" {
			errLogger.ServerError(w, r, errors.New("missing 'code' in platform response"))
			return
		}

		// Exchange code for token using verifier, if applicable
		var tok *oauth2.Token
		var err error
		if cfg.Xero.PKCEEnabled {
			tok, err = cfg.Xero.OAuth2Config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
			if err != nil {
				errLogger.ServerError(w, r, fmt.Errorf("token with pkce exchange failed: %w", err))
				return
			}
		} else {
			tok, err = cfg.Xero.OAuth2Config.Exchange(ctx, code)
			if err != nil {
				errLogger.ServerError(w, r, fmt.Errorf("token exchange failed: %w", err))
				return
			}
		}

		// Save the token
		if err := SaveTokenToFile(tok, cfg.Xero.TokenFilePath); err != nil {
			errLogger.ServerError(w, r, fmt.Errorf("failed to save new token: %w", err))
			return
		}

		// Success. Redirect to the "/connect" landing page.
		http.Redirect(w, r, "/connect", http.StatusSeeOther)
	})
}

// LoadTokenFromFile reads an OAuth2 token from a JSON file.
func LoadTokenFromFile(path string) (*oauth2.Token, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// SaveTokenToFile writes an OAuth2 token to a JSON file with secure permissions.
func SaveTokenToFile(token *oauth2.Token, path string) error {
	log.Printf("Saving token to %s", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("unable to cache oauth token: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()
	return json.NewEncoder(f).Encode(token)
}

// DeleteToken removes the token file from disk.
func DeleteToken(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// TokenIsValid loads a token from file and checks if the token is valid or if a refresh
// token exists to get a new token. Tokens that expire after the expirationDuration will
// be considered invalid.
func TokenIsValid(path string, expirationDuration time.Duration) bool {
	token, err := LoadTokenFromFile(path)
	if err != nil {
		return false
	}
	if token == nil {
		return false
	}
	if token.Expiry.IsZero() {
		return false
	}
	projectedExpiry := time.Now().UTC().Add(-1 * expirationDuration)
	if !token.Expiry.After(projectedExpiry) {
		return false
	}
	return token.Valid() || token.RefreshToken != ""
}
