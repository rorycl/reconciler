// reconciler-dev is the development mode cli for the cli project.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/urfave/cli/v3"
)

const (
	ShortUsage      = "dev mode reconciler app"
	LongDescription = `
This reconciler app is a *development mode only* local web server for web development.

Please see the project README for more information at
https://github.com/rorycl/reconciler.

The development mode server may use a local database file on disk but it is the
developer's responsibility to keep this safe and remove it.

This server does not presently automatically restart to recompile templates.

A pin is required to start the server to stop this being run in production.
`
)

// WebRunner is an interface to the central coordinator for the project (concretely
// provided by App in app.go) to allow for testing.
type WebRunner interface {
	RunWebServer() error
}

// AppMaker instantiates a concrete implementation of WebRunner.
type AppMaker func(configFile string, logLevel slog.Level, inDevelopment bool, staticPath, templatePath, sqlPath, databasePath string) (WebRunner, error)

// BuildCLI creates a cli app to run the capabilities provided by
// a WebRunner dependency. A verifier func can be provided to guard app running.
func BuildCLI(apper AppMaker, verifier func() error) *cli.Command {
	// func BuildCLI(apper AppMaker, verifier func() error) *cli.Command {

	var configFile string

	logLevelFlag := &cli.StringFlag{
		Name:    "logLevel",
		Aliases: []string{"l"},
		Value:   "Error",
		Usage:   "slog logger debug level",
	}
	staticFlag := &cli.StringFlag{
		Name:     "staticPath",
		Aliases:  []string{"s"},
		Required: true,
		Usage:    "path to the web static directory",
	}
	tplFlag := &cli.StringFlag{
		Name:     "templatePath",
		Aliases:  []string{"t"},
		Required: true,
		Usage:    "path to the web templates directory",
	}
	sqlFlag := &cli.StringFlag{
		Name:     "sqlPath",
		Aliases:  []string{"q"},
		Required: true,
		Usage:    "path to the sql templates directory",
	}
	dbFlag := &cli.StringFlag{
		Name:     "database",
		Aliases:  []string{"d"},
		Required: true,
		Usage:    "':memory:' or path to database file (overrides config)",
	}

	fileArg := &cli.StringArg{
		Name: "configFile",
	}

	cmd := &cli.Command{
		Name:        "reconciler-dev",
		Usage:       ShortUsage,
		Description: LongDescription,
		ArgsUsage:   "<yamlfile>",

		// Attach the flags.
		Flags: []cli.Flag{
			logLevelFlag, staticFlag, tplFlag, sqlFlag, dbFlag,
		},

		// Attach the arguments.
		Arguments: []cli.Argument{
			fileArg,
		},

		// Before runs verification before "Action" is run
		Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {

			configFile = c.Args().Get(0)
			if configFile == "" {
				return ctx, fmt.Errorf("error: config file not provided")
			}
			// Validate config file exists.
			if _, err := os.Stat(configFile); err != nil {
				return ctx, fmt.Errorf("error: could not stat config file %q: %w", configFile, err)
			}

			// Validate log level.
			switch c.String("logLevel") {
			case "Error", "Warn", "Info", "Debug":
			default:
				return ctx, fmt.Errorf("error: expected a debug level of 'Error', 'Warn', 'Info' or 'Debug' got %s", c.String("logLevel"))

			}

			return ctx, nil
		},
		Action: func(ctx context.Context, c *cli.Command) error {

			err := verifier()
			if err != nil {
				return fmt.Errorf("verifier error: %w", err)
			}

			debugLevel := func(s string) slog.Level {
				switch s {
				case "Warn":
					return slog.LevelWarn
				case "Info":
					return slog.LevelInfo
				case "Debug":
					return slog.LevelDebug
				default:
					return slog.LevelError
				}
			}(c.String("logLevel"))

			app, err := apper(
				configFile,
				debugLevel,
				true,                     // inDevelopment
				c.String("staticPath"),   // staticPath
				c.String("templatePath"), // templatePath
				c.String("sqlPath"),      // sqlPath
				c.String("database"),     // overrides config
			)
			if err != nil {
				return err
			}
			return app.RunWebServer()
		},
	}

	// custom help template.
	// cmd.CustomRootCommandHelpTemplate = cmdHelpTemplate

	return cmd
}
