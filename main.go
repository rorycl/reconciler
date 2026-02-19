package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"reconciler/config"
	"reconciler/db"
	"reconciler/internal"
	"reconciler/web"
)

func runServer() {

	if len(os.Args) != 2 {
		fmt.Println("Please provide the configuration yaml file as an argument")
		os.Exit(1)
	}
	yamlFile := os.Args[1]

	// Load the configuration file.
	cfg, err := config.Load(yamlFile)
	if err != nil {
		fmt.Println("configuration error:", err)
		os.Exit(1)
	}

	// Configure Logging.
	logger := slog.Default()
	if cfg.InDevelopmentMode {
		slog.SetLogLoggerLevel(slog.LevelInfo)
	}

	// Create the database connection.
	accountCodes := cfg.DonationAccountCodesRegex()
	thisDB, err := db.NewConnection(cfg.DatabasePath, "", accountCodes, logger)
	if err != nil {
		fmt.Println("database setup error", err)
		os.Exit(1)
	}

	// Mount the web static resources and template filesystems.
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

	// Configure and launch the web server.
	webApp, err := web.New(logger, cfg, thisDB, staticFS, templatesFS)
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
