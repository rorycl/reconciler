package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

func main() {

	tpl := []byte(`
    SELECT
        date('2025-04-01') AS DateFrom             /* @DateFrom */
        ,date('2026-03-31') AS DateTo              /* @DateTo */
        -- All | Linked | NotLinked
        ,'Linked' AS LinkageStatus                 /* @LinkageStatus */
        ,'JG-PAYOUT-2025-04-15' AS PayoutReference /* @PayoutReference */
	`)

	p, err := parameterize(tpl)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println(p)
}

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
//	,date('2026-03-31') AS DateTo    /* @DateTo */
//
// for extracting the `DateTo` parameter and replacing the provided
// parameter with a '?', for example:
//
//	,date(?) AS DateTo    /* @DateTo */
//
// Note that the spacing around the parameter needs to be precise.
var regexpParam = regexp.MustCompile(`'([^']+)('.*/\* @)(?P<param>\w+) \*/`)

func parameterize(tpl []byte) (*ParameterizedSQLTemplate, error) {

	matches := regexpParam.FindAllSubmatch(tpl, -1)
	names := regexpParam.SubexpNames()
	if len(names) == 0 {
		return nil, errors.New("parameterize: no parameters found")
	}

	pst := &ParameterizedSQLTemplate{
		Parameters: make([]string, len(names)),
	}

	// There are four matches per parameter line -- the first is the
	// full match -- the named parameter is the last (third)
	// match for each line.
	paramOffset := 3
	for i := range names {
		subMatches := matches[i]
		pst.Parameters[i] = string(subMatches[paramOffset])
	}

	pst.Body = regexpParam.ReplaceAll(tpl, []byte(`'?${2}${3} */`))
	return pst, nil
}
