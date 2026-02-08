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

// tokenCache is a helper struct to reliably save and load the OAuth2 token
// and the critical instance_url from a file.
type tokenCache struct {
	Token       *oauth2.Token `json:"token"`
	InstanceURL string        `json:"instance_url"`
}

// NewClient handles the OAuth2 flow to return an authenticated Salesforce client.
func NewClient(ctx context.Context, cfg *config.Config) (*Client, error) {
	cache, err := loadTokenCacheFromFile(cfg.Salesforce.TokenFilePath)
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
		if err := saveTokenCacheToFile(cache, cfg.Salesforce.TokenFilePath); err != nil {
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

// InitiateLogin starts the interactive OAuth2 flow to get a new token
// from the web. It saves the new token and instance URL to the
// specified configuration path upon success.
func InitiateLogin(ctx context.Context, cfg *config.Config) error {
	tok, err := getNewTokenFromWeb(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to get new token: %w", err)
	}

	instanceURL, ok := tok.Extra("instance_url").(string)
	if !ok || instanceURL == "" {
		return fmt.Errorf("oauth token did not contain the required 'instance_url'")
	}

	cache := &tokenCache{Token: tok, InstanceURL: instanceURL}
	if err := saveTokenCacheToFile(cache, cfg.Salesforce.TokenFilePath); err != nil {
		return fmt.Errorf("failed to save new token: %w", err)
	}
	log.Println("Login successful. Token saved.")
	return nil
}

// getNewTokenFromWeb starts a temporary web server to handle the OAuth2 callback.
// It uses the PKCE extension for enhanced security.
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

// loadTokenCacheFromFile reads a token cache from a JSON file.
func loadTokenCacheFromFile(path string) (*tokenCache, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cache := &tokenCache{}
	err = json.NewDecoder(f).Decode(cache)
	return cache, err
}

// saveTokenCacheToFile writes a token cache to a JSON file with secure permissions.
func saveTokenCacheToFile(cache *tokenCache, path string) error {
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
	token, err := loadTokenCacheFromFile(path)
	if err != nil {
		return false
	}
	return token.Token.Valid()
}
