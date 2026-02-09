package salesforce

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"reconciler/config"

	"golang.org/x/oauth2"
)

// SalesforceAPIVersionNumber sets out the currently supported
// Salesforce API used for this client.
const SalesforceAPIVersionNumber = "v65.0"

// TokenCache is a helper struct to reliably save and load the OAuth2 token
// and the critical instance_url from a file.
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

	return tok, nil
}

// ValueStorer is an interface for storing variables. Typically this will be implemented
// by a session store such as `github.com/alexedwards/scs/v2`.
type valueStorer interface {
	Put(ctx context.Context, key string, val any)
	GetString(ctx context.Context, key string) string
}

type webServerError interface {
	serverError(w http.ResponseWriter, r *http.Request, errs ...error)
}

// InitiateWebLogin is an http.Handler for preparing a Salesforce OAuth2
// flow from a web interface.
func InitiateWebLogin(cfg *config.Config, vs valueStorer, log *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		verifier := oauth2.GenerateVerifier()
		authURL := cfg.Salesforce.OAuth2Config.AuthCodeURL(
			"state-string", oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(verifier),
		)
		vs.Put(ctx, "verifier", verifier) // put to session
		http.Redirect(w, r, authURL, http.StatusSeeOther)
	})
}

// WebLoginCallBack is an http.Handler for receiving a web callback initiated from a web
// interface.
// Todo: consider injecting the logger and web error function.
func WebLoginCallBack(cfg *config.Config, vs valueStorer) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		verifier := vs.GetString("verifier") // retrieve from session.
		if verifier == "" {
			http.Error(w, "could not get verifier code from session", http.StatusInternalServerError)
			return
		}

		// Extract the code from the api platform's response.
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "did not get code in OAuth2 response", http.StatusInternalServerError)
			return
		}

		// Check that the token could be verified using the PKCE code verifier.
		tok, err := cfg.Salesforce.OAuth2Config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to exchange authorization code for token: %v", err), http.StatusInternalServerError)
			return
		}

		// Salesforce requires an "instance_url" to be extracted from the token.
		instanceURL, ok := tok.Extra("instance_url").(string)
		if !ok || instanceURL == "" {
			http.Error(w, fmt.Sprintff("oauth token did not contain the required 'instance_url'"), http.StatusInternalServerError)
			return
		}

		// Save the token with the instance url.
		cache := &TokenCache{Token: tok, InstanceURL: instanceURL}
		if err := SaveTokenCacheToFile(cache, cfg.Salesforce.TokenFilePath); err != nil {
			http.Error(w, fmt.Sprintf("failed to save new token: %v", err), http.StatusInternalServerError)
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

// TokenIsValid loads a token from file and checks if it is valid.
func TokenIsValid(path string) bool {
	token, err := LoadTokenCacheFromFile(path)
	if err != nil {
		return false
	}
	return token.Token.Valid()
}
