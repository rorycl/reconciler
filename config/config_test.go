package config

import "testing"

func TestConfig(t *testing.T) {

	config, err := Load("config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}

	if got, want := config.DatabasePath, "./reconciliation.db"; got != want {
		t.Errorf("got %s want %s", got, want)
	}

}
