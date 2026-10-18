package sdk

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func isolateDownloadCache(t *testing.T) {
	t.Helper()
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("LOCALAPPDATA", cache)
	t.Setenv("HOME", cache)
}

func TestDownloadFilePreservesLegitimatePartNames(t *testing.T) {
	for _, order := range [][]string{{"a.bin.part", "a.bin"}, {"a.bin", "a.bin.part"}} {
		for _, concurrent := range []bool{false, true} {
			t.Run(fmt.Sprintf("%v/concurrent=%v", order, concurrent), func(t *testing.T) {
				isolateDownloadCache(t)
				contents := map[string]string{"a.bin": "hello resumable world", "a.bin.part": "WRONG!"}
				var server *httptest.Server
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == FILE_DOWNLOAD {
						var request struct {
							FIDs []string `json:"fids"`
						}
						if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
							t.Error(err)
							return
						}
						name := "a.bin"
						if request.FIDs[0] == "part" {
							name = "a.bin.part"
						}
						_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "status": 200, "data": []map[string]string{{"download_url": server.URL + "/" + name}}})
						return
					}
					if r.Header.Get("Range") != "" {
						t.Errorf("unproven resume requested: %s", r.Header.Get("Range"))
					}
					_, _ = w.Write([]byte(contents[strings.TrimPrefix(r.URL.Path, "/")]))
				}))
				defer server.Close()
				qc := NewQuarkClient("__pus=test; __puus=test")
				qc.SetBaseURL(server.URL)
				qc.authCheckValid = true
				qc.lastAuthCheck = time.Now()
				dir := t.TempDir()
				var wg sync.WaitGroup
				for _, name := range order {
					run := func(name string) {
						defer wg.Done()
						fid := "main"
						if name == "a.bin.part" {
							fid = "part"
						}
						if err := qc.DownloadFile(fid, filepath.Join(dir, name), name, nil); err != nil {
							t.Error(err)
						}
					}
					wg.Add(1)
					if concurrent {
						go run(name)
					} else {
						run(name)
					}
				}
				wg.Wait()
				for name, want := range contents {
					got, err := os.ReadFile(filepath.Join(dir, name))
					if err != nil || string(got) != want {
						t.Errorf("%s = %q, %v; want %q", name, got, err, want)
					}
				}
			})
		}
	}
}

func TestDownloadFileResumesOnlyItsInterruptedState(t *testing.T) {
	isolateDownloadCache(t)
	content := "hello resumable world"
	requests := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == FILE_DOWNLOAD {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "status": 200, "data": []map[string]string{{"download_url": server.URL + "/blob"}}})
			return
		}
		requests++
		if requests == 1 {
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			_, _ = w.Write([]byte(content[:6]))
			return
		}
		if got := r.Header.Get("Range"); got != "bytes=6-" {
			t.Errorf("Range=%q", got)
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 6-%d/%d", len(content)-1, len(content)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte(content[6:]))
	}))
	defer server.Close()
	qc := NewQuarkClient("__pus=test; __puus=test")
	qc.SetBaseURL(server.URL)
	qc.authCheckValid = true
	qc.lastAuthCheck = time.Now()
	target := filepath.Join(t.TempDir(), "a.bin")
	if err := os.WriteFile(target, []byte("old target"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target+".part", []byte("legitimate sibling"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := qc.DownloadFile("main", target, "a.bin", nil); err == nil {
		t.Fatal("expected interrupted transfer")
	}
	old, _ := os.ReadFile(target)
	if string(old) != "old target" {
		t.Fatalf("old target modified: %q", old)
	}
	if err := qc.DownloadFile("main", target, "a.bin", nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != content {
		t.Fatalf("content=%q", got)
	}
	sibling, _ := os.ReadFile(target + ".part")
	if string(sibling) != "legitimate sibling" {
		t.Fatalf("sibling modified: %q", sibling)
	}
}

func TestDownloadStateRequiresMatchingIdentity(t *testing.T) {
	isolateDownloadCache(t)
	target := filepath.Join(t.TempDir(), "file.bin")
	partial, err := prepareDownloadPartial("first", target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(partial, []byte("untrusted"), 0o600); err != nil {
		t.Fatal(err)
	}
	other, err := prepareDownloadPartial("second", target)
	if err != nil {
		t.Fatal(err)
	}
	if partial == other {
		t.Fatal("different remote IDs share state")
	}
	another, err := prepareDownloadPartial("first", target+".part")
	if err != nil {
		t.Fatal(err)
	}
	if partial == another {
		t.Fatal("different target paths share state")
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(partial), "identity.json"), []byte(`{"version":1,"fid":"wrong","target":"wrong"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareDownloadPartial("first", target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(partial); !os.IsNotExist(err) {
		t.Fatal("mismatched state reused")
	}
}

func TestInstallDownloadedFilePreservesTargetOnFailure(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.bin")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := installDownloadedFile(filepath.Join(root, "missing"), target); err == nil {
		t.Fatal("expected error")
	}
	got, _ := os.ReadFile(target)
	if string(got) != "original" {
		t.Fatalf("target changed: %q", got)
	}
	// Installing over a nonempty directory must fail without destroying it or
	// leaking the sibling install temp, while leaving resumable cache data intact.
	partial := filepath.Join(root, "partial")
	_ = os.WriteFile(partial, []byte("new"), 0o600)
	directory := filepath.Join(root, "directory")
	_ = os.Mkdir(directory, 0o700)
	_ = os.WriteFile(filepath.Join(directory, "keep"), []byte("keep"), 0o600)
	if err := installDownloadedFile(partial, directory); err == nil {
		t.Fatal("expected directory error")
	}
	if _, err := os.Stat(partial); err != nil {
		t.Fatal(err)
	}
	matches, _ := filepath.Glob(filepath.Join(root, ".kuake-install-*"))
	if len(matches) != 0 {
		t.Fatalf("temporary copies leaked: %v", matches)
	}
}

func TestConcurrentDownloadSameTarget(t *testing.T) {
	isolateDownloadCache(t)
	content := "same target concurrent content"
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == FILE_DOWNLOAD {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "status": 200, "data": []map[string]string{{"download_url": server.URL + "/blob"}}})
			return
		}
		if r.Header.Get("Range") != "" {
			t.Errorf("unexpected Range: %s", r.Header.Get("Range"))
		}
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()
	qc := NewQuarkClient("__pus=test; __puus=test")
	qc.SetBaseURL(server.URL)
	qc.authCheckValid = true
	qc.lastAuthCheck = time.Now()
	target := filepath.Join(t.TempDir(), "target.bin")
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := qc.DownloadFile("main", target, "target.bin", nil); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	got, err := os.ReadFile(target)
	if err != nil || string(got) != content {
		t.Fatalf("target=%q %v", got, err)
	}
	downloadLocks.Lock()
	defer downloadLocks.Unlock()
	if len(downloadLocks.entries) != 0 {
		t.Fatalf("lock entries leaked: %d", len(downloadLocks.entries))
	}
}
