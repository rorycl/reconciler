package salesforce

import (
	"encoding/json"
	"os"
	"testing"
)

func TestTypes(t *testing.T) {

	b, err := os.ReadFile("testdata/salesforce_response.json")
	if err != nil {
		t.Fatal(err)
	}

	var sr SOQLResponse
	err = json.Unmarshal(b, &sr)
	if err != nil {
		t.Fatalf("json unmarshalling error: %v", err)
	}

	fieldMappings := map[string]string{
		"StageName":           "Stage",
		"Account.Name":        "Account",
		"CreatedBy.Name":      "CreatedBy",
		"LastModifiedBy.Name": "ModifiedBy",
	}
	if err := sr.MapAdditionalFields(fieldMappings); err != nil {
		t.Fatalf("mapping error: %v", err)
	}
	if got, want := len(sr.Records), 34; got != want {
		t.Errorf("got %d records, want %d", got, want)
	}
}
