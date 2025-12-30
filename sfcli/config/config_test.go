package config

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	tf, err := os.CreateTemp("", "tmp_config_*")
	if err != nil {
		t.Fatal(err)
	}
	_ = os.RemoveAll(tf.Name())
	_, err = LoadConfig(tf.Name())
	if err == nil {
		t.Errorf("expected error loading empty file %q", tf.Name())
	}

	rc, err := LoadConfig("../config.example.yaml")
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	// Validation checks.
	old := rc.Salesforce.ClientID
	rc.Salesforce.ClientID = ""
	if err := validateAndPrepareConfig(rc); err == nil {
		t.Errorf("expected validation error for ClientID")
	}
	rc.Salesforce.ClientID = old

	old = rc.Salesforce.Query
	rc.Salesforce.Query = "WHERE 1 = 1"
	if err := validateAndPrepareConfig(rc); err == nil {
		t.Errorf("expected validation error for Query")
	}
	rc.Salesforce.Query = old

	old = rc.Salesforce.LinkingFieldName
	rc.Salesforce.LinkingFieldName = ""
	if err := validateAndPrepareConfig(rc); err == nil {
		t.Errorf("expected validation error for LinkingFieldName")
	}
	rc.Salesforce.LinkingFieldName = old

}
