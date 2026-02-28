package config

import "testing"

func TestConfig(t *testing.T) {

	config, err := Load("config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}

	// file possibility: "./reconciliation.db"
	if got, want := config.DatabasePath, ":memory:"; got != want {
		t.Errorf("got %s want %s", got, want)
	}

	if got, want := config.Web.XeroCallBackAddr, "http://localhost:8080/xero/callback"; got != want {
		t.Errorf("config.Web.XeroCallBackAddr got %q want %q", got, want)
	}

	if got, want := config.Web.SalesforceCallBackAddr, "http://localhost:8080/salesforce/callback"; got != want {
		t.Errorf("config.Web.XeroCallBackAddr got %q want %q", got, want)
	}

	config.Web.ListenAddress = "127.0.0.2:9001"
	if err := validateAndPrepare(config); err == nil {
		t.Errorf("expected error for invalid address %q", config.Web.ListenAddress)
	}
	config.Web.ListenAddress = "127.0.0.1:9001"

	q := config.Salesforce.Query
	config.Salesforce.Query = `SELECT something FROM otherthing WHERE {{.WhereClause}}`
	if err := validateAndPrepare(config); err == nil {
		t.Errorf("expected error for query with WHERE (%q)", config.Salesforce.Query)
	}
	config.Salesforce.Query = q

}

func TestConfigRegexp(t *testing.T) {

	c := &Config{
		DonationAccountPrefixes: []string{"a", "b", "c"},
	}
	if got, want := c.DonationAccountCodesRegex(), "^(a|b|c)"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if c.DonationAccountCodesAsRegex() == nil {
		t.Error("unexpected c.DonationAccountCodesARegex error")
	}
	c.DonationAccountPrefixes = []string{"(xn", "fail"}
	if c.DonationAccountCodesAsRegex() != nil {
		t.Error("expected c.DonationAccountCodesARegex error")
	}
}
