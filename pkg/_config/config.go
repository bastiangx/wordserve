package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds all application configuration
type Config struct {
	App        AppConfig        `json:"app"`
	Completion CompletionConfig `json:"completion"`
	Fuzzy      FuzzyConfig      `json:"fuzzy"`
	Server     ServerConfig     `json:"server"`
	Dictionary DictionaryConfig `json:"dictionary"`
}

type AppConfig struct {
	LogLevel    string `json:"log_level"`
	Environment string `json:"environment"` // dev, prod, test
	DataDir     string `json:"data_dir"`
}

type CompletionConfig struct {
	MinFrequencyThreshold int  `json:"min_frequency_threshold"`
	ShortWordThreshold    int  `json:"short_word_threshold"`
	MaxPrefixLength       int  `json:"max_prefix_length"`
	DefaultLimit          int  `json:"default_limit"`
	EnableFrequencyBoost  bool `json:"enable_frequency_boost"`
}

type FuzzyConfig struct {
	Enabled               bool `json:"enabled"`
	MaxEditDistance       int  `json:"max_edit_distance"`
	MinWordLength         int  `json:"min_word_length"`
	UseFirstCharHeuristic bool `json:"use_first_char_heuristic"`
}

type ServerConfig struct {
	Mode           string `json:"mode"` // ipc, http, tcp
	Port           int    `json:"port,omitempty"`
	ReadTimeout    int    `json:"read_timeout"`
	WriteTimeout   int    `json:"write_timeout"`
	MaxRequestSize int    `json:"max_request_size"`
}

type DictionaryConfig struct {
	BinaryDir    string   `json:"binary_dir"`
	TextFiles    []string `json:"text_files"`
	AutoLoad     bool     `json:"auto_load"`
	CacheEnabled bool     `json:"cache_enabled"`
	Languages    []string `json:"languages"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		App: AppConfig{
			LogLevel:    "info",
			Environment: "prod",
			DataDir:     "./data",
		},
		Completion: CompletionConfig{
			MinFrequencyThreshold: 40,
			ShortWordThreshold:    60,
			MaxPrefixLength:       60,
			DefaultLimit:          10,
			EnableFrequencyBoost:  true,
		},
		Fuzzy: FuzzyConfig{
			Enabled:               true,
			MaxEditDistance:       2,
			MinWordLength:         3,
			UseFirstCharHeuristic: true,
		},
		Server: ServerConfig{
			Mode:           "ipc",
			ReadTimeout:    30,
			WriteTimeout:   30,
			MaxRequestSize: 1024,
		},
		Dictionary: DictionaryConfig{
			BinaryDir:    "./data",
			AutoLoad:     true,
			CacheEnabled: true,
			Languages:    []string{"en"},
		},
	}
}

// LoadConfig loads configuration from file with environment overrides
func LoadConfig(configPath string) (*Config, error) {
	cfg := DefaultConfig()

	// Load from file if it exists
	if configPath != "" {
		if err := cfg.loadFromFile(configPath); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}

	// Apply environment overrides
	cfg.applyEnvOverrides()

	return cfg, nil
}

func (c *Config) loadFromFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	return decoder.Decode(c)
}

func (c *Config) applyEnvOverrides() {
	if env := os.Getenv("TYPR_LOG_LEVEL"); env != "" {
		c.App.LogLevel = env
	}
	if env := os.Getenv("TYPR_ENVIRONMENT"); env != "" {
		c.App.Environment = env
	}
	if env := os.Getenv("TYPR_FUZZY_ENABLED"); env == "false" {
		c.Fuzzy.Enabled = false
	}
	if env := os.Getenv("TYPR_BINARY_DIR"); env != "" {
		c.Dictionary.BinaryDir = env
	}
}

// SaveConfig saves the current config to a file
func (c *Config) SaveConfig(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(c)
}
