package app

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"golang.org/x/oauth2"
	"gopkg.in/yaml.v2"
)

// Config represents the application configuration from a YAML file.
type Config struct {
	ClientID           string `yaml:"client_id"`
	ClientSecret       string `yaml:"client_secret"`
	TokenFilePath      string `yaml:"token_file_path"`
	DatabasePath       string `yaml:"database_path"`
	FinancialYearStart time.Time
	OAuth2Config       *oauth2.Config

	// Internal representation of the date string from YAML
	FinancialYearStartStr string `yaml:"financial_year_start"`
}

// LoadConfig loads and validates the configuration from the given file path.
func LoadConfig(filePath string) (*Config, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file does not exist: %s", filePath)
	}

	configFile, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	err = yaml.Unmarshal(configFile, &cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to parse YAML config file: %w", err)
	}

	if err := validateAndPrepareConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validateAndPrepareConfig checks for required fields and sets up derived values.
func validateAndPrepareConfig(c *Config) error {
	if c.ClientID == "" || c.ClientID == "YOUR_XERO_CLIENT_ID" {
		return fmt.Errorf("client_id is missing from config file")
	}
	if c.ClientSecret == "" || c.ClientSecret == "YOUR_XERO_CLIENT_SECRET" {
		return fmt.Errorf("client_secret is missing from config file")
	}
	if c.TokenFilePath == "" {
		return fmt.Errorf("token_file_path is missing from config file")
	}
	if c.DatabasePath == "" {
		return fmt.Errorf("database_path is missing from config file")
	}
	if c.FinancialYearStartStr == "" {
		return fmt.Errorf("financial_year_start is missing from config file")
	}

	parsedDate, err := time.Parse("2006-01-02", c.FinancialYearStartStr)
	if err != nil {
		return fmt.Errorf("invalid financial_year_start date format: %w", err)
	}
	c.FinancialYearStart = parsedDate

	// Configure the OAuth2 client
	c.OAuth2Config = &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://login.xero.com/identity/connect/authorize",
			TokenURL: "https://identity.xero.com/connect/token",
		},
		RedirectURL: "http://localhost:8080/callback",
		Scopes: []string{
			"accounting.transactions", // Read/write access for invoices and bank transactions
			"accounting.settings.read",  // To get organisation details
			"offline_access",          // To get a refresh token
		},
	}
	return nil
}
