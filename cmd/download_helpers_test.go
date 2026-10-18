package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveFileDownloadPath(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name string
		dest string
		want string
	}{
		{name: "no extension becomes directory", dest: filepath.Join(root, "downloads"), want: filepath.Join(root, "downloads", "data.xlsx")},
		{name: "extension is explicit filename", dest: filepath.Join(root, "renamed.xlsx"), want: filepath.Join(root, "renamed.xlsx")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveFileDownloadPath(test.dest, "data.xlsx")
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}

	existing := filepath.Join(root, "existing.with-extension")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveFileDownloadPath(existing, "data.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(existing, "data.xlsx"); got != want {
		t.Fatalf("existing directory: got %q, want %q", got, want)
	}
}

func TestResolveDirectoryDownloadPathAlwaysCreatesDirectory(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "looks-like-a-file.xlsx")
	got, err := resolveDirectoryDownloadPath(dest, "remote")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(got)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("%q was not created as a directory", got)
	}
}

func TestResolveDirectoryDownloadPathUsesRemoteNameByDefault(t *testing.T) {
	root := t.TempDir()
	oldWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWorkingDirectory) })

	got, err := resolveDirectoryDownloadPath("", "remote-folder")
	if err != nil {
		t.Fatal(err)
	}
	if got != "remote-folder" {
		t.Fatalf("got %q, want remote-folder", got)
	}
}

func TestParseDownloadArgs(t *testing.T) {
	args, workers, err := parseDownloadArgs([]string{"/remote", "./local", "--workers", "8"})
	if err != nil {
		t.Fatal(err)
	}
	if workers != 8 || len(args) != 2 || args[0] != "/remote" || args[1] != "./local" {
		t.Fatalf("unexpected parse result: args=%v workers=%d", args, workers)
	}
	if _, _, err := parseDownloadArgs([]string{"/remote", "--workers", "17"}); err == nil {
		t.Fatal("expected invalid workers error")
	}
}

func TestResolveLocalDownloadPathStaysWithinRoot(t *testing.T) {
	root := t.TempDir()
	got, err := resolveLocalDownloadPath(root, "nested/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(filepath.Join(root, "nested", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if _, err := resolveLocalDownloadPath(root, "../escape.txt"); err == nil {
		t.Fatal("expected parent traversal to be rejected")
	}
}
