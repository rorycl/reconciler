package salesforce

import (
	"errors"
	"os"
	"testing"
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
