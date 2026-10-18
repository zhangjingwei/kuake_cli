package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoredCookieLifecycleAndPermissions(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	t.Setenv("KUAKE_CONFIG_DIR", configDir)
	cookie := "__pus=secret-one; __puus=secret-two; tfstk=secret-three"

	path, err := saveStoredCookie(cookie)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(configDir, credentialsFileName) {
		t.Fatalf("unexpected path %q", path)
	}
	assertStoredCredentialPermissions(t, configDir, path)
	loaded, _, err := loadStoredCookie()
	if err != nil {
		t.Fatal(err)
	}
	if loaded != cookie {
		t.Fatal("stored Cookie did not round trip")
	}
	if _, err := clearStoredCookie(); err != nil {
		t.Fatal(err)
	}
	loaded, _, err = loadStoredCookie()
	if err != nil || loaded != "" {
		t.Fatalf("Cookie still present after clear: cookie=%q err=%v", loaded, err)
	}
}

func TestAuthStatusDoesNotExposeCookieValues(t *testing.T) {
	t.Setenv("KUAKE_CONFIG_DIR", t.TempDir())
	cookie := "__pus=do-not-print; __puus=also-secret"
	if _, err := saveStoredCookie(cookie); err != nil {
		t.Fatal(err)
	}
	result := handleAuth([]string{"status"}, "")
	encoded := result.Message
	for _, value := range result.Data {
		encoded += " " + fmt.Sprint(value)
	}
	if strings.Contains(encoded, "do-not-print") || strings.Contains(encoded, "also-secret") {
		t.Fatal("auth status exposed a Cookie value")
	}
}
