package web

import (
	"log/slog"
	"reconciler/config"
	"reconciler/db"
	"reconciler/internal"
	"testing"
)

func TestWebApp(t *testing.T) {

	logger := slog.Default()
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

	webApp, err := New(logger, cfg, db, staticFS, templatesFS)
	if err != nil {
		t.Fatal(err)
	}
	err = webApp.StartServer()
	if err != nil {
		t.Fatal(err)
	}

}
