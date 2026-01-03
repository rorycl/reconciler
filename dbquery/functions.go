package dbquery

// This functions.go registers a regexpFunc function as set out in the
// package docs for modernc.org/sqlite.RegisterFunction and
// modernc.org/sqlite.FunctionImpl.
//
// Note that a cached regexp might be worth implementing.

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"regexp"
	"sync"

	"modernc.org/sqlite"
)

var registerOnce sync.Once

// regexpFunc is a function to be registered with sqlite.FunctionImpl.
func regexpFunc(pattern, s string) (bool, error) {
	return regexp.MatchString(pattern, s)
}

// RegisterFunctions registers the custom Go function with the sqlite
// driver. Refer to the sqlite `func_test.go` test for further examples.
func RegisterFunctions() {
	registerOnce.Do(func() {
		sqlite.MustRegisterDeterministicScalarFunction(
			// Register the function "REGEXP" globally for all connections.
			"REGEXP",
			2,
			func(ctx *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
				var s1 string
				var s2 string

				switch arg0 := args[0].(type) {
				case string:
					s1 = arg0
				default:
					return nil, errors.New("expected argv[0] to be text")
				}

				switch arg1 := args[1].(type) {
				case string:
					s2 = arg1
				default:
					return nil, errors.New("expected argv[1] to be text")
				}

				matched, err := regexp.MatchString(s1, s2)
				if err != nil {
					return nil, fmt.Errorf("bad regular expression: %q", err)
				}

				return matched, nil
			},
		)
	})
}
