package config

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/oauth2"
)

func TestConfig(t *testing.T) {

	config, err := Load("config.example.yaml")
	if err != nil {
		t.Fatal(err)
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

/*
// litterOutput provides a way of dumping a struct.
func litterOutput(data any) string {
	// https://github.com/sanity-io/litter/issues/12#issuecomment-1144643251
	litter.Config.FormatTime = true
	// litter.Config.FieldExclusions = regexp.MustCompile("^(Reader|Encoding)$")
	litter.Config.DisablePointerReplacement = true
	return litter.Sdump(data)
}
*/

func TestConfigDumpFile(t *testing.T) {

	got, err := Load("config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}

	want := &Config{
		Organisation:     "My Organisation",
		DataStartDateStr: "2025-04-01",
		DonationAccountPrefixes: []string{
			"53",
			"55",
			"57",
		},
		Web: WebConfig{
			ListenAddress:          "localhost:8080",
			XeroCallBack:           "/xero/callback",
			SalesforceCallBack:     "/salesforce/callback",
			XeroCallBackAddr:       "http://localhost:8080/xero/callback",
			SalesforceCallBackAddr: "http://localhost:8080/salesforce/callback",
		},
		Xero: XeroConfig{
			ClientID:     "XERO_CLIENT_ID",
			ClientSecret: "",
			Scopes: []string{ // p0
				"accounting.invoices.read",
				"accounting.banktransactions.read",
				"accounting.settings.read",
				"offline_access",
			},
			OAuth2Config: &oauth2.Config{
				ClientID:     "XERO_CLIENT_ID",
				ClientSecret: "",
				Endpoint: oauth2.Endpoint{
					AuthURL:       "https://login.xero.com/identity/connect/authorize",
					DeviceAuthURL: "",
					TokenURL:      "https://identity.xero.com/connect/token",
					AuthStyle:     0,
				},
				RedirectURL: "http://localhost:8080/xero/callback",
				Scopes: []string{ // p0
					"accounting.invoices.read",
					"accounting.banktransactions.read",
					"accounting.settings.read",
					"offline_access",
				},
			},
		},
		Salesforce: SalesforceConfig{
			LoginDomain:  "test.salesforce.com",
			ClientID:     "SALESFORCE_CONSUMER_KEY",
			ClientSecret: "SALESFORCE_CONSUMER_SECRET",
			Scopes: []string{ // p1
				"api",
				"refresh_token",
			},
			OAuth2Config: &oauth2.Config{
				ClientID:     "SALESFORCE_CONSUMER_KEY",
				ClientSecret: "SALESFORCE_CONSUMER_SECRET",
				Endpoint: oauth2.Endpoint{
					AuthURL:       "https://test.salesforce.com/services/oauth2/authorize",
					DeviceAuthURL: "",
					TokenURL:      "https://test.salesforce.com/services/oauth2/token",
					AuthStyle:     0,
				},
				RedirectURL: "http://localhost:8080/salesforce/callback",
				Scopes: []string{ // p1
					"api",
					"refresh_token",
				},
			},
			Query: "SELECT\n  Id, Name, Amount, CloseDate, LastModifiedDate, Payout_Reference__c,\n  StageName, RecordType.Name, Account.Name,\n  CreatedBy.Name, CreatedDate, LastModifiedBy.Name\nFROM Opportunity\n  WHERE {{.WhereClause}}",
			FieldMappings: map[string]string{
				"Account.Name":        "Account",
				"CreatedBy.Name":      "CreatedBy",
				"LastModifiedBy.Name": "ModifiedBy",
				"StageName":           "Stage",
			},
			LinkingObject:    "Opportunity",
			LinkingFieldName: "Payout_Reference__c",
		},
		DataStartDate: time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC),
	}

	if diff := cmp.Diff(got, want, cmpopts.IgnoreUnexported(Config{}, oauth2.Config{})); diff != "" {
		t.Errorf("unexpected diff:\n%s", diff)
	}

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
