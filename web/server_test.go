package web

import (
	"log"
	"reconciler/config"
	"reconciler/db"
	"reconciler/internal"
	"testing"
	"time"
)

func TestWebApp(t *testing.T) {

	logger := log.Default()

	cfg := &config.Config{
		Web: config.WebConfig{
			ListenAddress: "127.0.0.1:8000",
		},
	}
	accountCodes := "^(53|55|57)"
	db, err := db.NewConnectionInTestMode("file::memory:?cache=shared", "", accountCodes)
	if err != nil {
		t.Fatal(err)
	}

	staticFS, err := internal.NewFileMount("static", staticEmbeddedFS, "")
	if err != nil {
		t.Fatal(err)
	}
	templatesFS, err := internal.NewFileMount("templates", templatesEmbeddedFS, "")
	if err != nil {
		t.Fatal(err)
	}

	// testing data start and end dates
	startDate := time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC)

	webApp, err := New(logger, cfg, db, staticFS, templatesFS, startDate, endDate)
	if err != nil {
		t.Fatal(err)
	}
	err = webApp.StartServer()
	if err != nil {
		t.Fatal(err)
	}

}
