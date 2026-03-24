package main

import (
	"errors"
	"fmt"
	"iter"
	"strings"

	"github.com/rorycl/reconciler/apiclients/salesforce"
)

// Parser is an interface met by an ExcelParser.
type Parser interface {
	Headers() []string
	Rows() iter.Seq[[]string]
}

// Data represents the validated contents of a spreadsheet describing a set of unique
// salesforce IDs whose reference field (as described by the associated config file)
// should be updated with the associated reference. Note that for linking actions the
// reference ("Ref" part of "IDRef") will be empty.
type Data struct {
	idRefs   []salesforce.IDRef
	uniqueID map[string]struct{}
	Records  int
}

// NewData initialises a new Data and fills it with content from the provided parser.
func NewData(parser Parser) (*Data, error) {
	data := &Data{
		idRefs:   []salesforce.IDRef{},
		uniqueID: map[string]struct{}{},
	}
	header := parser.Headers()
	if err := data.validHeaders(header); err != nil {
		return data, err
	}
	data.Records = 0
	for row := range parser.Rows() {
		data.Records++
		idRef, err := data.validateRow(row)
		if err != nil {
			return data, fmt.Errorf("row %d (1 indexed) error: %w", data.Records, err)
		}
		data.idRefs = append(data.idRefs, idRef)
	}
	return data, nil
}

// validHeaders ensures that the Excel file headers match the
//
//	`ID | Ref | ...`
//
// pattern.
func (d *Data) validHeaders(headers []string) error {
	if len(headers) < 2 {
		return errors.New("at least two headers -- ID and Ref -- expected")
	}
	if strings.TrimSpace(strings.ToLower(headers[0])) != "id" {
		return fmt.Errorf("header %q found -- expected ID", headers[0])
	}
	if strings.TrimSpace(strings.ToLower(headers[1])) != "ref" {
		return fmt.Errorf("header %q found -- expected ref", headers[1])
	}
	return nil
}

// validateRow validates the contents of an Excel Row, ensuring the contents are valid
// and unique Salesforce IDs.
func (d *Data) validateRow(row []string) (salesforce.IDRef, error) {

	idRef := salesforce.IDRef{}

	if len(row) < 2 {
		return idRef, errors.New("row did not have at least 2 columns")
	}
	idRef = salesforce.IDRef{row[0], row[1]}
	if err := salesforce.IDsValid(idRef.ID); err != nil {
		return idRef, err
	}
	if _, exists := d.uniqueID[idRef.ID]; exists {
		return idRef, fmt.Errorf("duplicate ID %q found", idRef.ID)
	}
	d.uniqueID[idRef.ID] = struct{}{}
	return idRef, nil
}

// Batch returns the data.idRefs in slices no larger than batchsize.
func (d *Data) Batch(batchsize int) iter.Seq[[]salesforce.IDRef] {
	refLen := len(d.idRefs)
	return func(yield func([]salesforce.IDRef) bool) {
		if d.Records == 0 {
			return
		}
		start, end := 0, batchsize
		for {
			if end > refLen {
				end = refLen
			}
			if !yield(d.idRefs[start:end]) {
				return
			}
			start += batchsize
			end += batchsize
			if start >= refLen {
				return
			}
		}
	}
}
