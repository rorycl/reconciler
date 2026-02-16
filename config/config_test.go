package config

import "testing"

func TestConfig(t *testing.T) {

	config, err := Load("config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}

	if got, want := config.DatabasePath, "./reconciliation.db"; got != want {
		t.Errorf("got %s want %s", got, want)
	}

	if got, want := config.Web.XeroCallBackAddr, "http://localhost:8080/xero/callback"; got != want {
		t.Errorf("config.Web.XeroCallBackAddr got %q want %q", got, want)
	}

	if got, want := config.Web.SalesforceCallBackAddr, "http://localhost:8080/salesforce/callback"; got != want {
		t.Errorf("config.Web.XeroCallBackAddr got %q want %q", got, want)
	}

	if got, want := config.Xero.PKCEEnabled, true; got != want {
		t.Errorf("config.Xero.PKCEEnabled got %t want %t", got, want)
	}

	config.Web.ListenAddress = "127.0.0.2:9001"
	if err := validateAndPrepare(config); err == nil {
		t.Errorf("expected error for invalid address %q", config.Web.ListenAddress)
	}
	config.Web.ListenAddress = "127.0.0.1:9001"

	config.Xero.TokenTimeout = "25h"
	if err := validateAndPrepare(config); err != nil {
		t.Errorf("unexpected error for token timeout %v", err)
	}

	config.Xero.TokenTimeout = "not valid"
	if err := validateAndPrepare(config); err == nil {
		t.Errorf("expected error for invalid token timeout %q", config.Xero.TokenTimeout)
	}

}
