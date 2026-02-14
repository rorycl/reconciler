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
		"StageName":    "Stage",
		"Account.Name": "Account",
	}

	unmarshaller := SOQLUnmarshaller{Mapper: fieldMappings}
	sr, err := unmarshaller.UnmarshalSOQLResponse(b)
	if err != nil {
		t.Fatalf("UnmarshalSOQLResponse error: %v", err)
	}

	if got, want := len(sr.Donations), 34; got != want {
		t.Errorf("got %d records, want %d", got, want)
	}

	if sr.Donations[0].AdditionalFields["Account"] == nil {
		t.Error("expected 'Account' field to be mapped, but it was nil")
	}
}

func TestTypesExtended(t *testing.T) {
	b, err := os.ReadFile("testdata/salesforce_response_extended.json")
	if err != nil {
		t.Fatal(err)
	}

	fieldMappings := map[string]string{
		"StageName":    "Stage",
		"Account.Name": "Account",
	}

	unmarshaller := SOQLUnmarshaller{Mapper: fieldMappings}
	sr, err := unmarshaller.UnmarshalSOQLResponse(b)
	if err != nil {
		t.Fatalf("UnmarshalSOQLResponse error: %v", err)
	}

	if got, want := len(sr.Donations), 3; got != want {
		t.Fatalf("got %d records, want %d", got, want)
	}

	ptrStr := func(s string) *string {
		return &s
	}

	expectedThirdDonation := Donation{
		CoreFields: CoreFields{
			ID:               "006gL00000EsB99QAF",
			Name:             "Express Logistics Standby Generator",
			Amount:           220000,
			CloseDate:        SalesforceDate{time.Date(2025, time.August, 18, 0, 0, 0, 0, time.UTC)},
			CreatedDate:      SalesforceTime{time.Date(2025, time.November, 27, 10, 21, 45, 0, time.Local)},
			LastModifiedDate: SalesforceTime{time.Date(2025, time.December, 20, 20, 21, 50, 0, time.Local)},
			CreatedBy:        FlattenedName("OrgFarm EPIC"),
			LastModifiedBy:   FlattenedName("Test User"),
			PayoutReference:  ptrStr("ENTH-20251112"),
		},
		AdditionalFields: map[string]any{
			"Account":         "Express Logistics and Transport",
			"MadeUpValue":     "789",
			"Stage":           "Closed Won",
			"Attributes.Type": "Opportunity",
			"Attributes.Url":  "/services/data/v65.0/sobjects/Opportunity/006gL00000EsB99QAF",
		},
	}

	if diff := cmp.Diff(expectedThirdDonation, sr.Donations[2]); diff != "" {
		t.Errorf("unexpected third record diff:\n%v", diff)
	}
}

func TestTypesFail(t *testing.T) {
	var euf *ErrUnmarshallFieldNotFoundError

	SOQLStrictMapping = true
	t.Cleanup(func() { SOQLStrictMapping = false })

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
