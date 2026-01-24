package salesforce

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestTypesOK(t *testing.T) {
	b, err := os.ReadFile("testdata/salesforce_response.json")
	if err != nil {
		t.Fatal(err)
	}

	fieldMappings := map[string]string{
		"StageName":           "Stage",
		"Account.Name":        "Account",
		"CreatedBy.Name":      "CreatedBy",
		"LastModifiedBy.Name": "ModifiedBy",
	}

	unmarshaller := SOQLUnmarshaller{Mapper: fieldMappings}
	sr, err := unmarshaller.UnmarshalSOQLResponse(b)
	if err != nil {
		t.Fatalf("UnmarshalSOQLResponse error: %v", err)
	}

	if got, want := len(sr.Records), 34; got != want {
		t.Errorf("got %d records, want %d", got, want)
	}

	if sr.Records[0].AdditionalFields["Account"] == nil {
		t.Error("expected 'Account' field to be mapped, but it was nil")
	}
}

func TestTypesExtended(t *testing.T) {
	b, err := os.ReadFile("testdata/salesforce_response_extended.json")
	if err != nil {
		t.Fatal(err)
	}

	fieldMappings := map[string]string{
		"StageName":           "Stage",
		"Account.Name":        "Account",
		"CreatedBy.Name":      "CreatedBy",
		"LastModifiedBy.Name": "ModifiedBy",
	}

	unmarshaller := SOQLUnmarshaller{Mapper: fieldMappings}
	sr, err := unmarshaller.UnmarshalSOQLResponse(b)
	if err != nil {
		t.Fatalf("UnmarshalSOQLResponse error: %v", err)
	}

	if got, want := len(sr.Records), 3; got != want {
		t.Fatalf("got %d records, want %d", got, want)
	}

	ptrStr := func(s string) *string {
		return &s
	}

	expectedThirdRecord := Record{
		CoreFields: CoreFields{
			ID:               "006gL00000EsB99QAF",
			Name:             "Express Logistics Standby Generator",
			Amount:           220000,
			CloseDate:        SalesforceDate{time.Date(2025, time.August, 18, 0, 0, 0, 0, time.UTC)},
			CreatedDate:      SalesforceTime{time.Date(2025, time.November, 27, 10, 21, 45, 0, time.Local)},
			LastModifiedDate: SalesforceTime{time.Date(2025, time.December, 20, 20, 21, 50, 0, time.Local)},
			CreatedBy: struct {
				Name string "json:\"Name\""
			}{Name: "OrgFarm EPIC"},
			LastModifiedBy: struct {
				Name string "json:\"Name\""
			}{Name: "Rory Campbell-Lange"},
			PayoutReference: ptrStr("ENTH-20251112"),
		},
		AdditionalFields: map[string]any{
			"Account": "Express Logistics and Transport",
			"Account.Attributes": map[string]any{
				"type": "Account",
				"url":  "/services/data/v65.0/sobjects/Account/001gL00000WwNI0QAN",
			},
			"CreatedBy": string("OrgFarm EPIC"),
			"CreatedBy.Attributes": map[string]any{
				"type": string("User"),
				"url":  string("/services/data/v65.0/sobjects/User/005gL00000B1Y1JQAV"),
			},
			"LastModifiedBy.Attributes": map[string]any{
				"type": string("User"),
				"url":  string("/services/data/v65.0/sobjects/User/005gL00000BOwSkQAL"),
			},
			"MadeUpValue":         string("789"),
			"ModifiedBy":          string("Rory Campbell-Lange"),
			"Payout_Reference__c": "ENTH-20251112",
			"RecordType":          nil,
			"Stage":               "Closed Won",
			"Attributes.Type":     "Opportunity",
			"Attributes.Url":      "/services/data/v65.0/sobjects/Opportunity/006gL00000EsB99QAF",
		},
	}

	if diff := cmp.Diff(expectedThirdRecord, sr.Records[2]); diff != "" {
		t.Errorf("unexpected third record diff:\n%v", diff)
	}
}

func TestTypesFail(t *testing.T) {
	var euf *ErrUnmarshallFieldNotFoundError

	b, err := os.ReadFile("testdata/salesforce_response.json")
	if err != nil {
		t.Fatal(err)
	}

	fieldMappings := map[string]string{
		"Invalid.Name": "Invalid", // subkey
	}

	unmarshaller := SOQLUnmarshaller{Mapper: fieldMappings}
	_, err = unmarshaller.UnmarshalSOQLResponse(b)
	if !errors.As(err, &euf) {
		t.Fatalf("expected fieldMapping error not triggered")
	}

	fieldMappings = map[string]string{
		"DoesNotExist": "dne", // key
	}

	unmarshaller = SOQLUnmarshaller{Mapper: fieldMappings}
	_, err = unmarshaller.UnmarshalSOQLResponse(b)
	if !errors.As(err, &euf) {
		t.Fatalf("expected fieldMapping error not triggered")
	}
}
