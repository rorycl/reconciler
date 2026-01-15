package main

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

//go:embed static
var staticFS embed.FS

//go:embed templates
var templatesFS embed.FS

//go:embed sql
var sqlFS embed.FS

type fileMounts struct {
	static    fs.FS
	templates fs.FS
	sqlDir    fs.FS
}

// makeMounts is a horrible function with hard coded values.
func makeMounts(inDevelopment bool) (*fileMounts, error) {

	// dirExists checks if the path is to a valid directory.
	dirExists := func(path string) bool {
		s, err := os.Stat(path)
		if err != nil {
			return false
		}
		if !s.IsDir() {
			return false
		}
		return true
	}

	fm := fileMounts{}
	if inDevelopment {
		if !dirExists("static") {
			return nil, errors.New("no static dir")
		}
		fm.static = os.DirFS("static")

		if !dirExists("templates") {
			return nil, errors.New("no templates dir")
		}
		fm.templates = os.DirFS("templates")

		if !dirExists("sql") {
			return nil, errors.New("no sql dir")
		}
		fm.sqlDir = os.DirFS("sql")
		return &fm, nil
	}
	var err error
	fm.static, err = fs.Sub(staticFS, "static")
	if err != nil {
		return nil, fmt.Errorf("staticfs err: %v", err)
	}
	fm.templates, err = fs.Sub(templatesFS, "templates")
	if err != nil {
		return nil, fmt.Errorf("templatesfs err: %v", err)
	}
	fm.sqlDir, err = fs.Sub(sqlFS, "sql")
	if err != nil {
		return nil, fmt.Errorf("sqlfs err: %v", err)
	}
	return &fm, nil
}
