package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rorycl/reconciler/config"
)

type mockRunner struct{}

func (m *mockRunner) run() error { return nil }

// mockRunMaker generates a mockRunner
func mockRunMaker(
	filename string,
	action string,
	cfg *config.Config,
	logger *slog.Logger,
	sfMaker sfClientMakerFunc,
	loginAgent oauth2Agent) (cliRunner, error) {
	return &mockRunner{}, nil
}

func verify() error { return nil }

func TestParseCLI(t *testing.T) {

	// Create a temporary dummy config file so os.Stat passes
	tmpDir := t.TempDir()
	validExcel := filepath.Join(tmpDir, "excelFile.xslx")
	if err := os.WriteFile(validExcel, []byte("fakeexcel"), 0644); err != nil {
		t.Fatal(err)
	}
	invalidExcel := filepath.Join(tmpDir, "invalidExcelFile.xslx")
	validConfig := "../../config/config.example.yaml"
	invalidConfig := filepath.Join(tmpDir, "invalidConfig.yaml")

	tests := []struct {
		name string
		args []string
		err  string
	}{
		{
			name: "help",
			args: strings.Split("program -h", " "),
		},
		{
			name: "help link",
			args: strings.Split("program link -h", " "),
		},
		{
			name: "help unlink",
			args: strings.Split("program unlink -h", " "),
		},
		{
			name: "link valid",
			args: []string{"program", "link", "-l", "Error", "-c", validConfig, validExcel},
		},
		{
			name: "link invalid config",
			args: []string{"program", "link", "-l", "Error", "-c", invalidConfig, validExcel},
			err:  "config file does not exist",
		},
		{
			name: "link invalid excel",
			args: []string{"program", "link", "-l", "Error", "-c", validConfig, invalidExcel},
			err:  "excel file:",
		},
		{
			name: "link invalid loglevel",
			args: []string{"program", "link", "-l", "InvalidLog", "-c", validConfig, validExcel},
			err:  "expected a log level",
		},
		{
			name: "unlink valid",
			args: []string{"program", "unlink", "-l", "Error", "-c", validConfig, validExcel},
		},
		{
			name: "unlink invalid config",
			args: []string{"program", "unlink", "-l", "Error", "-c", invalidConfig, validExcel},
			err:  "config file does not exist",
		},
		{
			name: "unlink invalid excel",
			args: []string{"program", "unlink", "-l", "Error", "-c", validConfig, invalidExcel},
			err:  "excel file:",
		},
		{
			name: "unlink invalid loglevel",
			args: []string{"program", "unlink", "-l", "InvalidLog", "-c", validConfig, validExcel},
			err:  "expected a log level",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			cmd := BuildCLI(mockRunMaker)
			cmd.Writer = io.Discard
			cmd.ErrWriter = io.Discard

			err := cmd.Run(context.Background(), tt.args)

			if tt.err != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.err)
				}
				if !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("error %q did not contain %q", err.Error(), tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
