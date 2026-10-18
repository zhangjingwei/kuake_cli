package sdk

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Serialize writes to the same destination while allowing directory workers to
// download different files concurrently. Entries live only while in use.
var downloadLocks = struct {
	sync.Mutex
	entries map[string]*downloadLock
}{entries: make(map[string]*downloadLock)}

type downloadLock struct {
	mu    sync.Mutex
	users int
}

func lockDownload(target string) func() {
	downloadLocks.Lock()
	entry := downloadLocks.entries[target]
	if entry == nil {
		entry = &downloadLock{}
		downloadLocks.entries[target] = entry
	}
	entry.users++
	downloadLocks.Unlock()
	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		downloadLocks.Lock()
		entry.users--
		if entry.users == 0 {
			delete(downloadLocks.entries, target)
		}
		downloadLocks.Unlock()
	}
}

type downloadIdentity struct {
	Version int    `json:"version"`
	FID     string `json:"fid"`
	Target  string `json:"target"`
}

// Resume state is deliberately outside the destination tree: a remote file
// named "x.part" must never be mistaken for the partial download of "x".
// Only state bearing the matching remote ID and absolute target is reusable.
func prepareDownloadPartial(fid, target string) (string, error) {
	target, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	identity := downloadIdentity{Version: 1, FID: fid, Target: target}
	payload, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve download cache: %w", err)
	}
	key := fmt.Sprintf("%x", sha256.Sum256(payload))
	directory := filepath.Join(cache, "kuake", "downloads", key)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	partial := filepath.Join(directory, "data.part")
	metadata := filepath.Join(directory, "identity.json")
	stored, readErr := os.ReadFile(metadata)
	var previous downloadIdentity
	if readErr == nil && json.Unmarshal(stored, &previous) == nil && previous == identity {
		return partial, nil
	}
	if readErr != nil && !os.IsNotExist(readErr) {
		return "", readErr
	}
	// Never adopt data without provenance, including old sibling .part files.
	if err := os.Remove(partial); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.WriteFile(metadata, payload, 0o600); err != nil {
		return "", err
	}
	return partial, nil
}

// Stage the final copy beside its destination so cache and downloads may live
// on different filesystems. The old target survives transfer/copy failures.
func installDownloadedFile(partial, target string) error {
	source, err := os.Open(partial)
	if err != nil {
		return err
	}
	defer source.Close()
	temporary, err := os.CreateTemp(filepath.Dir(target), ".kuake-install-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := io.Copy(temporary, source); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return err
	}
	// Failure to clean cache after a successful install must not mark the download
	// failed. Its provenance remains valid for any subsequent retry.
	_ = source.Close()
	_ = os.Remove(partial)
	return nil
}
