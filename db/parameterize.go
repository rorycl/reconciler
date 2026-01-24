package db

import (
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
)

// ParameterizedSQLTemplate is a struct holding a parsed template with
// parameters extracted and arguments replaced by the '?' symbol.
type ParameterizedSQLTemplate struct {
	Body       []byte
	Parameters []string
}

// String provides a printable representation.
func (p ParameterizedSQLTemplate) String() string {
	tpl := `
Params: %s
Body:   %s
`
	return fmt.Sprintf(tpl, strings.Join(p.Parameters, ", "), string(p.Body))
}

// regexpParam matches lines such as
//
//	,date('2026-03-31') AS DateTo    /* @param */
//
// for extracting the `DateTo` parameter and replacing the provided
// parameter with a '?', for example:
//
//	,date(?) AS DateTo    /* @param */
//
// Note that the spacing around the parameter needs to be precise.
var (
	paramAtoms = []string{
		`(?:date\('[^']+'\))`,        // date('2026-03-31')
		`(?:[a-zA-Z_]\w*\([^\)]*\))`, // any_func(...)
		`(?:'[^']*')`,                // 'a string' or ''
		`(?:-?\d*\.?\d+)`,            // 123 or 1.23 or -5
		`(?:null)`,                   // null
	}

	// regexParam is made of 4 components where are named for
	// identification. The 'value' element is built up out of the
	// non-capturing paramAtoms items.
	regexpParam = regexp.MustCompile(fmt.Sprintf(
		`(?P<value>%s)(?P<as>\s+AS\s+)(?P<param>[A-Za-z0-9_]+)(?P<end>\s+/\* @param \*/)`,
		strings.Join(paramAtoms, "|"),
	))
)

// parameterize takes an sql template as a slice of bytes with
// (potentially) inline field definitions in order to provide the
// functionality of functional procedural sql with declared variables in
// sqlite.
//
// The inline field definitions are defined with an `/* @param */`
// marker such as:
//
//	,date('2026-03-31') AS DateTo    /* @param */
//
// which are then replaced with SQL prepared statement '?' symbols and
// the field name extracted as a parameter, returning
//
//	*ParameterizedSQLTemplate{
//	    Parameters: []string{"DateTo"},
//	    Body      : string([]byte('    ,$DateTo AS DateTo),
//	}
//
// Multiple definitions in a template are handled, as shown in the test.
func parameterize(tpl []byte) (*ParameterizedSQLTemplate, error) {

	matches := regexpParam.FindAllSubmatch(tpl, -1)
	if len(matches) == 0 {
		return nil, errors.New("parameterize: no parameters found")
	}

	pst := &ParameterizedSQLTemplate{
		Parameters: make([]string, len(matches)),
	}

	paramIdx := regexpParam.SubexpIndex("param")
	for i := range matches {
		pst.Parameters[i] = string(matches[i][paramIdx])
	}

	// Use $ quoted parameter names such as `$DateFrom`.
	pst.Body = regexpParam.ReplaceAll(tpl, []byte(`:${param}${as}${param}`))
	return pst, nil
}

// ParameterizeFile takes an sql file and returns a
// ParameterizedSQLTemplate or error.
func ParameterizeFile(fileFS fs.FS, filePath string) (*ParameterizedSQLTemplate, error) {

	fileBytes, err := fs.ReadFile(fileFS, filePath)
	if err != nil {
		return nil, fmt.Errorf("file read error: %w", err)
	}
	query, err := parameterize(fileBytes)
	if err != nil {
		return nil, fmt.Errorf("query template error: %w", err)
	}
	return query, nil

}
