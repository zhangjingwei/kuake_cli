package sdk

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadConfig 从配置文件加载配置
// 如果 configPath 为空，使用默认路径 DEFAULT_CONFIG_PATH
func LoadConfig(configPath string) (*Config, error) {
	// 如果配置文件路径为空，使用默认路径
	if configPath == "" {
		configPath = DEFAULT_CONFIG_PATH
	}

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	// 解析 JSON
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// 验证必要的配置项
	if len(config.Quark.AccessTokens) == 0 {
		return nil, fmt.Errorf("access_tokens 必须至少配置一个")
	}

	return &config, nil
}

// SaveConfig 保存配置到文件
// 如果 configPath 为空，使用默认路径 DEFAULT_CONFIG_PATH
func SaveConfig(configPath string, config *Config) error {
	// 如果配置文件路径为空，使用默认路径
	if configPath == "" {
		configPath = DEFAULT_CONFIG_PATH
	}

	// 序列化为 JSON
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", configPath, err)
	}

	return nil
}
