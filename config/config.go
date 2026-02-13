package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"gopkg.in/yaml.v2"
)

// Todo: consider making OAuth2Config components injectable or come from the
// configuration file.

// Config represents the entire application configuration.
type Config struct {
	DatabasePath            string           `yaml:"database_path"`
	Web                     WebConfig        `yaml:"web"`
	DataStartDateStr        string           `yaml:"data_date_start"`
	DonationAccountPrefixes []string         `yaml:"donation_account_prefixes"`
	Xero                    XeroConfig       `yaml:"xero"`
	Salesforce              SalesforceConfig `yaml:"salesforce"`
	DataStartDate           time.Time        // Parsed from DataStartDateStr
}

// WebConfig holds settings specific to the web server.
type WebConfig struct {
	TemplatesPath      string `yaml:"templates_path"`
	StaticPath         string `yaml:"static_path"`
	ListenAddress      string `yaml:"listen_address"`
	XeroCallBack       string `yaml:"xero_callback"`
	SalesforceCallBack string `yaml:"salesforce_callback"`
	DevelopmentMode    bool   `yaml:"development_mode"`
}

// XeroConfig holds Xero-specific settings.
type XeroConfig struct {
	ClientID      string `yaml:"client_id"`
	ClientSecret  string `yaml:"client_secret"`
	TokenFilePath string `yaml:"token_file_path"`
	OAuth2Config  *oauth2.Config
}

// SalesforceConfig holds Salesforce-specific settings.
type SalesforceConfig struct {
	LoginDomain      string            `yaml:"login_domain"`
	ClientID         string            `yaml:"client_id"`
	ClientSecret     string            `yaml:"client_secret"`
	TokenFilePath    string            `yaml:"token_file_path"`
	Query            string            `yaml:"query"`
	FieldMappings    map[string]string `yaml:"field_mappings"`
	LinkingObject    string            `yaml:"linking_object"`
	LinkingFieldName string            `yaml:"linking_field_name"`
	OAuth2Config     *oauth2.Config
}

// Load loads and validates the configuration from the given file path.
func Load(filePath string) (*Config, error) {
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

	if err := validateAndPrepare(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validateAndPrepare checks for required fields and sets up derived values.
func validateAndPrepare(c *Config) error {
	// General
	if c.DatabasePath == "" {
		return errors.New("database_path is missing")
	}
	if c.DataStartDateStr == "" {
		return errors.New("date_range_start is missing")
	}
	parsedDate, err := time.Parse("2006-01-02", c.DataStartDateStr)
	if err != nil {
		return fmt.Errorf("invalid date_range_start format: %w", err)
	}
	c.DataStartDate = parsedDate
	if len(c.DonationAccountPrefixes) < 1 {
		return errors.New("at least one donation_account_prefix should be supplied")
	}

	// Web
	if c.Web.TemplatesPath == "" {
		return errors.New("web.templates_path is missing")
	}
	if c.Web.StaticPath == "" {
		return errors.New("web.static_path is missing")
	}
	if c.Web.ListenAddress == "" {
		return errors.New("web.listen_address is missing")
	}
	if c.Web.XeroCallBack == "" {
		return errors.New("web.xero_callback is missing")
	}
	if c.Web.SalesforceCallBack == "" {
		return errors.New("web.salesforce_callback is missing")
	}

	// Xero
	xc := &c.Xero
	if xc.ClientID == "" {
		return errors.New("xero.client_id is missing")
	}
	if xc.ClientSecret == "" {
		return errors.New("xero.client_secret is missing")
	}
	if xc.TokenFilePath == "" {
		return errors.New("xero.token_file_path is missing")
	}
	xc.OAuth2Config = &oauth2.Config{
		ClientID:     xc.ClientID,
		ClientSecret: xc.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://login.xero.com/identity/connect/authorize",
			TokenURL: "https://identity.xero.com/connect/token",
		},
		RedirectURL: "http://localhost:8080/xero/callback",
		Scopes:      []string{"accounting.transactions", "accounting.settings.read", "offline_access"},
	}

	// Salesforce
	sc := &c.Salesforce
	if sc.ClientID == "" {
		return errors.New("salesforce.client_id is missing")
	}
	if sc.ClientSecret == "" {
		return errors.New("salesforce.client_secret is missing")
	}
	if sc.LoginDomain == "" {
		return errors.New("salesforce.login_domain is missing")
	}
	if sc.TokenFilePath == "" {
		return errors.New("salesforce.token_file_path is missing")
	}
	if sc.Query == "" {
		return errors.New("salesforce.query is missing")
	}
	if !strings.Contains(sc.Query, "{{.WhereClause}}") {
		return errors.New("salesforce.query must contain '{{.WhereClause}}'")
	}
	if sc.LinkingObject == "" {
		return errors.New("salesforce.linking_object is missing")
	}
	if sc.LinkingFieldName == "" {
		return errors.New("salesforce.linking_field_name is missing")
	}
	sc.OAuth2Config = &oauth2.Config{
		ClientID:     sc.ClientID,
		ClientSecret: sc.ClientSecret,
		// RedirectURL:  "http://localhost:8080/salesforce/callback",
		RedirectURL: "http://localhost:8080/sf-callback",
		Endpoint: oauth2.Endpoint{
			AuthURL:  fmt.Sprintf("https://%s/services/oauth2/authorize", sc.LoginDomain),
			TokenURL: fmt.Sprintf("https://%s/services/oauth2/token", sc.LoginDomain),
		},
		Scopes: []string{"api", "refresh_token"},
	}

	return nil
}

// DonationAccountCodesRegex returns the donation account prefixes as a
// compiled regex string suitable for SQLite.
func (c *Config) DonationAccountCodesRegex() string {
	return fmt.Sprintf("^(%s)", strings.Join(c.DonationAccountPrefixes, "|"))
}
