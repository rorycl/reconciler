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
			args: []string{"program", "-h"},
		},
		{
			name: "all options valid",
			args: []string{"program", "-l", "Error", validConfig},
		},
		{
			name: "defaults valid",
			args: []string{"program", validConfig},
		},
		{
			name:            "invalid debug level",
			args:            []string{"program", "-l", "Whatever", validConfig},
			wantErrContains: "expected a debug level of 'Error'",
		},
		{
			name:            "missing config file",
			args:            []string{"program", "-l", "Error"},
			wantErrContains: "config file not provided",
		},
		{
			name:            "invalid config",
			args:            []string{"program", filepath.Join(t.TempDir(), "whatever")},
			wantErrContains: "could not stat config file",
		},
		{
			name:            "invalid debug",
			args:            []string{"program", "-l", "Whatever", "../../config/config.example.yaml"},
			wantErrContains: "expected a debug level",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			cmd := BuildCLI(MockAppMaker)
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
