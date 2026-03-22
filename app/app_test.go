package app

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestAppInit(t *testing.T) {

	var programLevel = new(slog.LevelVar) // Info by default
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: programLevel})
	slog.SetDefault(slog.New(h))

	logger := slog.Default()

	tests := []struct {
		name          string
		configFile    string
		logLevel      slog.Level
		inDevelopment bool
		staticPath    string
		templatePath  string
		sqlPath       string
		databasePath  string
		err           error
	}{
		{
			name:          "default production",
			configFile:    "../config/config.example.yaml",
			logLevel:      slog.LevelDebug,
			inDevelopment: false,
			databasePath:  ":memory:",
			err:           nil,
		},
		{
			name:          "production error wrong db",
			configFile:    "../config/config.example.yaml",
			logLevel:      slog.LevelDebug,
			inDevelopment: false,
			databasePath:  "a database",
			err:           errors.New("cannot have non :memory: database"),
		},
		{
			name:          "production error wrong config",
			configFile:    t.TempDir(),
			logLevel:      slog.LevelDebug,
			inDevelopment: false,
			databasePath:  ":memory:",
			err:           errors.New("config file not found"),
		},
		{
			name:          "default indevelopment",
			configFile:    "../config/config.example.yaml",
			logLevel:      slog.LevelDebug,
			inDevelopment: true,
			staticPath:    "../web/static",
			templatePath:  "../web/templates",
			sqlPath:       "../db/sql",
			databasePath:  filepath.Join(t.TempDir(), "test.db"),
			err:           nil,
		},
		{
			name:          "indevelopment sqlpath wrong",
			configFile:    "../config/config.example.yaml",
			logLevel:      slog.LevelDebug,
			inDevelopment: true,
			staticPath:    "../web/static",
			templatePath:  "../web/templates",
			sqlPath:       t.TempDir(), // incorrect
			databasePath:  filepath.Join(t.TempDir(), "test.db"),
			err:           errors.New("should not be able to open 'schema.sql' file"),
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("%d_%s", ii, tt.name), func(t *testing.T) {
			programLevel.Set(tt.logLevel)
			_, err := NewApp(
				tt.configFile,
				logger,
				tt.inDevelopment,
				tt.staticPath,
				tt.templatePath,
				tt.sqlPath,
				tt.databasePath,
			)
			if err != nil && tt.err == nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if err == nil && tt.err != nil {
				t.Fatalf("expected error: %v got none", tt.err)
			}

		})
	}
}
