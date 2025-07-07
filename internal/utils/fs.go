package utils

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/log"
)

// DirCheckResult represents the result of dir checks
type DirCheckResult struct {
	Exists   bool
	Writable bool
	Error    error
}

// FileExists simply checks if a file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// EnsureDir creates directory if it doesn't exist
func EnsureDir(dirPath string) error {
	return os.MkdirAll(dirPath, 0755)
}

// SaveTOMLFile saves a struct to a TOML file
func SaveTOMLFile(data interface{}, filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		log.Errorf("Failed to create file: %v", err)
		return err
	}
	defer file.Close()
	encoder := toml.NewEncoder(file)
	return encoder.Encode(data)
}

// GetAbsolutePath returns the absolute path of a file
func GetAbsolutePath(configPath string) string {
	if configPath == "" {
		return "unknown"
	}

	if !filepath.IsAbs(configPath) {
		if absPath, err := filepath.Abs(configPath); err == nil {
			return absPath
		}
	}
	return configPath
}

// testWriteAccess tests if a directory can be written to
func testWriteAccess(dirPath string) bool {
	testFile := filepath.Join(dirPath, ".write_test")
	file, err := os.Create(testFile)
	if err != nil {
		log.Warnf("Cannot write to directory %s: %v", dirPath, err)
		return false
	}
	file.Close()
	os.Remove(testFile)
	return true
}

// GetExecutableDir returns the directory of the current executable
// Its a fallback to os.Executable() which may not work in all environments
// If this doesn't work too, (configInit) will fallback to builtin defaults.
func GetExecutableDir() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(execPath), nil
}

// CheckDirStatus performs dir status check
// Tests if directory exists, can be created, and is writable
func CheckDirStatus(dirPath string) DirCheckResult {
	result := DirCheckResult{}
	if _, err := os.Stat(dirPath); err == nil {
		result.Exists = true
		result.Writable = testWriteAccess(dirPath)
		return result
	}
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		result.Error = err
		log.Warnf("Cannot create directory %s: %v", dirPath, err)
		return result
	}
	result.Exists = true
	result.Writable = testWriteAccess(dirPath)
	return result
}
