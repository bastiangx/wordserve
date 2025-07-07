package utils

import (
	"os"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/log"
)

// LoadTOMLFile loads and parses a TOML file into the provided struct
func LoadTOMLFile(configPath string, config interface{}) error {
	if _, err := toml.DecodeFile(configPath, config); err != nil {
		log.Warnf("TOML parsing error in config file %s: %v. Attempting partial recovery...", configPath, err)
		return err
	}
	return nil
}

// ParseTOMLWithRecovery attempts to parse a TOML file with partial recovery
func ParseTOMLWithRecovery(configPath string) (map[string]any, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	tempConfig := make(map[string]any)
	if _, err := toml.Decode(string(data), &tempConfig); err != nil {
		log.Warnf("Could not parse any valid configuration from %s: %v", configPath, err)
		return nil, err
	}
	return tempConfig, nil
}

// ExtractSection extracts a specific section from parsed TOML data
func ExtractSection(data map[string]any, sectionName string) (map[string]any, bool) {
	section, ok := data[sectionName].(map[string]any)
	return section, ok
}

// ExtractInt64 safely extracts an int64 value from a map
func ExtractInt64(data map[string]any, key string) (int, bool) {
	if val, ok := data[key].(int64); ok {
		return int(val), true
	}
	return 0, false
}

// ExtractBool safely extracts a bool value from a map
func ExtractBool(data map[string]any, key string) (bool, bool) {
	if val, ok := data[key].(bool); ok {
		return val, true
	}
	return false, false
}
