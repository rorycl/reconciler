package dbquery

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"gopkg.in/yaml.v2"
)

// Config represents the application configuration from a YAML file.
type Config struct {
	DatabasePath            string   `yaml:"database_path"`
	DonationAccountPrefixes []string `yaml:"donation_account_prefixes"`
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
	if c.DatabasePath == "" {
		return errors.New("database_path is missing from config file")
	}
	if _, err := os.Stat(c.DatabasePath); err != nil {
		return fmt.Errorf("file %q not found", c.DatabasePath)
	}
	if len(c.DonationAccountPrefixes) < 1 {
		return errors.New("at least one donation account prefix should be supplied")
	}
	return nil
}
