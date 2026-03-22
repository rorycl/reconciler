// reconciler-dev is a version of the reconciler app for local development.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	charmlog "github.com/charmbracelet/log"
	"github.com/rorycl/reconciler/app"
)

// appInitialiser converts an app.NewApp to a cli WebRunner interface.
func appInitialiser(
	configFile string,
	logLevel slog.Level,
	inDevelopment bool,
	staticPath, templatePath, sqlPath, databasePath string,
) (WebRunner, error) {

	charmLogger := charmlog.NewWithOptions(os.Stdout, charmlog.Options{
		ReportCaller:    true,
		ReportTimestamp: true,
		TimeFormat:      time.Kitchen,
		Level:           charmlog.Level(logLevel),
	})
	logger := slog.New(charmLogger)
	return app.NewApp(configFile, logger, inDevelopment, staticPath, templatePath, sqlPath, databasePath)
}

// run is the entry point.
func run(args []string) error {

	// BuildCLI builds the command line application, injecting the app constructor for
	// filling with cli arguments.
	pin := newPin()
	cmd := BuildCLI(AppMaker(appInitialiser), pin.check)

	ctx := context.Background()

	// Run runs the production webserver.
	return cmd.Run(ctx, args)
}

func main() {
	err := run(os.Args)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
