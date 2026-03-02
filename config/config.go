package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"gopkg.in/yaml.v2"
)

const (
	XeroAuthURL  = "https://login.xero.com/identity/connect/authorize"
	XeroTokenURL = "https://identity.xero.com/connect/token"
)

// Todo: consider making OAuth2Config components injectable or come from the
// configuration file.

// Config represents the entire application configuration.
type Config struct {
	Organisation            string   `yaml:"organisation_name"`
	DataStartDateStr        string   `yaml:"data_date_start"`
	DonationAccountPrefixes []string `yaml:"donation_account_prefixes"`
	InDevelopmentMode       bool     `yaml:"-"`

	// subsections
	Web           WebConfig        `yaml:"web"`
	Xero          XeroConfig       `yaml:"xero"`
	Salesforce    SalesforceConfig `yaml:"salesforce"`
	DataStartDate time.Time        // Parsed from DataStartDateStr
}

// WebConfig holds settings specific to the web server.
// This includes the Xero and Salesforce OAuth2 callback urls.
type WebConfig struct {
	// Mandatory settings
	ListenAddress      string `yaml:"listen_address"`
	XeroCallBack       string `yaml:"xero_oauth2_callback"`
	SalesforceCallBack string `yaml:"salesforce_oauth2_callback"`
	// full addresses to callbacks
	XeroCallBackAddr       string
	SalesforceCallBackAddr string
}

// XeroConfig holds Xero-specific settings.
type XeroConfig struct {
	ClientID     string   `yaml:"client_id"`
	ClientSecret string   `yaml:"client_secret"`
	Scopes       []string `yaml:"-"`
	OAuth2Config *oauth2.Config
}

// SalesforceConfig holds Salesforce-specific settings.
type SalesforceConfig struct {
	LoginDomain  string   `yaml:"login_domain"`
	ClientID     string   `yaml:"client_id"`
	ClientSecret string   `yaml:"client_secret"`
	Scopes       []string `yaml:"-"`
	OAuth2Config *oauth2.Config
	// SOQL settings.
	Query            string            `yaml:"query"`
	FieldMappings    map[string]string `yaml:"field_mappings"`
	LinkingObject    string            `yaml:"linking_object"`
	LinkingFieldName string            `yaml:"linking_field_name"`
}

// Load loads and validates the configuration from the given file path.
func Load(filePath string) (*Config, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file does not exist: %s", filePath)
	}

	configFile, err := os.ReadFile(filePath)
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
	if c.Organisation == "" {
		return errors.New("organisation_name is missing")
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
	// check the accounts regexp compiles.
	if r := c.DonationAccountCodesAsRegex(); r == nil {
		return fmt.Errorf("accounts regexp did not compile: %v", c.DonationAccountCodesRegex())
	}

	// Web
	if c.Web.ListenAddress == "" {
		return errors.New("web.listen_address is missing")
	}
	if !strings.Contains(c.Web.ListenAddress, "127.0.0.1") && !strings.Contains(c.Web.ListenAddress, "localhost") {
		return errors.New("web.listen_address must be 127.0.0.1 or localhost")
	}
	if c.Web.XeroCallBack == "" {
		return errors.New("web.xero_oauth2_callback is missing")
	}
	if c.Web.SalesforceCallBack == "" {
		return errors.New("web.salesforce_oauth2_callback is missing")
	}

	// The full callback addresses are local (http rather than https) addresses.
	c.Web.XeroCallBackAddr, err = url.JoinPath(
		fmt.Sprintf("http://%s", c.Web.ListenAddress),
		c.Web.XeroCallBack,
	)
	if err != nil {
		return fmt.Errorf("could not create full xero callback address: %w", err)
	}
	c.Web.SalesforceCallBackAddr, err = url.JoinPath(
		fmt.Sprintf("http://%s", c.Web.ListenAddress),
		c.Web.SalesforceCallBack,
	)
	if err != nil {
		return fmt.Errorf("could not create full xero callback address: %w", err)
	}

	// Xero
	xc := &c.Xero
	if xc.ClientID == "" {
		return errors.New("xero.client_id is missing")
	}
	if xc.ClientSecret != "" {
		return errors.New("xero.client_secret should not be provided for Xero PKCE connections")
	}
	// Restrictive read-only scopes (for apps created from March 2, 2026).
	// See https://developer.xero.com/documentation/guides/oauth2/scopes/#organisation-scopes
	xc.Scopes = []string{
		"accounting.invoices.read",
		"accounting.banktransactions.read",
		"accounting.settings.read",
		"offline_access",
	}

	if len(xc.Scopes) < 1 {
		return errors.New("xero.scopes not defined")
	}
	if !slices.Contains(xc.Scopes, "offline_access") {
		return errors.New("xero.scopes does not contain 'offline_access' scope")
	}

	xc.OAuth2Config = &oauth2.Config{
		ClientID:     xc.ClientID,
		ClientSecret: xc.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  XeroAuthURL,
			TokenURL: XeroTokenURL,
		},
		RedirectURL: c.Web.XeroCallBackAddr,
		Scopes:      xc.Scopes,
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
	if sc.Query == "" {
		return errors.New("salesforce.query is missing")
	}
	if strings.Contains(strings.ToLower(sc.Query), "where") {
		return errors.New("salesforce.query may not provide a WHERE clause which is added by the program")
	}
	sc.Query += "\n  WHERE {{.WhereClause}}"
	if sc.LinkingObject == "" {
		return errors.New("salesforce.linking_object is missing")
	}
	if sc.LinkingFieldName == "" {
		return errors.New("salesforce.linking_field_name is missing")
	}
	// Required salesforce scopes.
	sc.Scopes = []string{
		"api",
		"refresh_token",
	}
	if !slices.Contains(sc.Scopes, "api") {
		return errors.New("salesforce.scopes does not contain 'api' scope")
	}
	if !slices.Contains(sc.Scopes, "refresh_token") {
		return errors.New("salesforce.scopes does not contain 'refresh_token' scope")
	}
	sc.OAuth2Config = &oauth2.Config{
		ClientID:     sc.ClientID,
		ClientSecret: sc.ClientSecret,
		RedirectURL:  c.Web.SalesforceCallBackAddr,
		Endpoint: oauth2.Endpoint{
			AuthURL:  fmt.Sprintf("https://%s/services/oauth2/authorize", sc.LoginDomain),
			TokenURL: fmt.Sprintf("https://%s/services/oauth2/token", sc.LoginDomain),
		},
		Scopes: sc.Scopes,
	}

	return nil
}

// DonationAccountCodesRegex returns the donation account prefixes as a
// string suitable for a regex expression for SQLite.
func (c *Config) DonationAccountCodesRegex() string {
	return fmt.Sprintf("^(%s)", strings.Join(c.DonationAccountPrefixes, "|"))
}

// DonationAccountCodesAsRegex returns the donation account prefixes as a
// compiled regex version of DonationAccountCodesRegex. A nil regexp is an error.
func (c *Config) DonationAccountCodesAsRegex() *regexp.Regexp {
	r, _ := regexp.Compile(c.DonationAccountCodesRegex())
	return r
}
