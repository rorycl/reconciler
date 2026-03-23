package main

import (
	"errors"
	"fmt"
	"iter"

	"github.com/xuri/excelize/v2"
)

// ExcelParser opens an excel file and reads the contents of the first sheet.
type ExcelParser struct {
	fileName   string
	sheetNames []string
	contents   [][]string
	row        int
}

// NewExcelParser initialises an ExcelParser by reading an excel file, its sheet names
// and the contents of the first sheet.
func NewExcelParser(fileName string) (*ExcelParser, error) {
	if fileName == "" {
		return nil, errors.New("newparser received an empty file name argument")
	}

	p := &ExcelParser{fileName: fileName}

	f, err := excelize.OpenFile(fileName)
	if err != nil {
		return p, fmt.Errorf("open %q error: %w", fileName, err)
	}

	// Note that excelize uses 1-indexed sheets.
	sheetMap := f.GetSheetMap()
	p.sheetNames = make([]string, len(sheetMap))
	for k, v := range sheetMap {
		p.sheetNames[k-1] = v
	}
	if len(p.sheetNames) != 1 {
		return p, fmt.Errorf("excel files with only one sheet supported, got %d", len(p.sheetNames))
	}

	sheetName := p.sheetNames[0]
	p.contents, err = f.GetRows(sheetName)
	if err != nil {
		return p, fmt.Errorf("could not get rows for %q, sheet %q: %w", fileName, sheetName, err)
	}
	if len(p.contents) == 0 {
		return p, errors.New("no contents found")
	}

	return p, nil
}

// Headers return the first row of the parser contents.
func (p *ExcelParser) Headers() []string {
	p.row++
	return p.contents[0]
}

// Rows returns an iterator over the parser contents.
func (p *ExcelParser) Rows() iter.Seq[[]string] {
	return func(yield func([]string) bool) {
		for _, r := range p.contents[p.row:] {
			p.row++
			if !yield(r) {
				return
			}
		}
	}
}
