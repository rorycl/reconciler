package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/urfave/cli/v3"
)

const (
	ShortUsage      = "A webapp for reconciling financial and CRMS system donations"
	LongDescription = `
The reconciler app is a local web server for creating OAuth2 API connections to an
organisation's financial and CRMS systems for reconciling these with codes from the
financial system. Please see the project readme for more information.
https://github.com/rorycl/reconciler.`
)

// WebRunner is an interface to the central coordinator for the project (concretely
// provided by App in app.go) to allow for testing.
type WebRunner interface {
	RunWebServer() error
}

// AppMaker instantiates a concrete implementation of WebRunner.
type AppMaker func(configFile string, logLevel slog.Level, inDevelopment bool, staticPath, templatePath, sqlPath, databasePath string) (WebRunner, error)

// BuildCLI creates a cli app to run the capabilities provided by
// a WebRunner dependency.
func BuildCLI(apper AppMaker) *cli.Command {

	var configFile string

	logLevelFlag := &cli.StringFlag{
		Name:    "logLevel",
		Aliases: []string{"l"},
		Value:   "Error",
		Usage:   "slog logger debug level",
	}
	fileArg := &cli.StringArg{
		Name: "configFile",
	}

	cmd := &cli.Command{
		Name:        "reconciler",
		Usage:       ShortUsage,
		Description: LongDescription,
		ArgsUsage:   "<yamlfile>",

		// Attach the flags.
		Flags: []cli.Flag{
			logLevelFlag,
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
				false,      // not inDevelopment
				"",         // staticPath : use embedded
				"",         // templatePath : use embedded
				"",         // sqlPath : use embedded
				":memory:", // force use of memory database in production
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
