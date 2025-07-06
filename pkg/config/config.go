/*
Package config manages TOML config for WordServe services.

InitConfig handles automatic config file creation and loading with fallback to defaults.
LoadConfig and SaveConfig provide direct fs for runtime changes.
Update allows targeted parameter changes with persistence.
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
		return nil, err
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		config := DefaultConfig()
		if err := SaveConfig(config, configPath); err != nil {
			return nil, err
		}
		log.Debugf("Created default config file at: ( %s )", configPath)
		return config, nil
	}
	config, err := LoadConfig(configPath)
	if err != nil {
		log.Warnf("Failed to load config, using defaults: %v", err)
		return DefaultConfig(), nil
	}
	return config, nil
}

// LoadConfig loads from a TOML file
func LoadConfig(configPath string) (*Config, error) {
	var config Config
	if _, err := toml.DecodeFile(configPath, &config); err != nil {
		log.Errorf("Failed to decode config file: %v", err)
		return nil, err
	}
	return &config, nil
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
