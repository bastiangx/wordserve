/*
Package config manages TOML config for WordServe services.
*/
package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/log"
)

// Config holds the entire config structure
type Config struct {
	Server ServerConfig `toml:"server"`
	Dict   DictConfig   `toml:"dict"`
	CLI    CliConfig    `toml:"cli"`
}

// ServerConfig has server related options.
type ServerConfig struct {
	MaxLimit     int  `toml:"max_limit"`
	MinPrefix    int  `toml:"min_prefix"`
	MaxPrefix    int  `toml:"max_prefix"`
	EnableFilter bool `toml:"enable_filter"`
}

// DictConfig holds dictionary options.
type DictConfig struct {
	MaxWords               int `toml:"max_words"`
	ChunkSize              int `toml:"chunk_size"`
	MinFreqThreshold       int `toml:"min_frequency_threshold"`
	MinFreqShortPrefix     int `toml:"min_frequency_short_prefix"`
	MaxWordCountValidation int `toml:"max_word_count_validation"`
}

// CliConfig holds cli interface options.
type CliConfig struct {
	DefaultLimit    int  `toml:"default_limit"`
	DefaultMinLen   int  `toml:"default_min_len"`
	DefaultMaxLen   int  `toml:"default_max_len"`
	DefaultNoFilter bool `toml:"default_no_filter"`
}

// GetConfigDir returns the confif directory (std golang)
func GetConfigDir() (string, error) {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(userConfigDir, "wordserve"), nil
}

// GetDefaultConfigPath returns the default path for config.toml
func GetDefaultConfigPath() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "config.toml"), nil
}

// LoadConfigWithPriority loads config with priority:
// 1. Custom path from --config flag
// 2. Default path: [UserConfigDir]/wordserve/config.toml
// 3. Builtin defaults
func LoadConfigWithPriority(customConfigPath string) (*Config, string, error) {
	var config *Config
	var err error

	if customConfigPath != "" {
		if _, statErr := os.Stat(customConfigPath); statErr == nil {
			config, err = LoadConfig(customConfigPath)
			if err != nil {
				log.Warnf("Failed to load custom config from %s: %v. Trying default path...", customConfigPath, err)
			} else {
				log.Debugf("Loaded config from custom path: %s", customConfigPath)
				return config, customConfigPath, nil
			}
		} else {
			log.Warnf("Custom config file not found at %s: %v. Trying default path...", customConfigPath, statErr)
		}
	}
	defaultPath, err := GetDefaultConfigPath()
	if err != nil {
		log.Warnf("Failed to determine default config path: %v. Using built-in defaults...", err)
		return DefaultConfig(), "", nil
	}

	config, err = InitConfig(defaultPath)
	if err != nil {
		log.Warnf("Failed to load/create config at default path %s: %v. Using builtin defaults...", defaultPath, err)
		return DefaultConfig(), "", nil
	}
	log.Debugf("Loaded config from default path: %s", defaultPath)
	return config, defaultPath, nil
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			MaxLimit:     64,
			MinPrefix:    1,
			MaxPrefix:    60,
			EnableFilter: true,
		},
		Dict: DictConfig{
			MaxWords:               50000,
			ChunkSize:              10000,
			MinFreqThreshold:       20,
			MinFreqShortPrefix:     24,
			MaxWordCountValidation: 1000000,
		},
		CLI: CliConfig{
			DefaultLimit:    24,
			DefaultMinLen:   1,
			DefaultMaxLen:   24,
			DefaultNoFilter: false,
		},
	}
}

// InitConfig loads config from file or creates default if missing
func InitConfig(configPath string) (*Config, error) {
	configDir := filepath.Dir(configPath)

	if err := os.MkdirAll(configDir, 0755); err != nil {
		log.Warnf("Failed to create config directory %s: %v. Using built-in defaults...", configDir, err)
		return DefaultConfig(), nil
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config file
		config := DefaultConfig()
		if err := SaveConfig(config, configPath); err != nil {
			log.Warnf("Failed to create default config file at %s: %v. Using built-in defaults...", configPath, err)
			return DefaultConfig(), nil
		}
		log.Debugf("Created default config file at: %s", configPath)
		return config, nil
	}
	config, err := LoadConfig(configPath)
	if err != nil {
		log.Warnf("Failed to load config from %s: %v. Using built-in defaults...", configPath, err)
		return DefaultConfig(), nil
	}
	return config, nil
}

