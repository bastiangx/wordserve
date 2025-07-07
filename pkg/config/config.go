/*
Package config manages TOML config for WordServe services.
*/
package config

import (
	"os"
	"path/filepath"

	"github.com/bastiangx/wordserve/internal/utils"
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

// GetConfigDir returns the config directory with fallback priority:
// 1. ~/.config/
// 2. ~/Library/Application Support/ (macOS)
// 3. Current executable dir
// 4. builtin defaults
func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Errorf("Failed to get home directory: %v", err)
		execDir, execErr := utils.GetExecutableDir()
		if execErr != nil {
			return "", execErr
		}
		return execDir, nil
	}
	primaryPath := filepath.Join(homeDir, ".config", "wordserve")
	if result := utils.CheckDirStatus(primaryPath); result.Writable {
		return primaryPath, nil
	}
	// Not conventional, fallback from ~/.config if not writable
	macOSPath := filepath.Join(homeDir, "Library", "Application Support", "wordserve")
	if result := utils.CheckDirStatus(macOSPath); result.Writable {
		return macOSPath, nil
	}
	execDir, err := utils.GetExecutableDir()
	if err != nil {
		log.Errorf("Failed to get executable directory: %v", err)
		return "", err
	}
	return execDir, nil
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

	if err := utils.EnsureDir(configDir); err != nil {
		log.Warnf("Failed to create config directory %s: %v. Using built-in defaults...", configDir, err)
		return DefaultConfig(), nil
	}

	if !utils.FileExists(configPath) {
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

	if err := utils.LoadTOMLFile(configPath, config); err != nil {
		return tryPartialParse(configPath)
	}
	return config, nil
}

// tryPartialParse attempts to parse a TOML file
func tryPartialParse(configPath string) (*Config, error) {
	config := DefaultConfig()

	tempConfig, err := utils.ParseTOMLWithRecovery(configPath)
	if err != nil {
		log.Warnf("Could not parse any valid configuration from %s: %v. Using all defaults.", configPath, err)
		return config, nil
	}

	if serverSection, ok := utils.ExtractSection(tempConfig, "server"); ok {
		extractServerConfig(serverSection, &config.Server)
	}
	if dictSection, ok := utils.ExtractSection(tempConfig, "dict"); ok {
		extractDictConfig(dictSection, &config.Dict)
	}
	if cliSection, ok := utils.ExtractSection(tempConfig, "cli"); ok {
		extractCliConfig(cliSection, &config.CLI)
	}
	return config, nil
}

// extractServerConfig extracts server configuration from a map
func extractServerConfig(data map[string]any, server *ServerConfig) {
	if val, ok := utils.ExtractInt64(data, "max_limit"); ok {
		server.MaxLimit = val
	}
	if val, ok := utils.ExtractInt64(data, "min_prefix"); ok {
		server.MinPrefix = val
	}
	if val, ok := utils.ExtractInt64(data, "max_prefix"); ok {
		server.MaxPrefix = val
	}
	if val, ok := utils.ExtractBool(data, "enable_filter"); ok {
		server.EnableFilter = val
	}
}

// extractDictConfig extracts dictionary configuration from a map
func extractDictConfig(data map[string]any, dict *DictConfig) {
	if val, ok := utils.ExtractInt64(data, "max_words"); ok {
		dict.MaxWords = val
	}
	if val, ok := utils.ExtractInt64(data, "chunk_size"); ok {
		dict.ChunkSize = val
	}
	if val, ok := utils.ExtractInt64(data, "min_frequency_threshold"); ok {
		dict.MinFreqThreshold = val
	}
	if val, ok := utils.ExtractInt64(data, "min_frequency_short_prefix"); ok {
		dict.MinFreqShortPrefix = val
	}
	if val, ok := utils.ExtractInt64(data, "max_word_count_validation"); ok {
		dict.MaxWordCountValidation = val
	}
}

// extractCliConfig extracts CLI config from a map
func extractCliConfig(data map[string]any, cli *CliConfig) {
	if val, ok := utils.ExtractInt64(data, "default_limit"); ok {
		cli.DefaultLimit = val
	}
	if val, ok := utils.ExtractInt64(data, "default_min_len"); ok {
		cli.DefaultMinLen = val
	}
	if val, ok := utils.ExtractInt64(data, "default_max_len"); ok {
		cli.DefaultMaxLen = val
	}
	if val, ok := utils.ExtractBool(data, "default_no_filter"); ok {
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
	if err := utils.EnsureDir(configDir); err != nil {
		return err
	}
	config := DefaultConfig()
	return utils.SaveTOMLFile(config, defaultPath)
}

// GetActiveConfigPath returns the absolute path of loaded config file
func GetActiveConfigPath(configPath string) string {
	if configPath == "" {
		if defaultPath, err := GetDefaultConfigPath(); err == nil {
			return defaultPath
		}
		return "unknown"
	}
	return utils.GetAbsolutePath(configPath)
}

// SaveConfig saves into a TOML file
func SaveConfig(config *Config, configPath string) error {
	return utils.SaveTOMLFile(config, configPath)
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
