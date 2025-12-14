package main

import (
	"context"
	"fmt"
	"os"

	"xerocli/app"
)

// main is the entry point for the application.
// It initializes the core application logic, builds the CLI interface,
// and executes the command provided by the user.
func main() {
	// Create the core application object which contains the business logic.
	application := app.New()

	// Build the CLI command structure, injecting the application logic.
	cmd := BuildCLI(application)

	// Run the CLI, passing command-line arguments.
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
