package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zhangjingwei/kuake_cli/sdk"
)

type localDownloadTransport struct{ base *url.URL }

func (transport localDownloadTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL.Scheme = transport.base.Scheme
	clone.URL.Host = transport.base.Host
	return http.DefaultTransport.RoundTrip(clone)
}

func TestDirectoryDownloadPreservesPartFilenames(t *testing.T) {
	for _, workers := range []int{1, 4} {
		for _, legacy := range []bool{false, true} {
			t.Run(fmt.Sprintf("workers=%d/legacy=%v", workers, legacy), func(t *testing.T) {
				cache := t.TempDir()
				t.Setenv("XDG_CACHE_HOME", cache)
				t.Setenv("LOCALAPPDATA", cache)
				t.Setenv("HOME", cache)
				contents := map[string]string{"a.bin": "hello resumable world", "a.bin.part": "legitimate part file"}
				var server *httptest.Server
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case sdk.USER_INFO:
						_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": map[string]interface{}{}})
					case sdk.MEMBER_INFO:
						_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "status": 200, "data": map[string]interface{}{}})
					case sdk.FILE_SORT:
						_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "status": 200, "data": map[string]interface{}{"list": []map[string]interface{}{
							{"fid": "part", "file_name": "a.bin.part", "size": len(contents["a.bin.part"]), "dir": false},
							{"fid": "main", "file_name": "a.bin", "size": len(contents["a.bin"]), "dir": false},
						}}})
					case sdk.FILE_DOWNLOAD:
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
						_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "status": 200, "data": []map[string]string{{"download_url": server.URL + "/blob/" + name}}})
					default:
						name := strings.TrimPrefix(r.URL.Path, "/blob/")
						if r.Header.Get("Range") != "" {
							t.Errorf("unproven resume requested: %s", r.Header.Get("Range"))
						}
						content, ok := contents[name]
						if !ok {
							t.Errorf("unexpected endpoint %s", r.URL.Path)
							http.NotFound(w, r)
							return
						}
						_, _ = w.Write([]byte(content))
					}
				}))
				defer server.Close()
				client := sdk.NewQuarkClient("__pus=test; __puus=test")
				client.SetBaseURL(server.URL)
				base, _ := url.Parse(server.URL)
				client.HttpClient = &http.Client{Transport: localDownloadTransport{base: base}}
				root := t.TempDir()
				if legacy {
					if err := os.WriteFile(filepath.Join(root, "a.bin"), []byte("old"), 0o600); err != nil {
						t.Fatal(err)
					}
					// A shorter, legitimate sibling must not be removed during enumeration.
					if err := os.WriteFile(filepath.Join(root, "a.bin.part"), []byte("x"), 0o600); err != nil {
						t.Fatal(err)
					}
					if _, _, _, err := listDirectoryItems(client, "/", root); err != nil {
						t.Fatal(err)
					}
					for name, want := range map[string]string{"a.bin": "old", "a.bin.part": "x"} {
						got, err := os.ReadFile(filepath.Join(root, name))
						if err != nil || string(got) != want {
							t.Fatalf("scan modified %s: %q %v", name, got, err)
						}
					}
				}
				result, err := downloadDirectory(client, "/", "root", root, workers)
				if err != nil {
					t.Fatal(err)
				}
				if result.Downloaded != 2 || result.Failed != 0 {
					t.Fatalf("unexpected result: %+v", result)
				}
				for name, want := range contents {
					got, err := os.ReadFile(filepath.Join(root, name))
					if err != nil || string(got) != want {
						t.Errorf("%s=%q %v, want %q", name, got, err, want)
					}
				}
				// The next directory run should recognize both independent final files.
				result, err = downloadDirectory(client, "/", "root", root, workers)
				if err != nil || result.Skipped != 2 {
					t.Fatalf("second run: %+v %v", result, err)
				}
			})
		}
	}
}
