package salesforce

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
	"sfcli/app" // Import the app package to access config structs
)

// tokenCache is a helper struct to reliably save and load the OAuth2 token
// and the critical instance_url from a file.
type tokenCache struct {
	Token       *oauth2.Token `json:"token"`
	InstanceURL string        `json:"instance_url"`
}

// NewClient handles the OAuth2 flow to return an authenticated Salesforce client.
func NewClient(ctx context.Context, cfg app.SalesforceConfig, tokenPath string) (*Client, error) {
	cache, err := loadTokenCacheFromFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("no token file found at '%s'. Please run the 'login' command first", tokenPath)
	}

	tokenSource := cfg.OAuth2Config.TokenSource(ctx, cache.Token)
	refreshedToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	if refreshedToken.AccessToken != cache.Token.AccessToken {
		log.Println("Access token was refreshed. Saving new token.")
		cache.Token = refreshedToken
		if err := saveTokenCacheToFile(cache, tokenPath); err != nil {
			return nil, fmt.Errorf("failed to save refreshed token: %w", err)
		}
	}

	oauthClient := oauth2.NewClient(ctx, tokenSource)
	return &Client{
		httpClient:  oauthClient,
		instanceURL: cache.InstanceURL,
		apiVersion:  "v59.0",
		config:      cfg,
	}, nil
}

// ... rest of auth.go is unchanged
// InitiateLogin, getNewTokenFromWeb, loadTokenCacheFromFile, saveTokenCacheToFile
