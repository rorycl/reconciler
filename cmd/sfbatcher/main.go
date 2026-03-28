package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/rorycl/reconciler/config"
)

// runConverter converts a newRunner to a cliRunner
func runConverter(
	filename string,
	action string,
	cfg *config.Config,
	logger *slog.Logger,
	sfMaker sfClientMakerFunc,
	loginAgent oauth2Agent) (cliRunner, error) {
	return newRunner(filename, action, cfg, logger, sfMaker, loginAgent)
}

func main() {

	ctx := context.Background()
	cli := BuildCLI(runConverter)
	if err := cli.Run(ctx, os.Args); err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
}
