package salesforce

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"reconciler/config"

	"golang.org/x/oauth2"
)

// SalesforceAPIVersionNumber sets out the currently supported
// Salesforce API used for this client.
const SalesforceAPIVersionNumber = "v65.0"

// ErrNewLoginRequired reports that a new login is required.
var ErrNewLoginRequired = errors.New("new login required")

// TokenCache is a helper struct to save and load a Salesforce OAuth2 token and the
// instance_url from a file.
type TokenCache struct {
	Token       *oauth2.Token `json:"token"`
	InstanceURL string        `json:"instance_url"`
}

// NewClient handles the OAuth2 flow to return an authenticated Salesforce client.
func NewClient(ctx context.Context, cfg *config.Config) (*Client, error) {
	cache, err := LoadTokenCacheFromFile(cfg.Salesforce.TokenFilePath)
	if err != nil {
		return nil, fmt.Errorf("no token file found at '%s'. Please run the 'login' command first", cfg.Salesforce.TokenFilePath)
	}

	tokenSource := cfg.Salesforce.OAuth2Config.TokenSource(ctx, cache.Token)
	refreshedToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	// Fix the Salesforce lack of an Expiry date.
	fixSalesforceTokenExpiry(refreshedToken)

	if refreshedToken.AccessToken != cache.Token.AccessToken {
		log.Println("Access token was refreshed. Saving new token.")
		cache.Token = refreshedToken

		// The instance_url does not change on refresh, so keep the old one.
		if err := SaveTokenCacheToFile(cache, cfg.Salesforce.TokenFilePath); err != nil {
			return nil, fmt.Errorf("failed to save refreshed token: %w", err)
		}
	}

	oauthClient := oauth2.NewClient(ctx, tokenSource)
	return &Client{
		httpClient:  oauthClient,
		instanceURL: cache.InstanceURL,
		apiVersion:  SalesforceAPIVersionNumber,
		config:      *cfg,
	}, nil
}

// InitiateLogin starts the interactive cli OAuth2 flow to get a new token from the web.
// It saves the new token and instance URL to the specified configuration path upon
// success.
func InitiateLogin(ctx context.Context, cfg *config.Config) error {
	tok, err := getNewTokenFromWeb(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to get new token: %w", err)
	}

	// Check if the token (and refresh token) is still valid.
	if !TokenIsValid(cfg.Salesforce.TokenFilePath, cfg.Salesforce.TokenTimeoutDuration) {
		return ErrNewLoginRequired
	}

	instanceURL, ok := tok.Extra("instance_url").(string)
	if !ok || instanceURL == "" {
		return fmt.Errorf("oauth token did not contain the required 'instance_url'")
	}

	cache := &TokenCache{Token: tok, InstanceURL: instanceURL}
	if err := SaveTokenCacheToFile(cache, cfg.Salesforce.TokenFilePath); err != nil {
		return fmt.Errorf("failed to save new token: %w", err)
	}
	log.Println("Login successful. Token saved.")
	return nil
}

// getNewTokenFromWeb starts a temporary web server for the cli to handle the OAuth2
// callback. It uses the PKCE extension for enhanced security.
func getNewTokenFromWeb(ctx context.Context, cfg *config.Config) (*oauth2.Token, error) {
	codeChan := make(chan string)
	errChan := make(chan error)
	server := &http.Server{Addr: cfg.Web.ListenAddress}

	http.HandleFunc(cfg.Web.SalesforceCallBack, func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("did not receive authorization code in callback")
			return
		}
		fmt.Fprintln(w, "Authorization successful! You can close this window.")
		codeChan <- code
	})

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			errChan <- fmt.Errorf("HTTP server failed: %w", err)
		}
	}()

	verifier := oauth2.GenerateVerifier()
	authURL := cfg.Salesforce.OAuth2Config.AuthCodeURL("state-string", oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(verifier))

	fmt.Printf("\nPlease open this URL in your browser to authorize the application:\n%s\n\n", authURL)

	var authCode string
	select {
	case code := <-codeChan:
		authCode = code
	case err := <-errChan:
		return nil, err
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("authentication timed out")
	}

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Failed to shut down server gracefully: %v", err)
	}

	tok, err := cfg.Salesforce.OAuth2Config.Exchange(ctx, authCode, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("failed to exchange authorization code for token: %w", err)
	}

	// Generate Expiry (as salesforce tokens are invalid).
	fixSalesforceTokenExpiry(tok)

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
		vs.Put(ctx, "state", state) // Save state to session

		// Generate verifier.
		verifier := oauth2.GenerateVerifier()
		vs.Put(ctx, "verifier", verifier) // Save verifier to session

		authURL := cfg.Salesforce.OAuth2Config.AuthCodeURL(
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
		state := vs.GetString(ctx, "state")
		if state == "" {
			errLogger.ServerError(w, r, errors.New("missing 'state' in session"))
			return
		}
		vs.Remove(ctx, "state") // Remove state from session.

		queryState := r.URL.Query().Get("state")
		if queryState == "" || queryState != state {
			errLogger.ServerError(w, r, errors.New("missing oauth 'state' in platform response"))
			return
		}

		// Retrieve the PKCE verifier from the session.
		verifier := vs.GetString(ctx, "verifier")
		if verifier == "" {
			errLogger.ServerError(w, r, errors.New("missing 'verifier' in session"))
			return
		}
		vs.Remove(ctx, "verifier") // Remove verifier from session.

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

		// Generate Expiry (as salesforce tokens are invalid).
		fixSalesforceTokenExpiry(tok)

		// An "instance_url" is required from the token.
		// See https://help.salesforce.com/s/articleView?id=xcloud.remoteaccess_oauth_client_credentials_flow.htm&type=5
		instanceURL, ok := tok.Extra("instance_url").(string)
		if !ok || instanceURL == "" {
			errLogger.ServerError(w, r, fmt.Errorf("oauth token did not contain the required 'instance_url'"))
			return
		}
		vs.Put(ctx, "salesforce-instance-url", instanceURL)

		// Save the token with the instance url.
		cache := &TokenCache{Token: tok, InstanceURL: instanceURL}
		if err := SaveTokenCacheToFile(cache, cfg.Salesforce.TokenFilePath); err != nil {
			errLogger.ServerError(w, r, fmt.Errorf("failed to save new token: %w", err))
			return
		}

		// Success. Redirect to the "/connect" landing page.
		http.Redirect(w, r, "/connect", http.StatusSeeOther)
	})
}

