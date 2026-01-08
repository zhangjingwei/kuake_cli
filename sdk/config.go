package sdk

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// getExecutableDir 获取可执行文件所在的目录
func getExecutableDir() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	// 解析符号链接（如果有）
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve executable symlink: %w", err)
	}
	return filepath.Dir(execPath), nil
}

// resolveConfigPath 解析配置文件路径
// 如果是绝对路径，则直接使用
// 如果是相对路径，则相对于可执行文件所在目录
func resolveConfigPath(configPath string) (string, error) {
	// 如果是绝对路径，直接返回
	if filepath.IsAbs(configPath) {
		return configPath, nil
	}

	// 相对路径，需要获取可执行文件所在目录
	execDir, err := getExecutableDir()
	if err != nil {
		return "", fmt.Errorf("failed to get executable directory: %w", err)
	}

	// 拼接路径
	return filepath.Join(execDir, configPath), nil
}

// LoadConfig 从配置文件加载配置
// 如果 configPath 为空，使用默认路径 DEFAULT_CONFIG_PATH
// 相对路径会相对于可执行文件所在目录解析
func LoadConfig(configPath string) (*Config, error) {
	// 如果配置文件路径为空，使用默认路径
	if configPath == "" {
		configPath = DEFAULT_CONFIG_PATH
	}

	// 解析配置文件路径（相对路径相对于可执行文件所在目录）
	resolvedPath, err := resolveConfigPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve config path: %w", err)
	}

	// 读取配置文件
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", resolvedPath, err)
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
// 相对路径会相对于可执行文件所在目录解析
func SaveConfig(configPath string, config *Config) error {
	// 如果配置文件路径为空，使用默认路径
	if configPath == "" {
		configPath = DEFAULT_CONFIG_PATH
	}

	// 解析配置文件路径（相对路径相对于可执行文件所在目录）
	resolvedPath, err := resolveConfigPath(configPath)
	if err != nil {
		return fmt.Errorf("failed to resolve config path: %w", err)
	}

	// 序列化为 JSON
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(resolvedPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", resolvedPath, err)
	}

	return nil
}
