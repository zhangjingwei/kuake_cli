package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zhangjingwei/kuake_cli/sdk"
)

const credentialsFileName = "credentials.json"

type storedCredentials struct {
	Version int    `json:"version"`
	Cookie  string `json:"cookie"`
}

func credentialsPath() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("KUAKE_CONFIG_DIR")); configured != "" {
		return filepath.Join(configured, credentialsFileName), nil
	}
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(configRoot, "kuake", credentialsFileName), nil
}

func saveStoredCookie(cookie string) (string, error) {
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		return "", fmt.Errorf("cookie is empty")
	}
	path, err := credentialsPath()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create credentials directory: %w", err)
	}
	if err := secureCredentialPath(dir, true); err != nil {
		return "", fmt.Errorf("secure credentials directory: %w", err)
	}
	payload, err := json.MarshalIndent(storedCredentials{Version: 1, Cookie: cookie}, "", "  ")
	if err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(dir, ".credentials-*")
	if err != nil {
		return "", fmt.Errorf("create temporary credentials file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := secureCredentialPath(temporaryPath, false); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("secure temporary credentials file: %w", err)
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return "", fmt.Errorf("install credentials file: %w", err)
	}
	if err := secureCredentialPath(path, false); err != nil {
		return "", fmt.Errorf("secure credentials file: %w", err)
	}
	return path, nil
}

func loadStoredCookie() (string, string, error) {
	path, err := credentialsPath()
	if err != nil {
		return "", "", err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", path, nil
		}
		return "", path, fmt.Errorf("read credentials: %w", err)
	}
	var credentials storedCredentials
	if err := json.Unmarshal(payload, &credentials); err != nil {
		return "", path, fmt.Errorf("decode credentials: %w", err)
	}
	if credentials.Version != 1 {
		return "", path, fmt.Errorf("unsupported credentials version: %d", credentials.Version)
	}
	return strings.TrimSpace(credentials.Cookie), path, nil
}

func clearStoredCookie() (string, error) {
	path, err := credentialsPath()
	if err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("remove credentials: %w", err)
	}
	return path, nil
}

func storedCookieNames(cookie string) []string {
	names := make([]string, 0)
	for _, part := range strings.Split(cookie, ";") {
		name, _, found := strings.Cut(strings.TrimSpace(part), "=")
		if found && name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func handleAuth(args []string, commandCookie string) *CLIResult {
	if len(args) != 1 {
		return &CLIResult{Success: false, Code: "INVALID_ARGS", Message: "Usage: auth <save|status|clear>"}
	}
	switch args[0] {
	case "save":
		cookie := sdk.ResolveEnvCookieString()
		if cookie == "" && strings.TrimSpace(commandCookie) != "" {
			cookie = normalizeQuarkCookieInput(commandCookie)
		}
		if cookie == "" {
			return &CLIResult{
				Success: false,
				Code:    "COOKIE_NOT_SET",
				Message: "set KUAKE_COOKIE or KUAKE_PUS+KUAKE_PUUS, then run `kuake auth save`",
			}
		}
		path, err := saveStoredCookie(cookie)
		if err != nil {
			return &CLIResult{Success: false, Code: "SAVE_FAILED", Message: err.Error()}
		}
		return &CLIResult{Success: true, Code: "OK", Message: "Cookie saved securely", Data: map[string]interface{}{
			"path": path, "cookie_names": storedCookieNames(cookie),
		}}
	case "status":
		cookie, path, err := loadStoredCookie()
		if err != nil {
			return &CLIResult{Success: false, Code: "LOAD_FAILED", Message: err.Error()}
		}
		return &CLIResult{Success: true, Code: "OK", Message: "Credential status", Data: map[string]interface{}{
			"configured": cookie != "", "path": path, "cookie_names": storedCookieNames(cookie),
		}}
	case "clear":
		path, err := clearStoredCookie()
		if err != nil {
			return &CLIResult{Success: false, Code: "CLEAR_FAILED", Message: err.Error()}
		}
		return &CLIResult{Success: true, Code: "OK", Message: "Stored Cookie cleared", Data: map[string]interface{}{"path": path}}
	default:
		return &CLIResult{Success: false, Code: "INVALID_ARGS", Message: "Usage: auth <save|status|clear>"}
	}
}
