package salesforce

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"

	"reconciler/config"
	"reconciler/internal/token"

	"golang.org/x/oauth2"
)

// ErrNewLoginRequired reports that a new login is required.
var ErrNewLoginRequired = errors.New("new login required")

// ValueStorer is an interface for storing values. Typically this will be implemented
// by a session store such as `github.com/alexedwards/scs/v2`. Note that with scs custom
// types such as token.ExtendedToken need to registered with gob.
type ValueStorer interface {
	Put(ctx context.Context, key string, val any)
	Remove(ctx context.Context, key string)
	GetString(ctx context.Context, key string) string
}

// WebServerError is an interface for raising web server errors.
type WebServerError interface {
	ServerError(w http.ResponseWriter, r *http.Request, errs ...error)
}

// InitiateWebLogin is an http.Handler for preparing a Salesforce OAuth2
// flow from a web interface.
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
		vs.Put(ctx, "sf-state", state) // Save state to session

		// Generate verifier.
		verifier := oauth2.GenerateVerifier()
		vs.Put(ctx, "sf-verifier", verifier) // Save verifier to session

		authURL := cfg.Salesforce.OAuth2Config.AuthCodeURL(
			state,
			oauth2.AccessTypeOffline,
			oauth2.S256ChallengeOption(verifier),
		)
		http.Redirect(w, r, authURL, http.StatusSeeOther)
	})
}

// WebLoginCallBack is an http.Handler for receiving a web callback initiated from a web
// interface with PKCE protection.
func WebLoginCallBack(cfg *config.Config, vs ValueStorer, errLogger WebServerError) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Retrieve the state (CSRF protection) from the session and then check it
		// matches the state returned by the platform in the incoming url.
		state := vs.GetString(ctx, "sf-state")
		if state == "" {
			errLogger.ServerError(w, r, errors.New("missing 'state' in session"))
			return
		}
		vs.Remove(ctx, "sf-state") // Remove state from session.

		queryState := r.URL.Query().Get("state")
		if queryState == "" || queryState != state {
			errLogger.ServerError(w, r, errors.New("missing oauth 'state' in platform response"))
			return
		}

		// Retrieve the PKCE verifier from the session.
		verifier := vs.GetString(ctx, "sf-verifier")
		if verifier == "" {
			errLogger.ServerError(w, r, errors.New("missing pkce 'verifier' in session"))
			return
		}
		vs.Remove(ctx, "sf-verifier") // Remove verifier from session.

		// Extract the authorization code from the api platform's response.
		code := r.URL.Query().Get("code")
		if code == "" {
			errLogger.ServerError(w, r, errors.New("missing 'code' in platform response"))
			return
		}

		// Exchange code for token using verifier.
		tok, err := cfg.Salesforce.OAuth2Config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
		if err != nil {
			errLogger.ServerError(w, r, fmt.Errorf("token exchange failed: %w", err))
			return
		}

		// Create and register new token.
		et, err := token.NewExtendedToken(token.SalesforceToken, tok)
		if err != nil {
			errLogger.ServerError(w, r, fmt.Errorf("token registration error: %w", err))
		}
		vs.Put(ctx, "salesforce-token", et)

		// Success. Redirect to the "/connect" landing page.
		http.Redirect(w, r, "/connect", http.StatusSeeOther)
	})
}