// LoadTokenCacheFromFile reads a token cache from a JSON file.
func LoadTokenCacheFromFile(path string) (*TokenCache, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cache := &TokenCache{}
	err = json.NewDecoder(f).Decode(cache)
	return cache, err
}

// SaveTokenCacheToFile writes a token cache to a JSON file with secure permissions.
func SaveTokenCacheToFile(cache *TokenCache, path string) error {
	log.Printf("Saving token to %s", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("unable to cache oauth token: %w", err)
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(cache)
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
	tokenCache, err := LoadTokenCacheFromFile(path)
	if err != nil {
		return false
	}
	token := tokenCache.Token
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

// fixSalesforceTokenExpiry fixes the "IsZero" expiry of Saleforce's tokens.
// sf tokens are something like the following. Annoyingly, the expiration time is set to the Go zero time.
// This causes the TokenIsValid check to fail.
//
//	&oauth2.Token{
//		AccessToken:"00DgL00000GE1HZ!AQEAQLfSAKItTlIlnlHXdspaJKhLFlgxDxbxDpaiGav_JvZSTH227U16WWqOil8Y7O.ZhbfaJ8GmuY1Bu58rkXCXXXXXXXXX",
//		TokenType:"Bearer",
//		RefreshToken:"5Aep861eN26Sp9j0R6usW_6CAZUnTI_3Now99RwZbU929ZCwAjRmSTEXHQdUigeK121uGSsEqoAW8xxxxxxxxxx",
//		Expiry:time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC),
//		ExpiresIn:0,
//		raw:map[string]interface {}{
//			"access_token":"00DgL00000GE1HZ!AQEAQLfSAKItTlIlnlHXdspaJKhLFlgxDxbxDpaiGav_JvZSTH227U16WWqOil8Y7O.ZhbfaJ8GmuY1Bu58rkXCxxxxxxxxx",
//			"id":"https://login.salesforce.com/id/00DgL00000GE1HZUA1/005gL00000BOwSkQAL",
//			"instance_url":"https://orgfarm-xxxxxxxxxx-dev-ed.develop.my.salesforce.com",
//			"issued_at":"1771257689412",
//			"refresh_token":"5Aep861eN26Sp9j0R6usW_6CAZUnTI_3Now99RwZbU929ZCwAjRmSTEXHQdUigeK121uGSsEqoAW80Yxxxxxxxx",
//			"scope":"refresh_token api",
//			"signature":"ze9U3HZv46gm3wmLJ/JmlWgg2wGVvLh0Fxxxxxxxxxxx",
//			"token_type":"Bearer"
//		},
//			expiryDelta:0,
//	}
func fixSalesforceTokenExpiry(tok *oauth2.Token) {
	if tok == nil {
		return
	}

	if !tok.Expiry.IsZero() {
		return
	}

	// Extract issued_at from the raw token map.
	issuedAtStr, ok := tok.Extra("issued_at").(string)
	if !ok {
		return
	}

	// Salesforce sends issued_at in milliseconds as a string
	ms, err := strconv.ParseInt(issuedAtStr, 10, 64)
	if err != nil {
		return
	}

	// Set Expiry.
	const sessionLength = 2 * time.Hour
	tok.Expiry = time.UnixMilli(ms).Add(sessionLength)
}