// LoadConfig loads from a TOML file
func LoadConfig(configPath string) (*Config, error) {
	config := DefaultConfig()

	if _, err := toml.DecodeFile(configPath, config); err != nil {
		log.Warnf("TOML parsing error in config file %s: %v. Attempting partial recovery...", configPath, err)
		return tryPartialParse(configPath)
	}
	return config, nil
}

// tryPartialParse attempts to parse a TOML file
func tryPartialParse(configPath string) (*Config, error) {
	config := DefaultConfig()
	// section by section
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	// Create a temp config to try parsing into
	tempConfig := make(map[string]any)
	if _, err := toml.Decode(string(data), &tempConfig); err != nil {
		log.Warnf("Could not parse any valid configuration from %s: %v. Using all defaults.", configPath, err)
		return config, nil
	}
	if serverSection, ok := tempConfig["server"].(map[string]any); ok {
		extractServerConfig(serverSection, &config.Server)
	}
	if dictSection, ok := tempConfig["dict"].(map[string]any); ok {
		extractDictConfig(dictSection, &config.Dict)
	}
	if cliSection, ok := tempConfig["cli"].(map[string]any); ok {
		extractCliConfig(cliSection, &config.CLI)
	}
	return config, nil
}

// extractServerConfig extracts server configuration from a map
func extractServerConfig(data map[string]any, server *ServerConfig) {
	if val, ok := data["max_limit"].(int64); ok {
		server.MaxLimit = int(val)
	}
	if val, ok := data["min_prefix"].(int64); ok {
		server.MinPrefix = int(val)
	}
	if val, ok := data["max_prefix"].(int64); ok {
		server.MaxPrefix = int(val)
	}
	if val, ok := data["enable_filter"].(bool); ok {
		server.EnableFilter = val
	}
}

// extractDictConfig extracts dictionary configuration from a map
func extractDictConfig(data map[string]any, dict *DictConfig) {
	if val, ok := data["max_words"].(int64); ok {
		dict.MaxWords = int(val)
	}
	if val, ok := data["chunk_size"].(int64); ok {
		dict.ChunkSize = int(val)
	}
	if val, ok := data["min_frequency_threshold"].(int64); ok {
		dict.MinFreqThreshold = int(val)
	}
	if val, ok := data["min_frequency_short_prefix"].(int64); ok {
		dict.MinFreqShortPrefix = int(val)
	}
	if val, ok := data["max_word_count_validation"].(int64); ok {
		dict.MaxWordCountValidation = int(val)
	}
}

// extractCliConfig extracts CLI config from a map
func extractCliConfig(data map[string]any, cli *CliConfig) {
	if val, ok := data["default_limit"].(int64); ok {
		cli.DefaultLimit = int(val)
	}
	if val, ok := data["default_min_len"].(int64); ok {
		cli.DefaultMinLen = int(val)
	}
	if val, ok := data["default_max_len"].(int64); ok {
		cli.DefaultMaxLen = int(val)
	}
	if val, ok := data["default_no_filter"].(bool); ok {
		cli.DefaultNoFilter = val
	}
}

// RebuildConfigFile force creates a new config.toml at default
func RebuildConfigFile() error {
	defaultPath, err := GetDefaultConfigPath()
	if err != nil {
		return err
	}
	configDir := filepath.Dir(defaultPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}
	config := DefaultConfig()
	return SaveConfig(config, defaultPath)
}

// GetActiveConfigPath returns the absolute path of loaded config file
func GetActiveConfigPath(configPath string) string {
	if configPath == "" {
		if defaultPath, err := GetDefaultConfigPath(); err == nil {
			return defaultPath
		}
		return "unknown"
	}

	// Convert if relative
	if !filepath.IsAbs(configPath) {
		if absPath, err := filepath.Abs(configPath); err == nil {
			return absPath
		}
	}
	return configPath
}

// SaveConfig saves into a TOML file
func SaveConfig(config *Config, configPath string) error {
	file, err := os.Create(configPath)
	if err != nil {
		log.Errorf("Failed to create config file: %v", err)
		return err
	}
	defer file.Close()
	encoder := toml.NewEncoder(file)
	return encoder.Encode(config)
}

// Update changes the config values and saves to file
func (c *Config) Update(configPath string, maxLimit, minPrefix, maxPrefix *int, enableFilter *bool) error {
	server := &c.Server
	if maxLimit != nil {
		server.MaxLimit = *maxLimit
	}
	if minPrefix != nil {
		server.MinPrefix = *minPrefix
	}
	if maxPrefix != nil {
		server.MaxPrefix = *maxPrefix
	}
	if enableFilter != nil {
		server.EnableFilter = *enableFilter
	}
	return SaveConfig(c, configPath)
}
