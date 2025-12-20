package app

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"gopkg.in/yaml.v2"
)

// RootConfig represents the top-level structure of the config.yaml file.
type RootConfig struct {
	Salesforce     SalesforceConfig `yaml:"salesforce"`
	TokenFilePath  string           `yaml:"token_file_path"`
	DatabasePath   string           `yaml:"database_path"`
	DateRangeStart time.Time

	// Internal representation of the date string from YAML
	DateRangeStartStr string `yaml:"date_range_start"`
}

// SalesforceConfig holds Salesforce-specific settings, including the query configuration.
type SalesforceConfig struct {
	LoginDomain   string            `yaml:"login_domain"`
	ClientID      string            `yaml:"client_id"`
	ClientSecret  string            `yaml:"client_secret"`
	Query         string            `yaml:"query"`
	FieldMappings map[string]string `yaml:"field_mappings"`
	OAuth2Config  *oauth2.Config
}

// LoadConfig loads and validates the configuration from the given file path.
func LoadConfig(filePath string) (*RootConfig, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file does not exist: %s", filePath)
	}

	configFile, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg RootConfig
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
func validateAndPrepareConfig(c *RootConfig) error {
	sc := &c.Salesforce
	if sc.ClientID == "" {
		return fmt.Errorf("salesforce.client_id is missing")
	}
	if sc.ClientSecret == "" {
		return fmt.Errorf("salesforce.client_secret is missing")
	}
	if sc.LoginDomain == "" {
		return fmt.Errorf("salesforce.login_domain is missing")
	}
	if sc.Query == "" {
		return fmt.Errorf("salesforce.query is missing")
	}
	if !strings.Contains(sc.Query, "{{.WhereClause}}") {
		return fmt.Errorf("salesforce.query must contain the '{{.WhereClause}}' placeholder")
	}
	if c.TokenFilePath == "" {
		return fmt.Errorf("token_file_path is missing")
	}
	if c.DatabasePath == "" {
		return fmt.Errorf("database_path is missing")
	}
	if c.DateRangeStartStr == "" {
		return fmt.Errorf("date_range_start is missing")
	}

	parsedDate, err := time.Parse("2006-01-02", c.DateRangeStartStr)
	if err != nil {
		return fmt.Errorf("invalid date_range_start date format: %w", err)
	}
	c.DateRangeStart = parsedDate

	// Configure the OAuth2 client for Salesforce, including PKCE support
	sc.OAuth2Config = &oauth2.Config{
		ClientID:     sc.ClientID,
		ClientSecret: sc.ClientSecret,
		RedirectURL:  "http://localhost:8080/sf-callback",
		Endpoint: oauth2.Endpoint{
			AuthURL:  fmt.Sprintf("https://%s/services/oauth2/authorize", sc.LoginDomain),
			TokenURL: fmt.Sprintf("https://%s/services/oauth2/token", sc.LoginDomain),
		},
		Scopes: []string{"api", "refresh_token"},
	}
	return nil
}
