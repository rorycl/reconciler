package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"xerocli/app/xero"
	"xerocli/db"
)

// App is the central orchestrator for the application's business logic.
// It coordinates interactions between configuration, the Xero API client, and the database.
type App struct{}

// New creates and returns a new App instance.
func New() *App {
	return &App{}
}

// Login orchestrates the OAuth2 login flow.
// It loads configuration and initiates the interactive authentication process.
func (a *App) Login(ctx context.Context, cfgPath string) error {
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		return err
	}
	return xero.InitiateLogin(ctx, cfg.OAuth2Config, cfg.TokenFilePath)
}

// Wipe removes local data for security and confidentiality.
// It deletes the OAuth2 token file and the DuckDB database file.
func (a *App) Wipe(ctx context.Context, cfgPath string) error {
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		return err
	}

	log.Printf("Deleting token file at: %s", cfg.TokenFilePath)
	if err := os.Remove(cfg.TokenFilePath); err != nil {
		return fmt.Errorf("failed to delete token file: %w", err)
	}

	log.Printf("Deleting database file at: %s", cfg.DatabasePath)
	if err := os.Remove(cfg.DatabasePath); err != nil {
		return fmt.Errorf("failed to delete database file: %w", err)
	}

	log.Println("Wipe complete.")
	return nil
}

// SyncBankTransactions fetches bank transactions from Xero and persists them to the database.
// It handles filtering by financial year and incremental updates.
func (a *App) SyncBankTransactions(ctx context.Context, cfgPath string, fromDate, ifModifiedSince time.Time) error {
	return a.sync(ctx, cfgPath, fromDate, ifModifiedSince, "bank_transactions")
}

// SyncInvoices fetches invoices from Xero and persists them to the database.
// It handles filtering by financial year and incremental updates.
func (a *App) SyncInvoices(ctx context.Context, cfgPath string, fromDate, ifModifiedSince time.Time) error {
	return a.sync(ctx, cfgPath, fromDate, ifModifiedSince, "invoices")
}

// SyncAccounts fetches accounts from Xero and persists them to the database.
// It handles incremental updates.
func (a *App) SyncAccounts(ctx context.Context, cfgPath string, ifModifiedSince time.Time) error {
	return a.sync(ctx, cfgPath, time.Time{}, ifModifiedSince, "accounts")
}

// sync is a generic helper function to handle the common sync logic for different data types.
func (a *App) sync(ctx context.Context, cfgPath string, fromDate, ifModifiedSince time.Time, dataType string) error {
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		return err
	}

	if fromDate.IsZero() {
		fromDate = cfg.FinancialYearStart
		log.Printf("No --fromDate specified, using default from config: %s", fromDate.Format("2006-01-02"))
	}

	if !ifModifiedSince.IsZero() && dataType != "accounts" {
		log.Printf("Only retrieving records modified since: %s", ifModifiedSince.Format(time.RFC1123))
	}

	xeroClient, err := xero.NewClient(ctx, cfg.OAuth2Config, cfg.TokenFilePath)
	if err != nil {
		return fmt.Errorf("failed to create xero client: %w", err)
	}
	log.Println("Xero client authenticated successfully.")

	dbConn, err := db.New(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer dbConn.Close()

	if err := dbConn.InitSchema(); err != nil {
		return fmt.Errorf("failed to initialize database schema: %w", err)
	}

	switch dataType {
	case "bank_transactions":
		log.Println("Fetching Bank Transactions from Xero...")
		records, err := xeroClient.GetBankTransactions(ctx, fromDate, ifModifiedSince)
		if err != nil {
			return err
		}
		log.Printf("Fetched %d bank transactions.", len(records))
		if err := dbConn.UpsertBankTransactions(records); err != nil {
			return fmt.Errorf("failed to upsert bank transactions: %w", err)
		}
		log.Println("Successfully upserted bank transactions to database.")

	case "invoices":
		log.Println("Fetching Invoices from Xero...")
		records, err := xeroClient.GetInvoices(ctx, fromDate, ifModifiedSince)
		if err != nil {
			return err
		}
		log.Printf("Fetched %d invoices.", len(records))
		if err := dbConn.UpsertInvoices(records); err != nil {
			return fmt.Errorf("failed to upsert invoices: %w", err)
		}
		log.Println("Successfully upserted invoices to database.")

	case "accounts":
		log.Println("Fetching Accounts from Xero...")
		records, err := xeroClient.GetAccounts(ctx, ifModifiedSince)
		if err != nil {
			return err
		}
		log.Printf("Fetched %d accounts.", len(records))
		if err := dbConn.UpsertAccounts(records); err != nil {
			return fmt.Errorf("failed to upsert invoices: %w", err)
		}
		log.Println("Successfully upserted accounts to database.")
	default:
		return fmt.Errorf("unknown data type for sync: %s", dataType)
	}

	return nil
}
