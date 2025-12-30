package xero

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
)

var connectionsURL = "https://api.xero.com/connections"

// NewClient handles the OAuth2 flow to return an authenticated http.Client.
// It attempts to use a saved token first and will refresh it if necessary.
// If no token exists, it will fail, requiring the user to run the `login` command.
func NewClient(ctx context.Context, cfg *oauth2.Config, tokenPath string) (*APIClient, error) {
	tok, err := loadTokenFromFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("no token file found at '%s'. Please run the 'login' command first", tokenPath)
	}

	tokenSource := cfg.TokenSource(ctx, tok)

	// Check if the token was refreshed and save the new one if it was.
	refreshedToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	if refreshedToken.AccessToken != tok.AccessToken {
		log.Println("Access token was refreshed. Saving new token.")
		if err := saveTokenToFile(refreshedToken, tokenPath); err != nil {
			return nil, fmt.Errorf("failed to save refreshed token: %w", err)
		}
	}

	oauthClient := oauth2.NewClient(ctx, tokenSource)

	tenantID, err := getTenantID(ctx, oauthClient)
	if err != nil {
		return nil, fmt.Errorf("failed to determine tenant ID: %w", err)
	}

	return NewAPIClient(tenantID, oauthClient), nil
}

// InitiateLogin starts the interactive OAuth2 flow to get a new token from the web.
// It saves the new token to the specified path upon success.
func InitiateLogin(ctx context.Context, cfg *oauth2.Config, tokenPath string) error {
	tok, err := getNewTokenFromWeb(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to get new token: %w", err)
	}
	if err := saveTokenToFile(tok, tokenPath); err != nil {
		return fmt.Errorf("failed to save new token: %w", err)
	}
	log.Println("Login successful. Token saved.")
	return nil
}

// getNewTokenFromWeb starts a temporary web server to handle the OAuth2 callback.
func getNewTokenFromWeb(ctx context.Context, cfg *oauth2.Config) (*oauth2.Token, error) {
	codeChan := make(chan string)
	errChan := make(chan error)
	server := &http.Server{Addr: ":8080"}

	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
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

	authURL := cfg.AuthCodeURL("state-string", oauth2.AccessTypeOffline)
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

	tok, err := cfg.Exchange(ctx, authCode)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange authorization code for token: %w", err)
	}

	return tok, nil
}

// loadTokenFromFile reads an OAuth2 token from a JSON file.
func loadTokenFromFile(path string) (*oauth2.Token, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// saveTokenToFile writes an OAuth2 token to a JSON file with secure permissions.
func saveTokenToFile(token *oauth2.Token, path string) error {
	log.Printf("Saving token to %s", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("unable to cache oauth token: %w", err)
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(token)
}

// DeleteToken removes the token file from disk.
func DeleteToken(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// getTenantID fetches the list of connections and returns the first TenantID found.
func getTenantID(ctx context.Context, client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", connectionsURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create connections request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get connections: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error getting connections (status %d)", resp.StatusCode)
	}

	var connections []Connection
	if err := json.NewDecoder(resp.Body).Decode(&connections); err != nil {
		return "", fmt.Errorf("failed to decode connections response: %w", err)
	}

	if len(connections) == 0 {
		return "", fmt.Errorf("no tenants found for this connection")
	}

	return connections[0].TenantID, nil
}
