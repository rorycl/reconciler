package salesforce

import (
	"encoding/json"
	"strings"
	"time"
)

// SalesforceTime is a custom type to handle Salesforce's specific datetime format.
type SalesforceTime struct {
	time.Time
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (st *SalesforceTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "null" || s == "" {
		return nil
	}
	// Handles Salesforce's custom format: "2025-07-14T02:25:51.000+0000"
	t, err := time.Parse("2006-01-02T15:04:05.000-0700", s)
	if err != nil {
		return err
	}
	st.Time = t
	return nil
}

// SOQLResponse is the top-level envelope for a SOQL query response.
type SOQLResponse struct {
	TotalSize      int      `json:"totalSize"`
	Done           bool     `json:"done"`
	NextRecordsURL string   `json:"nextRecordsUrl"`
	Records        []Record `json:"records"`
}

// CoreFields defines the essential, non-negotiable fields the application requires.
type CoreFields struct {
	ID               string         `json:"Id"`
	Name             string         `json:"Name"`
	Amount           float64        `json:"Amount"`
	CloseDate        string         `json:"CloseDate"`
	CreatedDate      SalesforceTime `json:"CreatedDate"`
	LastModifiedDate SalesforceTime `json:"LastModifiedDate"`
	CreatedBy        struct {
		Name string `json:"Name"`
	} `json:"CreatedBy"`
	LastModifiedBy struct {
		Name string `json:"Name"`
	} `json:"LastModifiedBy"`
	PayoutReference *string `json:"Payout_Reference__c"` // Pointer to handle null values
}

// Record represents the data for a single Salesforce record, combining core and additional fields.
type Record struct {
	CoreFields
	AdditionalFields map[string]interface{}
}

// UnmarshalJSON provides custom JSON decoding for the Record type.
// It populates the static CoreFields and captures all other fields into the dynamic AdditionalFields map.
func (r *Record) UnmarshalJSON(data []byte) error {
	// First, unmarshal into the embedded CoreFields struct to populate known fields.
	if err := json.Unmarshal(data, &r.CoreFields); err != nil {
		return err
	}

	// Second, unmarshal into a generic map to capture all fields from the response.
	var allFields map[string]interface{}
	if err := json.Unmarshal(data, &allFields); err != nil {
		return err
	}

	// Define the set of fields that are already handled by CoreFields.
	// This includes "attributes" which is metadata sent by Salesforce in every record.
	coreFieldNames := map[string]bool{
		"Id": "true", "Name": "true", "Amount": "true", "CloseDate": "true",
		"CreatedDate": "true", "LastModifiedDate": "true", "CreatedBy": "true",
		"LastModifiedBy": "true", "Payout_Reference__c": "true", "attributes": "true",
	}

	// Populate the AdditionalFields map with any fields not in the core set.
	r.AdditionalFields = make(map[string]interface{})
	for key, value := range allFields {
		if _, isCore := coreFieldNames[key]; !isCore {
			// Handle nested relationship objects (e.g., Account.Name) by flattening them.
			if nestedMap, ok := value.(map[string]interface{}); ok {
				for nestedKey, nestedValue := range nestedMap {
					// Exclude metadata fields from nested objects.
					if nestedKey != "attributes" {
						// Create a flattened key like "AccountName".
						flatKey := key + nestedKey
						r.AdditionalFields[flatKey] = nestedValue
					}
				}
			} else {
				r.AdditionalFields[key] = value
			}
		}
	}
	return nil
}

// BatchRequest is the structure for a Salesforce Composite Batch API request.
type BatchRequest struct {
	AllOrNone bool         `json:"binary"`
	Requests  []Subrequest `json:"batchRequests"`
}

// Subrequest represents a single operation within a batch request.
type Subrequest struct {
	Method string      `json:"method"`
	URL    string      `json:"url"`
	Body   interface{} `json:"richInput"`
}

// BatchResponse is the structure of the response from the Composite Batch API.
type BatchResponse struct {
	HasErrors bool     `json:"hasErrors"`
	Results   []Result `json:"results"`
}

// Result represents the outcome of a single subrequest within a batch response.
type Result struct {
	StatusCode int         `json:"statusCode"`
	Result     interface{} `json:"result"`
}
