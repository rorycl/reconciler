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

// FlattenedName flattens an obj.Name string to a string.
type FlattenedName string

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

// UnmarshalJSON implements the json.Unmarshaler interface for a FlattenedName,
// extracting the "Name" field of the object pointed to by the struct tag into the
// string field.
func (fn *FlattenedName) UnmarshalJSON(data []byte) error {
	// Handle the case of a JSON null value.
	if string(data) == "null" {
		*fn = ""
		return nil
	}
	// Use a helper struct to extract the 'Name' field from the object.
	var helper struct {
		Name string `json:"Name"`
	}
	if err := json.Unmarshal(data, &helper); err != nil {
		return err
	}
	*fn = FlattenedName(helper.Name)
	return nil
}

// SOQLResponse is the top-level envelope for a SOQL query response.
type SOQLResponse struct {
	TotalSize      int        `json:"totalSize"`
	Done           bool       `json:"done"`
	NextRecordsURL string     `json:"nextRecordsUrl"`
	Donations      []Donation `json:"records"`
}

// CoreFields defines the essential, non-negotiable fields the application requires.
type CoreFields struct {
	ID               string         `json:"Id"`
	Name             string         `json:"Name"`
	Amount           float64        `json:"Amount"`
	CloseDate        SalesforceDate `json:"CloseDate"`
	CreatedDate      SalesforceTime `json:"CreatedDate"`
	LastModifiedDate SalesforceTime `json:"LastModifiedDate"`
	CreatedBy        FlattenedName  `json:"CreatedBy"`
	LastModifiedBy   FlattenedName  `json:"LastModifiedBy"`
	PayoutReference  *string        `json:"Payout_Reference__c"` // Pointer to handle null values
}

// Donation represents the data for a single Salesforce donation, combining
// core and additional fields.
type Donation struct {
	CoreFields
	AdditionalFields map[string]any
}

// SOQLUnmarshaller is a configurable struct for managing the custom
// unmarshalling of a SOQL response. The Mapper provides a map of
// fields (other than CoreFields) to store in each Donation's
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
		Donations:      make([]Donation, 0, len(rawResponse.Records)),
	}

	// Process rawResponse records into finalResponse.Donations.
	for _, rawRecord := range rawResponse.Records {
		donation, err := su.unmarshalAndMapRecord(rawRecord)
		if err != nil {
			return nil, fmt.Errorf("failed to process donation: %w", err)
		}
		finalResponse.Donations = append(finalResponse.Donations, donation)
	}

	return finalResponse, nil
}

// unmarshalAndMapRecord marshals raw data into Donation.CoreFields and
// Donation.AdditionalFields.
func (su *SOQLUnmarshaller) unmarshalAndMapRecord(data []byte) (Donation, error) {
	var donation Donation

	if err := json.Unmarshal(data, &donation.CoreFields); err != nil {
		return donation, fmt.Errorf("failed to unmarshal core fields: %v", err)
	}

	var allFields map[string]json.RawMessage
	if err := json.Unmarshal(data, &allFields); err != nil {
		return donation, fmt.Errorf("failed to unmarshal into generic map: %v", err)
	}

	// Delete core fields and unneeded fields from allFields. Retain the top-level
	// "attributes" for reference if needed.
	delete(allFields, "Id")
	delete(allFields, "Name")
	delete(allFields, "Amount")
	delete(allFields, "CloseDate")
	delete(allFields, "LastModifiedDate")
	delete(allFields, "Payout_Reference__c")
	delete(allFields, "CreatedDate")
	delete(allFields, "CreatedBy")
	delete(allFields, "ModifiedBy")
	delete(allFields, "LastModifiedBy")

	donation.AdditionalFields = make(map[string]any)
	for key, rawValue := range allFields {
		var subMap map[string]json.RawMessage
		// Determine if v is a map, if so recurse one level and make a compound key
		// such as "LastModifiedBy.Name".
		if err := json.Unmarshal(rawValue, &subMap); err == nil {
			for subKey, subValue := range subMap {
				if subKey == "attributes" {
					continue
				}
				newKey := strings.Join([]string{enTitle(key), enTitle(subKey)}, ".")
				if replacementKey, ok := su.Mapper[newKey]; ok {
					newKey = replacementKey
				}
				var v any
				_ = json.Unmarshal(subValue, &v)
				donation.AdditionalFields[newKey] = v
			}
		} else {
			newKey := enTitle(key)
			if replacementKey, ok := su.Mapper[newKey]; ok {
				newKey = replacementKey
			}
			var v any
			_ = json.Unmarshal(rawValue, &v)
			donation.AdditionalFields[newKey] = v
		}
	}

	// Check all anticipated mapper values exist in donation.
	for k, v := range su.Mapper {
		if _, ok := donation.AdditionalFields[v]; !ok {
			return donation, &ErrUnmarshallFieldNotFoundError{
				originalField: k,
				newField:      v,
			}
		}
	}

	return donation, nil
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

// SaveResult represents the outcome of a single donation update within
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
