package token

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"

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

// TokenWebClient is a type for providing OAuth2 web handlers as clients to platform
// OAuth2 servers.
type TokenWebClient struct {
	typer     TokenType
	oauthCfg  *oauth2.Config
	vs        ValueStorer
	errLogger WebServerError
	redirURL  string
}

// NewTokenWebClient creates a new TokenWebClient.
func NewTokenWebClient(
	typer TokenType,
	oauthCfg *oauth2.Config,
	vs ValueStorer,
	errLogger WebServerError,
	redirURL string, // eg "/connect"
) (*TokenWebClient, error) {
	if oauthCfg == nil {
		return nil, errors.New("nil oauthCfg provided to NewTokenWebClient")
	}
	if vs == nil {
		return nil, errors.New("nil ValueStorer (session) provided to NewTokenWebClient")
	}
	if errLogger == nil {
		return nil, errors.New("nil WebServerError provided to NewTokenWebClient")
	}
	if redirURL == "" {
		return nil, errors.New("empty redirection URL provided")
	}
	return &TokenWebClient{
		typer:    typer,
		oauthCfg: oauthCfg,
		vs:       vs,
		redirURL: redirURL,
	}, nil
}

func (twc *TokenWebClient) stateKey() string {
	return fmt.Sprintf("%s-%s", twc.typer, "state")
}

func (twc *TokenWebClient) verifierKey() string {
	return fmt.Sprintf("%s-%s", twc.typer, "verifier")
}

func (twc *TokenWebClient) SessionKey() string {
	return fmt.Sprintf("%s-%s", twc.typer, "session")
}

// InitiateWebLogin is an http.Handler for preparing a Salesforce OAuth2
// flow from a web interface, which takes an OAuth2 config, ValueStorer (typically
// session saver) interface and an idPrefix. The last is used for prefixing ValueStorer
// keys.
func (twc *TokenWebClient) InitiateWebLogin() http.Handler {

	if twc == nil {
		panic("TokenWebClient nil at InitiateWebLogin")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Generate random state.
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			twc.errLogger.ServerError(w, r, errors.New("failed to generate state"))
			return
		}
		state := base64.URLEncoding.EncodeToString(b)
		twc.vs.Put(ctx, twc.stateKey(), state) // Save state to session

		// Generate verifier.
		verifier := oauth2.GenerateVerifier()
		twc.vs.Put(ctx, twc.verifierKey(), verifier) // Save verifier to session

		authURL := twc.oauthCfg.AuthCodeURL(
			state,
			oauth2.AccessTypeOffline,
			oauth2.S256ChallengeOption(verifier),
		)
		http.Redirect(w, r, authURL, http.StatusSeeOther)
	})
}

// WebLoginCallBack is an http.Handler for receiving a web callback initiated from a web
// interface with PKCE protection.
func (twc *TokenWebClient) WebLoginCallBack() http.Handler {

	if twc == nil {
		panic("TokenWebClient nil at WebLoginCallBack")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Retrieve the state (CSRF protection) from the session and then check it
		// matches the state returned by the platform in the incoming url.
		state := twc.vs.GetString(ctx, twc.stateKey())
		if state == "" {
			twc.errLogger.ServerError(w, r, errors.New("missing 'state' in session"))
			return
		}
		twc.vs.Remove(ctx, twc.stateKey()) // Remove state from session.

		queryState := r.URL.Query().Get("state")
		if queryState == "" || queryState != state {
			twc.errLogger.ServerError(w, r, errors.New("missing oauth 'state' in platform response"))
			return
		}

		// Retrieve the PKCE verifier from the session.
		verifier := twc.vs.GetString(ctx, twc.verifierKey())
		if verifier == "" {
			twc.errLogger.ServerError(w, r, errors.New("missing pkce 'verifier' in session"))
			return
		}
		twc.vs.Remove(ctx, twc.verifierKey()) // Remove verifier from session.

		// Extract the authorization code from the api platform's response.
		code := r.URL.Query().Get("code")
		if code == "" {
			twc.errLogger.ServerError(w, r, errors.New("missing 'code' in platform response"))
			return
		}

		// Exchange code for token using verifier.
		tok, err := twc.oauthCfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
		if err != nil {
			twc.errLogger.ServerError(w, r, fmt.Errorf("token exchange failed: %w", err))
			return
		}
		if tok == nil {
			twc.errLogger.ServerError(w, r, fmt.Errorf("nil token received from oauth2.VerifierOption: %w", err))
			return
		}

		// Create and register new token.
		et, err := NewExtendedToken(twc.typer, tok)
		if err != nil {
			twc.errLogger.ServerError(w, r, fmt.Errorf("token registration error: %w", err))
			return
		}
		twc.vs.Put(ctx, twc.SessionKey(), et)

		// Success. Redirect to a url such as the "/connect" landing page.
		http.Redirect(w, r, twc.redirURL, http.StatusSeeOther)
	})
}
