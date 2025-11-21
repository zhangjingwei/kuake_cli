package sdk

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewQuarkClient(t *testing.T) {
	tests := []struct {
		name        string
		configPath  string
		setupConfig func() string
		wantPanic   bool
		cleanup     func(string)
	}{
		{
			name:       "create client with valid config",
			configPath: "",
			setupConfig: func() string {
				tmpFile := filepath.Join(t.TempDir(), "config.json")
				config := &Config{
					Quark: struct {
						AccessTokens []string `json:"access_tokens"`
					}{
						AccessTokens: []string{"test_token=value1; test_token2=value2;"},
					},
				}
				SaveConfig(tmpFile, config)
				return tmpFile
			},
			wantPanic: false,
			cleanup: func(path string) {
				os.Remove(path)
			},
		},
		{
			name:       "create client with empty tokens",
			configPath: "",
			setupConfig: func() string {
				tmpFile := filepath.Join(t.TempDir(), "config_empty.json")
				config := &Config{
					Quark: struct {
						AccessTokens []string `json:"access_tokens"`
					}{
						AccessTokens: []string{},
					},
				}
				SaveConfig(tmpFile, config)
				return tmpFile
			},
			wantPanic: true,
			cleanup: func(path string) {
				os.Remove(path)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := tt.configPath
			if tt.setupConfig != nil {
				tmpPath := tt.setupConfig()
				if tmpPath != "" {
					configPath = tmpPath
				}
			}

			defer func() {
				r := recover()
				if (r != nil) != tt.wantPanic {
					t.Errorf("NewQuarkClient() panic = %v, wantPanic %v", r, tt.wantPanic)
				}
				if tt.cleanup != nil {
					tt.cleanup(configPath)
				}
			}()

			client := NewQuarkClient(configPath)
			if client == nil {
				t.Errorf("NewQuarkClient() returned nil")
			}

			if !tt.wantPanic {
				if client.baseURL == "" {
					t.Errorf("NewQuarkClient() client has empty baseURL")
				}
				if client.accessToken == "" {
					t.Errorf("NewQuarkClient() client has empty accessToken")
				}
				if len(client.accessTokens) == 0 {
					t.Errorf("NewQuarkClient() client has empty accessTokens")
				}
			}
		})
	}
}

func TestSetBaseURL(t *testing.T) {
	client := createTestClient(t)
	if client == nil {
		t.Fatal("Failed to create test client")
	}

	testURL := "https://test.example.com"
	client.SetBaseURL(testURL)

	if client.baseURL != testURL {
		t.Errorf("SetBaseURL() = %v, want %v", client.baseURL, testURL)
	}
}

func TestGetCookies(t *testing.T) {
	client := createTestClient(t)
	if client == nil {
		t.Fatal("Failed to create test client")
	}

	cookies := client.GetCookies()
	if cookies == nil {
		t.Errorf("GetCookies() returned nil")
	}
}

func TestParseCookie(t *testing.T) {
	client := createTestClient(t)
	if client == nil {
		t.Fatal("Failed to create test client")
	}

	tests := []struct {
		name      string
		cookieStr string
		wantCount int
	}{
		{
			name:      "parse simple cookie",
			cookieStr: "key1=value1; key2=value2",
			wantCount: 2,
		},
		{
			name:      "parse cookie with spaces",
			cookieStr: "key1 = value1 ; key2 = value2 ",
			wantCount: 2,
		},
		{
			name:      "parse empty cookie",
			cookieStr: "",
			wantCount: 0,
		},
		{
			name:      "parse cookie with empty parts",
			cookieStr: "key1=value1;;key2=value2;",
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cookies := client.parseCookie(tt.cookieStr)
			if len(cookies) != tt.wantCount {
				t.Errorf("parseCookie() returned %d cookies, want %d", len(cookies), tt.wantCount)
			}
		})
	}
}

func TestParseResponse(t *testing.T) {
	client := createTestClient(t)
	if client == nil {
		t.Fatal("Failed to create test client")
	}

	tests := []struct {
		name    string
		respMap map[string]interface{}
		target  interface{}
		wantErr bool
	}{
		{
			name: "parse valid response",
			respMap: map[string]interface{}{
				"code":   0,
				"status": 200,
				"data": map[string]interface{}{
					"fid": "test_fid",
				},
			},
			target:  &CreateFolderResponse{},
			wantErr: false,
		},
		{
			name: "parse invalid response",
			respMap: map[string]interface{}{
				"code": "invalid",
			},
			target:  &CreateFolderResponse{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.parseResponse(tt.respMap, tt.target)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseResponse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConvertToFileInfo(t *testing.T) {
	client := createTestClient(t)
	if client == nil {
		t.Fatal("Failed to create test client")
	}

	tests := []struct {
		name string
		qf   QuarkFileInfo
		want *FileInfo
	}{
		{
			name: "convert file info",
			qf: QuarkFileInfo{
				Name:        "test.txt",
				Path:        "/test.txt",
				Size:        1024,
				IsDirectory: false,
			},
			want: &FileInfo{
				Name:        "test.txt",
				Path:        "/test.txt",
				Size:        1024,
				IsDirectory: false,
			},
		},
		{
			name: "convert directory info",
			qf: QuarkFileInfo{
				Name:        "test_dir",
				Path:        "/test_dir",
				Size:        0,
				IsDirectory: true,
			},
			want: &FileInfo{
				Name:        "test_dir",
				Path:        "/test_dir",
				Size:        0,
				IsDirectory: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.ConvertToFileInfo(tt.qf)
			if got.Name != tt.want.Name {
				t.Errorf("ConvertToFileInfo() Name = %v, want %v", got.Name, tt.want.Name)
			}
			if got.Path != tt.want.Path {
				t.Errorf("ConvertToFileInfo() Path = %v, want %v", got.Path, tt.want.Path)
			}
			if got.Size != tt.want.Size {
				t.Errorf("ConvertToFileInfo() Size = %v, want %v", got.Size, tt.want.Size)
			}
			if got.IsDirectory != tt.want.IsDirectory {
				t.Errorf("ConvertToFileInfo() IsDirectory = %v, want %v", got.IsDirectory, tt.want.IsDirectory)
			}
		})
	}
}

// createTestClient 创建测试用的客户端
func createTestClient(t *testing.T) *QuarkClient {
	tmpFile := filepath.Join(t.TempDir(), "test_config.json")
	config := &Config{
		Quark: struct {
			AccessTokens []string `json:"access_tokens"`
		}{
			AccessTokens: []string{"test_token=value1; test_token2=value2;"},
		},
	}

	if err := SaveConfig(tmpFile, config); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Failed to create client: %v", r)
		}
	}()

	client := NewQuarkClient(tmpFile)
	return client
}

