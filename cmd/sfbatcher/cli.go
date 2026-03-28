// sfbatcher is a reconciler command-line batching tool.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/rorycl/reconciler/config"
	"github.com/urfave/cli/v3"
)

const (
	ShortUsage      = "batch update salesforce records"
	LongDescription = `A batch update program for linking or unlinking salesforce donation
records, to assist in reconciling salesforce donation records with xero
invoice and bank transaction references.

Please see the project README for more information at
https://github.com/rorycl/reconciler.

Note that this program can affect many salesforce records in one
invocation.`
)

// Runner is an interface to runner.run to allow for testing.
type cliRunner interface {
	run() error
}

// runMaker instantiates a concrete implementation of cliRunner.
type runMaker func(
	filename string,
	action string,
	cfg *config.Config,
	logger *slog.Logger,
	sfMaker sfClientMakerFunc,
	loginAgent oauth2Agent) (cliRunner, error)

// fileCheck checks that a file exists.
func fileCheck(fileName string) error {
	if fileName == "" {
		return errors.New("empty filename")
	}
	if _, err := os.Stat(fileName); err != nil {
		return fmt.Errorf("file %q not found", fileName)
	}
	return nil
}

// BuildCLI creates a cli app to run the capabilities provided by a cliRunner.
func BuildCLI(runMaker runMaker) *cli.Command {

	var (
		excelFile  string
		configFile string
		action     string
		cfg        *config.Config
		logLevel   string
	)

	configFlag := &cli.StringFlag{
		Name:     "configFile",
		Aliases:  []string{"c"},
		Required: true,
		Usage:    "configuration file",
	}

	logLevelFlag := &cli.StringFlag{
		Name:    "logLevel",
		Aliases: []string{"l"},
		Value:   "Info",
		Usage:   "slog logger debug level",
	}

	fileArg := &cli.StringArg{
		Name: "excelFile",
	}

	// Alternative implementation would work like this:
	//	./sfbatcher link ...flags... excelfile.xlsx
	//	./sfbatcher unlink ...flags... excelfile.xlsx
	// i.e. "link" and "unlink" are subcommands, not flags.
	// cmdApp := cli.App{
	// 	Commands: []*cli.Command{},
	// }

	cmd := &cli.Command{
		Name:        "sfbatcher",
		Usage:       ShortUsage,
		Description: LongDescription,

		Commands: []*cli.Command{

			{
				Name:        "link",
				Usage:       "link salesforce records with references",
				UsageText:   "e.g. ./sfbatcher link -c <config> [-l <loglevel>] excelfile",
				Description: "link salesforce records with the references set out in in the provided excel file",
				ArgsUsage:   "<excelfile>",

				// Attach the flags.
				Flags: []cli.Flag{
					configFlag, logLevelFlag,
				},

				// Attach the arguments.
				Arguments: []cli.Argument{
					fileArg,
				},

				// Before runs verification before "Action" is run
				Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {

					action = "link"

					excelFile = c.Args().Get(0)
					if err := fileCheck(excelFile); err != nil {
						return ctx, fmt.Errorf("excel file: %w", err)
					}

					configFile = c.String("configFile")
					if err := fileCheck(excelFile); err != nil {
						return ctx, fmt.Errorf("config file: %w", err)
					}

					// Load config.
					var err error
					cfg, err = config.Load(configFile)
					if err != nil {
						return ctx, fmt.Errorf("config error: %w", err)
					}

					// Validate log level.
					logLevel = c.String("logLevel")
					switch logLevel {
					case "Error", "Warn", "Info", "Debug":
					default:
						return ctx, fmt.Errorf("error: expected a log level of 'Error', 'Warn', 'Info' or 'Debug' got %s", c.String("logLevel"))
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
					}(logLevel)

					// Initialise logger
					handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: debugLevel})
					logger := slog.New(handler)

					app, err := runMaker(
						excelFile,
						action,
						cfg,
						logger,
						nil, // salesforce client maker (use default)
						nil, // login agent (use default)
					)
					if err != nil {
						return err
					}
					return app.run()
				},
			},
			{
				Name:        "unlink",
				Usage:       "unlink salesforce records",
				UsageText:   "e.g. ./sfbatcher unlink -c <config> [-l <loglevel>] excelfile",
				Description: "unlink salesforce records set out in the excel file by setting the configured reference field to ''",
				ArgsUsage:   "<excelfile>",

				// Attach the flags.
				Flags: []cli.Flag{
					configFlag, logLevelFlag,
				},

				// Attach the arguments.
				Arguments: []cli.Argument{
					fileArg,
				},

				// Before runs verification before "Action" is run
				Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {

					action = "unlink"

					excelFile = c.Args().Get(0)
					if err := fileCheck(excelFile); err != nil {
						return ctx, fmt.Errorf("excel file: %w", err)
					}

					configFile = c.String("configFile")
					if err := fileCheck(excelFile); err != nil {
						return ctx, fmt.Errorf("config file: %w", err)
					}

					// Load config.
					var err error
					cfg, err = config.Load(configFile)
					if err != nil {
						return ctx, fmt.Errorf("config error: %w", err)
					}

					// Validate log level.
					logLevel = c.String("logLevel")
					switch logLevel {
					case "Error", "Warn", "Info", "Debug":
					default:
						return ctx, fmt.Errorf("error: expected a log level of 'Error', 'Warn', 'Info' or 'Debug' got %s", c.String("logLevel"))
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
					}(logLevel)

					// Initialise logger
					handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: debugLevel})
					logger := slog.New(handler)

					app, err := runMaker(
						excelFile,
						action,
						cfg,
						logger,
						nil, // salesforce client maker (use default)
						nil, // login agent (use default)
					)
					if err != nil {
						return err
					}
					return app.run()
				},
			},
		},
	}

	// custom help template.
	// cmd.CustomRootCommandHelpTemplate = cmdHelpTemplate

	return cmd
}
