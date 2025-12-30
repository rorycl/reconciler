package salesforce

import (
	"encoding/json"
	"fmt"
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

func (s *SOQLResponse) MapAdditionalFields(mapper map[string]string) error {
	for _, rr := range s.Records {
		if err := rr.mapAdditionalFields(mapper); err != nil {
			return fmt.Errorf("mapping error: %v", err)
		}
	}
	return nil
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

// Record represents the data for a single Salesforce record, combining
// core and additional fields.
type Record struct {
	CoreFields
	AdditionalFields map[string]interface{}
	otherFields      map[string]interface{}
}

// UnmarshalJSON provides custom JSON decoding for the Record type,
// populating CoreFields and
// It populates the static CoreFields and captures all other fields into
// the dynamic AdditionalFields map.
func (r *Record) UnmarshalJSON(data []byte) error {

	if err := json.Unmarshal(data, &r.CoreFields); err != nil {
		return err
	}
	var allFields map[string]interface{}
	if err := json.Unmarshal(data, &allFields); err != nil {
		return err
	}

	r.otherFields = make(map[string]interface{})
	for key, value := range allFields {
		// Flatten nested names.
		if nestedMap, ok := value.(map[string]interface{}); ok {
			for nestedKey, nestedValue := range nestedMap {
				// Exclude metadata fields.
				if nestedKey != "attributes" {
					// Create a flattened key like "Account.Name".
					flatKey := key + "." + nestedKey
					r.otherFields[flatKey] = nestedValue
				}
			}
		} else {
			r.otherFields[key] = value
		}
	}
	return nil
}

// mapAdditionalFields maps additional fields of interest with a more
// usable name.
func (r *Record) mapAdditionalFields(mapper map[string]string) error {
	r.AdditionalFields = make(map[string]interface{})
	for originalFieldName, newFieldName := range mapper {
		val, ok := r.otherFields[originalFieldName]
		if !ok {
			return fmt.Errorf("field %q not found", originalFieldName)
		}
		r.AdditionalFields[newFieldName] = val
	}
	r.otherFields = make(map[string]interface{}) // clean.
	return nil
}

// CollectionsUpdateRequest is the structure for the sObject Collections
// API request body.
type CollectionsUpdateRequest struct {
	AllOrNone bool                     `json:"allOrNone"`
	Records   []map[string]interface{} `json:"records"`
}

// CollectionsUpdateResponse is the response from the sObject
// Collections API, which is a slice of SaveResult objects.
type CollectionsUpdateResponse []SaveResult

// SaveResult represents the outcome of a single record update within
// the batch.
type SaveResult struct {
	ID      string        `json:"id"`
	Success bool          `json:"success"`
	Errors  []ErrorDetail `json:"errors"`
}

// ErrorDetail provides specific information about a failure.
type ErrorDetail struct {
	StatusCode string   `json:"statusCode"`
	Message    string   `json:"message"`
	Fields     []string `json:"fields"`
	ErrorCode  string   `json:"errorCode"`
}
