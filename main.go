package main

import (
	"fmt"
	"log"
	"os"
	"reconciler/config"
	"reconciler/db"
	"reconciler/internal"
	"reconciler/web"
	"time"
)

// This is a temporary function for development.
func runServer() {

	logger := log.Default()

	if len(os.Args) != 2 {
		fmt.Println("Please provide the configuration yaml file as an argument")
		os.Exit(1)
	}
	yamlFile := os.Args[1]

	cfg, err := config.Load(yamlFile)
	if err != nil {
		fmt.Println("configuration error:", err)
		os.Exit(1)
	}

	sqlFS, err := internal.NewFileMount("sql", db.SQLEmbeddedFS, "")
	if err != nil {
		fmt.Println("could not mount sql fs:", err)
		os.Exit(1)
	}

	// accountCodes := "^(53|55|57)"
	accountCodes := cfg.DonationAccountCodesRegex()

	thisDB, err := db.NewConnection(cfg.DatabasePath, "", accountCodes)
	if err != nil {
		fmt.Println("database setup error", err)
		os.Exit(1)
	}
	// Load the schema definitions.
	if err := thisDB.InitSchema(sqlFS, "schema.sql"); err != nil {
		_ = thisDB.Close()
		fmt.Println("database schema error", err)
		os.Exit(1)

	}

	staticFS, err := internal.NewFileMount("static", web.StaticEmbeddedFS, "")
	if err != nil {
		fmt.Println("static file mount error", err)
		os.Exit(1)
	}
	templatesFS, err := internal.NewFileMount("templates", web.TemplatesEmbeddedFS, "")
	if err != nil {
		fmt.Println("templates file mount error", err)
		os.Exit(1)
	}

	// testing data start and end dates
	startDate := time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC)

	webApp, err := web.New(logger, cfg, thisDB, staticFS, templatesFS, startDate, endDate)
	if err != nil {
		log.Fatal(err)
	}
	err = webApp.StartServer()
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	runServer()
}
