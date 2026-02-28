// package app is the main entry point to the program, providing different ways of
// accessing the RunWebServer function in production and development mode.
package app

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"reconciler/config"
	"reconciler/db"
	mounts "reconciler/internal/mounts"
	"reconciler/web"
)

// App encapsulates the functions of reconciler for use by command programs.
type App struct {
	cfg           *config.Config
	log           *slog.Logger
	logLevel      slog.Level
	inDevelopment bool
	db            *db.DB
	staticFS      fs.FS
	templateFS    fs.FS
	sqlFS         fs.FS
}

// NewApp initialises a new App.
func NewApp(
	configFile string,
	logLevel slog.Level,
	inDevelopment bool,
	staticPath string,
	templatePath string,
	sqlPath string,
	databasePath string,
) (*App, error) {

	if configFile == "" {
		return nil, errors.New("no config file provided")
	}

	if inDevelopment {
		if staticPath == "" || templatePath == "" || sqlPath == "" {
			return nil, errors.New("in development mode, all paths must be provided")
		}
		if databasePath == "" {
			return nil, errors.New("in development mode, the database path must be provided")
		}
	}
	if !inDevelopment {
		if databasePath != ":memory:" {
			return nil, errors.New("in production mode, the database path must be ':memory:'")
		}
	}

	// Load the configuration file.
	cfg, err := config.Load(configFile)
	if err != nil {
		return nil, fmt.Errorf("could not load configuration file: %w", err)
	}
	accountCodes := cfg.DonationAccountCodesRegex()

	// Initialise the logger.
	logger := slog.New(slog.NewTextHandler(
		os.Stdout,
		&slog.HandlerOptions{Level: logLevel},
	))

	// Mount the filesystems.
	staticFS, err := mounts.NewFileMount("static", web.StaticEmbeddedFS, staticPath)
	if err != nil {
		return nil, fmt.Errorf("could not mount static file system: %w", err)
	}
	templateFS, err := mounts.NewFileMount("templates", web.TemplatesEmbeddedFS, templatePath)
	if err != nil {
		return nil, fmt.Errorf("could not mount template file system: %w", err)
	}
	sqlFS, err := mounts.NewFileMount("sql", db.SQLEmbeddedFS, sqlPath)
	if err != nil {
		return nil, fmt.Errorf("could not mount template file system: %w", err)
	}

	// Initialise the database connection.
	dbCon, err := db.NewConnection(databasePath, sqlFS, accountCodes, logger)
	if err != nil {
		return nil, fmt.Errorf("could not initialise database: %w", err)
	}

	return &App{
		cfg:           cfg,
		log:           logger,
		logLevel:      logLevel,
		inDevelopment: inDevelopment,
		db:            dbCon,
		staticFS:      staticFS,
		templateFS:    templateFS,
		sqlFS:         sqlFS,
	}, nil

}

// RunWebServer configures and launches the web server.
func (a *App) RunWebServer() error {

	// Configure and launch the web server.
	webApp, err := web.New(a.log, a.cfg, a.db, a.staticFS, a.templateFS)
	if err != nil {
		a.log.Error(fmt.Sprintf("app web server init error: %v", err))
		return fmt.Errorf("could not initialise web server: %w", err)
	}
	return webApp.StartServer()
}
