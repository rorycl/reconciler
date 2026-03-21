package salesforce

import (
	"errors"
	"fmt"
	"regexp"
)

// Salesforce IDs
// https://developer.salesforce.com/docs/atlas.en-us.object_reference.meta/object_reference/field_types.htm#i1435616
// 15-Character and 18-Character IDs, and Case Sensitivity
//
// Salesforce IDs are often represented by 15-character, base-62, strings. Each of the
// 15 characters can be a numeric digit (0-9), a lowercase letter (a-z), or an uppercase
// letter (A-Z). These 15-character IDs are case-sensitive. To Salesforce,
// 000000000000Abc isn’t the same as 000000000000aBC
// ...
// To avoid these issues, all API calls return an 18-character ID that’s case-safe,
// meaning that it’s compared correctly by case-insensitive applications. The extra 3
// characters at the end of the ID encode the case of the preceding 15 characters. Use
// 18-character IDs in all API calls when creating, editing, or deleting data.

type salesforceID string

var regexpValidsalesforceID = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// NewSalesforceID converts a string into a SaleforceID, returning nil if the conversion
// was not possible. Checksum validation for 18 character IDs is not presently
// implemented.
func newSalesforceID(s string) *salesforceID {
	if len(s) != 15 && len(s) != 18 {
		return nil
	}
	if !regexpValidsalesforceID.MatchString(s) {
		return nil
	}
	sf := salesforceID(s)
	return &sf
}

// IDsValid returns an error if any of the provided strings are not valid Salesforce
// IDs. It is an error to provide no ids.
func IDsValid(ids ...string) error {
	if len(ids) == 0 {
		return errors.New("no ids provided to check Salesforce id validity")
	}
	for _, id := range ids {
		if newSalesforceID(id) == nil {
			return fmt.Errorf("%q is an invalid Salesforce ID", id)
		}
	}
	return nil
}
