package dbquery

import "testing"

func TestConfig(t *testing.T) {

	c, err := LoadConfig("config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := c.DatabasePath, "./test.db"; got != want {
		t.Errorf("got %q want %q", got, want)
	}

	_, err = LoadConfig("config.notexist.yaml")
	if err == nil {
		t.Fatal("expected error for non-existent yaml file")
	}
}
