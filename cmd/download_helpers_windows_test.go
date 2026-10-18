//go:build windows

package main

import "testing"

func TestResolveLocalDownloadPathRejectsWindowsSeparatorTraversal(t *testing.T) {
	root := t.TempDir()
	if _, err := resolveLocalDownloadPath(root, `safe\..\..\escape.txt`); err == nil {
		t.Fatal("expected Windows-separator traversal to be rejected")
	}
}
