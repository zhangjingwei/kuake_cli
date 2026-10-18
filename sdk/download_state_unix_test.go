//go:build !windows

package sdk

import (
	"os"
	"path/filepath"
	"testing"
)

// Check the stronger invariant directly without changing the process-global
// umask: final copies remain owner-only even under the default permissive mask.
func TestInstalledDownloadRemainsPrivate(t *testing.T) {
	root := t.TempDir()
	partial, target := filepath.Join(root, "partial"), filepath.Join(root, "target")
	if err := os.WriteFile(partial, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := installDownloadedFile(partial, target); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		t.Fatalf("final file grants group/other permissions: %o", mode)
	}
}
