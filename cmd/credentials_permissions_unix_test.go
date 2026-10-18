//go:build !windows

package main

import (
	"os"
	"testing"
)

func assertStoredCredentialPermissions(t *testing.T, configDir, path string) {
	t.Helper()
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("credentials mode = %o, want 600", got)
	}
	dirInfo, err := os.Stat(configDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("config directory mode = %o, want 700", got)
	}
}
