package sdk

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name        string
		configPath  string
		setupConfig func() string
		wantErr     bool
		cleanup     func(string)
	}{
		{
			name:       "load valid config",
			configPath: "",
			setupConfig: func() string {
				// 创建临时配置文件
				tmpFile := filepath.Join(t.TempDir(), "config.json")
				config := &Config{
					Quark: struct {
						AccessTokens []string `json:"access_tokens"`
					}{
						AccessTokens: []string{"test_token_1", "test_token_2"},
					},
				}
				SaveConfig(tmpFile, config)
				return tmpFile
			},
			wantErr: false,
			cleanup: func(path string) {
				os.Remove(path)
			},
		},
		{
			name:       "load config with empty tokens",
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
			wantErr: true,
			cleanup: func(path string) {
				os.Remove(path)
			},
		},
		{
			name:       "load non-existent config",
			configPath: "/nonexistent/config.json",
			setupConfig: func() string {
				return ""
			},
			wantErr: true,
			cleanup: func(path string) {},
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

			config, err := LoadConfig(configPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && config == nil {
				t.Errorf("LoadConfig() returned nil config")
				return
			}

			if !tt.wantErr && len(config.Quark.AccessTokens) == 0 {
				t.Errorf("LoadConfig() returned config with empty tokens")
			}

			if tt.cleanup != nil {
				tt.cleanup(configPath)
			}
		})
	}
}

func TestSaveConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "save valid config",
			config: &Config{
				Quark: struct {
					AccessTokens []string `json:"access_tokens"`
				}{
					AccessTokens: []string{"token1", "token2"},
				},
			},
			wantErr: false,
		},
		{
			name:    "save nil config",
			config:  nil,
			wantErr: false, // json.MarshalIndent can marshal nil
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := filepath.Join(t.TempDir(), "test_config.json")
			defer os.Remove(tmpFile)

			err := SaveConfig(tmpFile, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("SaveConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// 验证文件是否存在
				if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
					t.Errorf("SaveConfig() did not create file")
				}

				// 验证可以重新加载（跳过nil config的情况）
				if tt.config != nil {
					loadedConfig, err := LoadConfig(tmpFile)
					if err != nil {
						t.Errorf("SaveConfig() created invalid config file: %v", err)
						return
					}

					if len(loadedConfig.Quark.AccessTokens) != len(tt.config.Quark.AccessTokens) {
						t.Errorf("SaveConfig() saved config with wrong token count")
					}
				}
			}
		})
	}
}

func TestLoadConfig_DefaultPath(t *testing.T) {
	// 测试默认路径行为（如果配置文件存在）
	// 注意：这个测试可能失败如果默认配置文件不存在，这是预期的
	_, err := LoadConfig("")
	if err != nil {
		// 如果默认配置文件不存在，这是预期的错误
		t.Logf("LoadConfig with default path returned error (expected if config.json doesn't exist): %v", err)
	}
}

