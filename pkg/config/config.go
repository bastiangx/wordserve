package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/log"
)

// Config holds all application configuration - central point for constants
type Config struct {
	Server ServerConfig `toml:"server"`
	Dict   DictConfig   `toml:"dict"`
	CLI    CLIConfig    `toml:"cli"`
}

type ServerConfig struct {
	MaxLimit     int  `toml:"max_limit"`
	MinPrefix    int  `toml:"min_prefix"`
	MaxPrefix    int  `toml:"max_prefix"`
	EnableFilter bool `toml:"enable_filter"`
}

type DictConfig struct {
	MaxWords             int `toml:"max_words"`
	ChunkSize            int `toml:"chunk_size"`
	MaxHotWords          int `toml:"max_hot_words"`
	MinFreqThreshold     int `toml:"min_frequency_threshold"`
	MinFreqShortPrefix   int `toml:"min_frequency_short_prefix"`
	MaxWordCountValidation int `toml:"max_word_count_validation"`
}

type CLIConfig struct {
	DefaultLimit    int `toml:"default_limit"`
	DefaultMinLen   int `toml:"default_min_len"`
	DefaultMaxLen   int `toml:"default_max_len"`
	DefaultNoFilter bool `toml:"default_no_filter"`
}

// Default values - central place for all constants
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			MaxLimit:     50,
			MinPrefix:    1,
			MaxPrefix:    60,
			EnableFilter: true,
		},
		Dict: DictConfig{
			MaxWords:               50000,  // from main.go --words default
			ChunkSize:              10000,  // from main.go --chunk default
			MaxHotWords:            20000,  // from completion.go NewLazyCompleter
			MinFreqThreshold:       20,     // from completion.go minFrequencyThreshold
			MinFreqShortPrefix:     24,     // from completion.go short prefix threshold
			MaxWordCountValidation: 1000000, // from formats.go binary validation
		},
		CLI: CLIConfig{
			DefaultLimit:    24,
			DefaultMinLen:   1,
			DefaultMaxLen:   24,
			DefaultNoFilter: false,
		},
	}
}

// LoadOrCreate loads config from file or creates default if missing
func LoadOrCreate(configPath string) (*Config, error) {
	// Create config directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config file
		config := DefaultConfig()
		if err := SaveConfig(config, configPath); err != nil {
			return nil, err
		}
		log.Debugf("Created default config file: %s", configPath)
		return config, nil
	}

	// Load existing config
	config, err := LoadConfig(configPath)
	if err != nil {
		log.Warnf("Failed to load config, using defaults: %v", err)
		return DefaultConfig(), nil
	}

	log.Debugf("Loaded config from: %s", configPath)
	return config, nil
}

// LoadConfig loads configuration from TOML file
func LoadConfig(configPath string) (*Config, error) {
	var config Config
	if _, err := toml.DecodeFile(configPath, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// SaveConfig saves configuration to TOML file  
func SaveConfig(config *Config, configPath string) error {
	file, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := toml.NewEncoder(file)
	return encoder.Encode(config)
}

// UpdateConfig updates config values and saves to file
func (c *Config) Update(configPath string, maxLimit, minPrefix, maxPrefix *int, enableFilter *bool) error {
	// Update values if provided
	if maxLimit != nil {
		c.Server.MaxLimit = *maxLimit
	}
	if minPrefix != nil {
		c.Server.MinPrefix = *minPrefix
	}
	if maxPrefix != nil {
		c.Server.MaxPrefix = *maxPrefix
	}
	if enableFilter != nil {
		c.Server.EnableFilter = *enableFilter
	}

	// Save to file
	return SaveConfig(c, configPath)
}
