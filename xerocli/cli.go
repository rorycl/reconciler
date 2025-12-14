package main

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli/v3"
)

// Applicator defines the interface for the core application logic.
// This allows the CLI to be tested independently of the main app implementation.
type Applicator interface {
	Login(ctx context.Context, cfgPath string) error
	Wipe(ctx context.Context, cfgPath string) error
	SyncBankTransactions(ctx context.Context, cfgPath string, fromDate, ifModifiedSince time.Time) error
	SyncInvoices(ctx context.Context, cfgPath string, fromDate, ifModifiedSince time.Time) error
	UpdateBankTransactionReference(ctx context.Context, cfgPath, uuid, reference string) error
}

// BuildCLI creates the full CLI command structure for the application.
// It injects the core application logic (the Applicator) into the command actions.
func BuildCLI(app Applicator) *cli.Command {
	// Define flags that are common across multiple commands.
	configFlag := &cli.StringFlag{
		Name:    "config",
		Aliases: []string{"c"},
		Value:   "config.yaml",
		Usage:   "path to the configuration file",
	}

	agoFlag := &cli.StringFlag{
		Name:    "ago",
		Usage:   "only refresh records updated within this duration (e.g., '2h', '15m')",
		Aliases: []string{"a"},
	}

	sinceFlag := &cli.StringFlag{
		Name:    "since",
		Usage:   "only refresh records updated since this timestamp (format: '2006-01-02T15:04:05')",
		Aliases: []string{"s"},
	}

	fromDateFlag := &cli.StringFlag{
		Name:    "fromDate",
		Usage:   "start date for the financial year to sync (format: '2006-01-02')",
		Aliases: []string{"f"},
	}

	// Define all application commands.
	loginCmd := &cli.Command{
		Name:  "login",
		Usage: "Authorize the application with your Xero account",
		Flags: []cli.Flag{configFlag},
		Action: func(ctx context.Context, c *cli.Command) error {
			return app.Login(ctx, c.String("config"))
		},
	}

	wipeCmd := &cli.Command{
		Name:  "wipe",
		Usage: "Delete the local token and database files for security",
		Flags: []cli.Flag{configFlag},
		Action: func(ctx context.Context, c *cli.Command) error {
			return app.Wipe(ctx, c.String("config"))
		},
	}

	bankTransactionsCmd := &cli.Command{
		Name:    "bank-transactions",
		Usage:   "Fetch and save bank transactions from Xero",
		Aliases: []string{"bt"},
		Flags:   []cli.Flag{configFlag, agoFlag, sinceFlag, fromDateFlag},
		Action: func(ctx context.Context, c *cli.Command) error {
			fromDate, ifModifiedSince, err := parseDateFlags(c.String("fromDate"), c.String("since"), c.String("ago"))
			if err != nil {
				return err
			}
			return app.SyncBankTransactions(ctx, c.String("config"), fromDate, ifModifiedSince)
		},
	}

	invoicesCmd := &cli.Command{
		Name:    "invoices",
		Usage:   "Fetch and save invoices from Xero",
		Aliases: []string{"inv"},
		Flags:   []cli.Flag{configFlag, agoFlag, sinceFlag, fromDateFlag},
		Action: func(ctx context.Context, c *cli.Command) error {
			fromDate, ifModifiedSince, err := parseDateFlags(c.String("fromDate"), c.String("since"), c.String("ago"))
			if err != nil {
				return err
			}
			return app.SyncInvoices(ctx, c.String("config"), fromDate, ifModifiedSince)
		},
	}

	updateRefCmd := &cli.Command{
		Name:    "bank-transaction-reference",
		Usage:   "Update the reference of a single bank transaction",
		Aliases: []string{"btref"},
		Flags: []cli.Flag{
			configFlag,
			&cli.StringFlag{Name: "uuid", Usage: "the UUID of the bank transaction to update", Required: true},
			&cli.StringFlag{Name: "ref", Usage: "the new reference value to set", Required: true},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return app.UpdateBankTransactionReference(ctx, c.String("config"), c.String("uuid"), c.String("ref"))
		},
	}

	// Assemble the root command.
	rootCmd := &cli.Command{
		Name:     "xerocli",
		Usage:    "A CLI tool for interacting with the Xero API",
		Commands: []*cli.Command{loginCmd, wipeCmd, bankTransactionsCmd, invoicesCmd, updateRefCmd},
	}

	return rootCmd
}

// parseDateFlags processes the date-related flags and returns parsed time values.
// It enforces mutual exclusivity between --since and --ago.
func parseDateFlags(fromDateStr, sinceStr, agoStr string) (time.Time, time.Time, error) {
	var fromDate, ifModifiedSince time.Time
	var err error

	if fromDateStr != "" {
		fromDate, err = time.Parse("2006-01-02", fromDateStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --fromDate format: %w", err)
		}
	}

	if sinceStr != "" && agoStr != "" {
		return time.Time{}, time.Time{}, fmt.Errorf("--since and --ago flags are mutually exclusive")
	}

	if sinceStr != "" {
		ifModifiedSince, err = time.Parse("2006-01-02T15:04:05", sinceStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --since format: %w", err)
		}
	}

	if agoStr != "" {
		duration, err := time.ParseDuration(agoStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --ago duration format: %w", err)
		}
		ifModifiedSince = time.Now().Add(-duration)
	}

	return fromDate, ifModifiedSince, nil
}
