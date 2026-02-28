package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunner implements the App WebRunner interface for testing.
type MockWebRunner struct{}

func (m *MockWebRunner) RunWebServer() error { return nil }

// MockAppMaker generates a WebRunner
func MockAppMaker(configFile string, logLevel slog.Level, inDevelopment bool, staticPath, templatePath, sqlPath, databasePath string) (WebRunner, error) {
	return &MockWebRunner{}, nil
}

func verify() error { return nil }

func TestParseCLI(t *testing.T) {

	// Create a temporary dummy config file so os.Stat passes
	tmpDir := t.TempDir()
	validConfig := filepath.Join(tmpDir, "valid.yaml")
	if err := os.WriteFile(validConfig, []byte("fake: config"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name            string
		args            []string
		wantErrContains string
	}{
		{
			name: "help",
			args: strings.Split("program -h", " "),
		},
		{
			name: "all options valid",
			args: []string{"program", "-l", "Error", "-s", tmpDir, "-t", tmpDir, "-q", tmpDir, "-d", "whatever", validConfig},
		},
		{
			name:            "no config",
			args:            []string{"program", "-l", "Error", "-s", tmpDir, "-t", tmpDir, "-q", tmpDir, "-d", "whatever"},
			wantErrContains: "config file not provided",
		},
		{
			name:            "no sqlDir",
			args:            []string{"program", "-l", "Error", "-s", tmpDir, "-t", tmpDir, "-d", "whatever", validConfig},
			wantErrContains: `Required flag "sqlPath" not set`,
		},
		{
			name:            "no database",
			args:            []string{"program", "-l", "Error", "-s", tmpDir, "-t", tmpDir, "-q", tmpDir, validConfig},
			wantErrContains: `Required flag "database" not set`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			cmd := BuildCLI(MockAppMaker, verify)
			cmd.Writer = io.Discard
			cmd.ErrWriter = io.Discard

			err := cmd.Run(context.Background(), tt.args)

			if tt.wantErrContains != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrContains)
				}
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("error %q did not contain %q", err.Error(), tt.wantErrContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
