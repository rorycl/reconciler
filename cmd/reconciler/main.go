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

func run(args []string) {

	// BuildCLI builds the command line application, injecting the app constructor for
	// filling with cli arguments.
	cmd := BuildCLI(AppMaker(appInitialiser))

	ctx := context.Background()

	// Run runs the production webserver.
	if err := cmd.Run(ctx, args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func main() {
	run(os.Args)
}
