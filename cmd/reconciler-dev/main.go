// reconciler-dev is a version of the reconciler app for local development.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/rorycl/reconciler/app"
)

// appInitialiser converts an app.NewApp to a cli WebRunner interface.
func appInitialiser(
	configFile string,
	logLevel slog.Level,
	inDevelopment bool,
	staticPath, templatePath, sqlPath, databasePath string,
) (WebRunner, error) {
	return app.NewApp(configFile, logLevel, inDevelopment, staticPath, templatePath, sqlPath, databasePath)
}

func run(args []string) error {

	// BuildCLI builds the command line application, injecting the app constructor for
	// filling with cli arguments.
	cmd := BuildCLI(AppMaker(appInitialiser), getPin)

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
