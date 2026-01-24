package salesforce

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// SalesforceDate is a custom date type.
type SalesforceDate struct {
	time.Time
}

// SalesforceTime is a custom type to handle Salesforce's specific datetime format.
type SalesforceTime struct {
	time.Time
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (sd *SalesforceDate) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "null" || s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return err
	}
	sd.Time = t
	return nil
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
	CloseDate        SalesforceDate `json:"CloseDate"`
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
	AdditionalFields map[string]any
}

// SOQLUnmarshaller is a configurable struct for managing the custom
// unmarshalling of a SOQL response. The Mapper provides a map of
// fields (other than CoreFields) to store in each Record's
// AdditionalFields.
type SOQLUnmarshaller struct {
	Mapper map[string]string
}

// ErrUnmarshallFieldNotFoundError reports an error from trying to
// unmarshall a field that couldn't be found.
type ErrUnmarshallFieldNotFoundError struct {
	originalField string
	newField      string
}

func (e *ErrUnmarshallFieldNotFoundError) Error() string {
	return fmt.Sprintf("field %s mapped to %s not found", e.originalField, e.newField)
}

func (su *SOQLUnmarshaller) UnmarshalSOQLResponse(data []byte) (*SOQLResponse, error) {
	// rawResponse is an SQLResponse but with json.RawMesage Records,
	// which are processed separately.
	var rawResponse struct {
		TotalSize      int               `json:"totalSize"`
		Done           bool              `json:"done"`
		NextRecordsURL string            `json:"nextRecordsUrl"`
		Records        []json.RawMessage `json:"records"`
	}
	if err := json.Unmarshal(data, &rawResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal SOQL with raw records: %w", err)
	}

	// Prepare the final response.
	finalResponse := &SOQLResponse{
		TotalSize:      rawResponse.TotalSize,
		Done:           rawResponse.Done,
		NextRecordsURL: rawResponse.NextRecordsURL,
		Records:        make([]Record, 0, len(rawResponse.Records)),
	}

	// Process rawResponse records into finalResponse.Records.
	for _, rawRecord := range rawResponse.Records {
		record, err := su.unmarshalAndMapRecord(rawRecord)
		if err != nil {
			return nil, fmt.Errorf("failed to process record: %w", err)
		}
		finalResponse.Records = append(finalResponse.Records, record)
	}

	return finalResponse, nil
}

// unmarshalAndMapRecord marshals raw data into Record.CoreFields and
// Record.AdditionalFields.
func (su *SOQLUnmarshaller) unmarshalAndMapRecord(data []byte) (Record, error) {
	var record Record
	var allFields map[string]any

	if err := json.Unmarshal(data, &allFields); err != nil {
		return record, err
	}

	// Unmarshal the corefields.
	if err := json.Unmarshal(data, &record.CoreFields); err != nil {
		return record, err
	}

	// Unmarshal the selected additional fields. The provided map uses
	// Key.Subkey format for specifying second-level fields such as
	// Account.Name, otherwise the fields are expected to be at the top
	// level.
	//
	// All data from the SOQL query is reported as follows:
	// * CoreFields are recorded in only those fields
	// * Mapper field names are renamed in AdditionalFields
	// * Any other fields are stored in AdditionalFields
	record.AdditionalFields = make(map[string]any)
	for k, v := range allFields {
		switch k {
		// Skip CoreFields by field name, other than for CreatedBy and LastModifiedBy
		case "ID", "Name", "Amount", "CloseDate", "CreatedDate", "LastModifiedDate",
			"PayoutReference":
		// Skip CoreFields by struct tag and not specified above.
		case "Id":
		// If in mapper, save under new field name in AdditionalFields
		// else save in AdditionalFields.
		default:
			// Determine if v is a map, if so recurse one level and make a compound key
			// such as "LastModifiedBy.Name".
			if subMap, ok := v.(map[string]any); ok {
				for kk, sm := range subMap {
					newKey := strings.Join([]string{enTitle(k), enTitle(kk)}, ".")
					if replacementKey, ok := su.Mapper[newKey]; ok {
						newKey = replacementKey
					}
					record.AdditionalFields[newKey] = sm
				}
			} else {
				newKey := enTitle(k)
				if replacementKey, ok := su.Mapper[newKey]; ok {
					newKey = replacementKey
				}
				record.AdditionalFields[newKey] = v
			}
		}
	}

	// Check all anticipated mapper values exist in record.
	for k, v := range su.Mapper {
		if _, ok := record.AdditionalFields[v]; !ok {
			return record, &ErrUnmarshallFieldNotFoundError{
				originalField: k,
				newField:      v,
			}
		}
	}

	return record, nil
}

// CollectionsUpdateRequest is the structure for the sObject Collections
// API request body.
type CollectionsUpdateRequest struct {
	AllOrNone bool             `json:"allOrNone"`
	Records   []map[string]any `json:"records"`
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

var regexpIsTitle *regexp.Regexp = regexp.MustCompile("^[A-Z]")

// enTitle turns a string into title case (e.g. "Title") if the first letter is not
// already a capital leter. This means fields such as ErrorCode won't be turned into
// `Errorcode`.
func enTitle(s string) string {
	switch {
	case regexpIsTitle.MatchString(s):
		return s
	case len(s) < 2:
		return strings.ToTitle(s)
	default:
		return string(strings.ToTitle(s)[0]) + s[1:]
	}
}
