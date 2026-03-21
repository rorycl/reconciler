// package app is the main entry point to the program, providing different ways of
// accessing the RunWebServer function in production and development mode.
package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/rorycl/reconciler/config"
	"github.com/rorycl/reconciler/db"
	"github.com/rorycl/reconciler/domain"
	"github.com/rorycl/reconciler/internal/filewatcher"
	mounts "github.com/rorycl/reconciler/internal/mounts"
	"github.com/rorycl/reconciler/web"
)

// App encapsulates a reconciler and associated infrastructure elements for use by
// command programs.
type App struct {
	cfg        *config.Config
	log        *slog.Logger
	logLevel   slog.Level
	reconciler *domain.Reconciler
	staticFS   fs.FS
	templateFS fs.FS
	sqlFS      fs.FS

	// development fields
	inDevelopment bool
	watcher       <-chan error
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

	// Construct the reconciler
	reconciler := domain.NewReconciler(dbCon, logger)

	app := &App{
		cfg:           cfg,
		log:           logger,
		logLevel:      logLevel,
		inDevelopment: inDevelopment,
		reconciler:    reconciler,
		staticFS:      staticFS,
		templateFS:    templateFS,
		sqlFS:         sqlFS,
	}

	// Register filewatchers if in development, for automatic reloading.
	if inDevelopment {
		watcher, err := filewatcher.NewFileChangeNotifier(
			context.Background(),
			map[string][]string{
				filepath.Join(staticPath, "css"): {".css"},
				filepath.Join(staticPath, "js"):  {".js"},
				templatePath:                     {".html"},
				sqlPath:                          {".sql"},
			},
		)
		if err != nil {
			return nil, fmt.Errorf("file watcher error: %v", err)
		}
		app.watcher = watcher.Update()
	}

	return app, nil

}

// RunWebServer configures and launches the web server.
func (a *App) RunWebServer() error {

	// Configure and launch the web server. This uses the default xero and salesforce
	// Clients.
	webApp, err := web.New(a.cfg, a.reconciler, a.log, a.staticFS, a.templateFS, nil, nil)
	if err != nil {
		a.log.Error(fmt.Sprintf("app web server init error: %v", err))
		return fmt.Errorf("could not initialise web server: %w", err)
	}

	// If inDevelopment mode, set the webapp in development, and run the file watcher in
	// a goroutine to range over events to either deal with errors or trigger route
	// restarting (which recompiles the templates) if file changes are detected.
	if a.inDevelopment {

		webApp.SetInDevelopment()

		go func() {
			for err := range a.watcher {
				if err != nil {
					a.log.Error(fmt.Sprintf("file watcher error: %v", err))
					a.log.Error("file watching has stopped")
					return
				}
				a.log.Info("file watcher detected a file change; restarting routes")

				// Restart the web server routes.
				webApp.RestartRoutes()
			}
		}()
	}

	// Start the server.
	return webApp.StartServer()

}
