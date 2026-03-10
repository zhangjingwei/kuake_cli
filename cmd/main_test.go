package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsLegacyConfigArg(t *testing.T) {
	validConfig := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(validConfig, []byte(`{
  "Quark": {
    "access_tokens": ["__pus=test;"]
  }
}`), 0644); err != nil {
		t.Fatalf("failed to write valid config: %v", err)
	}

	plainJSON := filepath.Join(t.TempDir(), "test.json")
	if err := os.WriteFile(plainJSON, []byte(`{"name":"fixture"}`), 0644); err != nil {
		t.Fatalf("failed to write plain json: %v", err)
	}

	if !isLegacyConfigArg(validConfig) {
		t.Fatalf("expected valid kuake config to be recognized")
	}

	if isLegacyConfigArg(plainJSON) {
		t.Fatalf("expected ordinary json file not to be recognized as config")
	}

	if isLegacyConfigArg("missing.json") {
		t.Fatalf("expected missing json file not to be recognized as config")
	}
}
