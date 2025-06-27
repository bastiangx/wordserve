package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/log"
)

// PathResolver provides robust path resolution for the typer binary
type PathResolver struct {
	executablePath string
	executableDir  string
	homeDir        string
	configDir      string
}

// NewPathResolver creates a new path resolver that determines the executable location
func NewPathResolver() (*PathResolver, error) {
	// Get the path of the currently running executable
	execPath, err := os.Executable()
	if err != nil {
		return nil, err
	}

	// Resolve any symlinks to get the actual binary location
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return nil, err
	}

	execDir := filepath.Dir(execPath)

	// Get user home directory for config files
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Warnf("Could not determine home directory: %v", err)
		homeDir = "/tmp" // fallback
	}

	// Determine config directory (platform-specific)
	configDir := getConfigDir(homeDir)

	pr := &PathResolver{
		executablePath: execPath,
		executableDir:  execDir,
		homeDir:        homeDir,
		configDir:      configDir,
	}

	log.Debugf("PathResolver initialized: exec=%s, execDir=%s, configDir=%s",
		execPath, execDir, configDir)

	return pr, nil
}

// getConfigDir returns the appropriate config directory for the platform
func getConfigDir(homeDir string) string {
	switch runtime.GOOS {
	case "darwin": // macOS
		return filepath.Join(homeDir, ".config", "typer")
	case "linux":
		if configHome := os.Getenv("XDG_CONFIG_HOME"); configHome != "" {
			return filepath.Join(configHome, "typer")
		}
		return filepath.Join(homeDir, ".config", "typer")
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "typer")
		}
		return filepath.Join(homeDir, "AppData", "Roaming", "typer")
	default:
		return filepath.Join(homeDir, ".typer")
	}
}

// GetDataDir resolves the data directory containing binary chunk files
// It tries multiple locations in order of preference:
// 1. User-specified path (if absolute)
// 2. Relative to executable directory
// 3. Relative to current working directory (fallback)
func (pr *PathResolver) GetDataDir(userSpecifiedPath string) (string, error) {
	var candidatePaths []string

	// If user specified an absolute path, use it first
	if filepath.IsAbs(userSpecifiedPath) {
		candidatePaths = append(candidatePaths, userSpecifiedPath)
	}

	// Try relative to executable directory (most robust)
	execRelativePath := filepath.Join(pr.executableDir, userSpecifiedPath)
	candidatePaths = append(candidatePaths, execRelativePath)

	// Try relative to current working directory (fallback for development)
	if cwd, err := os.Getwd(); err == nil {
		cwdRelativePath := filepath.Join(cwd, userSpecifiedPath)
		candidatePaths = append(candidatePaths, cwdRelativePath)
	}

	// Try some common alternative locations
	commonPaths := []string{
		filepath.Join(pr.executableDir, "data"),
		filepath.Join(filepath.Dir(pr.executableDir), "data"), // parent/data
		filepath.Join(pr.configDir, "data"),                   // config/data
	}
	candidatePaths = append(candidatePaths, commonPaths...)

	// Test each candidate path
	for _, path := range candidatePaths {
		if pr.isValidDataDir(path) {
			log.Debugf("Found valid data directory: %s", path)
			return path, nil
		}
		log.Debugf("Data directory candidate not valid: %s", path)
	}

	// If nothing found, return the most likely path for error reporting
	return execRelativePath, nil
}

// isValidDataDir checks if a directory contains the expected binary chunk files
func (pr *PathResolver) isValidDataDir(path string) bool {
	// Check if directory exists
	if stat, err := os.Stat(path); err != nil || !stat.IsDir() {
		return false
	}

	// Look for dict_*.bin files
	pattern := filepath.Join(path, "dict_*.bin")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return false
	}

	// Must have at least one chunk file
	return len(matches) > 0
}

// GetConfigPath returns the full path for a config file
// It ensures the config directory exists and handles read-only filesystem issues
func (pr *PathResolver) GetConfigPath(filename string) (string, error) {
	// Try config directory first (preferred)
	configPath := filepath.Join(pr.configDir, filename)
	if pr.ensureConfigDir(pr.configDir) {
		return configPath, nil
	}

	// Fallback locations if config dir is not writable
	fallbackDirs := []string{
		filepath.Join(pr.homeDir, ".typer"),  // ~/.typer/
		filepath.Join(os.TempDir(), "typer"), // /tmp/typer/
		pr.executableDir,                     // same dir as executable
	}

	for _, dir := range fallbackDirs {
		if pr.ensureConfigDir(dir) {
			path := filepath.Join(dir, filename)
			log.Warnf("Using fallback config location: %s", path)
			return path, nil
		}
	}

	// Last resort: return temp file path
	tempPath := filepath.Join(os.TempDir(), filename)
	log.Warnf("Using temporary config file: %s", tempPath)
	return tempPath, nil
}

// ensureConfigDir creates the directory if it doesn't exist and tests writability
func (pr *PathResolver) ensureConfigDir(dir string) bool {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Debugf("Cannot create config directory %s: %v", dir, err)
		return false
	}

	// Test if directory is writable
	testFile := filepath.Join(dir, ".write_test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		log.Debugf("Config directory %s is not writable: %v", dir, err)
		return false
	}

	// Clean up test file
	os.Remove(testFile)
	return true
}

// GetExecutableDir returns the directory containing the executable
func (pr *PathResolver) GetExecutableDir() string {
	return pr.executableDir
}

// GetExecutablePath returns the full path to the executable
func (pr *PathResolver) GetExecutablePath() string {
	return pr.executablePath
}

// GetConfigDir returns the config directory
func (pr *PathResolver) GetConfigDir() string {
	return pr.configDir
}

// ResolveRelativePath resolves a path relative to the executable directory
func (pr *PathResolver) ResolveRelativePath(relativePath string) string {
	if filepath.IsAbs(relativePath) {
		return relativePath
	}
	return filepath.Join(pr.executableDir, relativePath)
}

// FindFileInPaths searches for a file in multiple possible locations
func (pr *PathResolver) FindFileInPaths(filename string, searchPaths []string) (string, error) {
	for _, searchPath := range searchPaths {
		fullPath := filepath.Join(searchPath, filename)
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath, nil
		}
	}

	return "", os.ErrNotExist
}

// GetRuntimeInfo returns debug information about the current runtime environment
func (pr *PathResolver) GetRuntimeInfo() map[string]string {
	cwd, _ := os.Getwd()

	info := map[string]string{
		"executable_path": pr.executablePath,
		"executable_dir":  pr.executableDir,
		"current_dir":     cwd,
		"home_dir":        pr.homeDir,
		"config_dir":      pr.configDir,
		"os":              runtime.GOOS,
		"arch":            runtime.GOARCH,
	}

	// Add environment variables that might be relevant
	envVars := []string{"PWD", "HOME", "XDG_CONFIG_HOME", "APPDATA", "PATH"}
	for _, envVar := range envVars {
		if value := os.Getenv(envVar); value != "" {
			info["env_"+strings.ToLower(envVar)] = value
		}
	}

	return info
}

// DiagnosePathIssues provides detailed diagnostics for path resolution problems
func (pr *PathResolver) DiagnosePathIssues(userDataPath string) map[string]interface{} {
	diag := make(map[string]interface{})

	// Basic runtime info
	diag["runtime_info"] = pr.GetRuntimeInfo()

	// Test data directory resolution
	dataDir, err := pr.GetDataDir(userDataPath)
	diag["data_dir_resolution"] = map[string]interface{}{
		"requested_path": userDataPath,
		"resolved_path":  dataDir,
		"error":          err,
		"exists":         pr.pathExists(dataDir),
		"is_valid":       pr.isValidDataDir(dataDir),
	}

	// Test all candidate data paths
	candidates := pr.getDataDirCandidates(userDataPath)
	candidateTests := make([]map[string]interface{}, 0, len(candidates))
	for _, candidate := range candidates {
		candidateTests = append(candidateTests, map[string]interface{}{
			"path":     candidate,
			"exists":   pr.pathExists(candidate),
			"is_dir":   pr.isDirectory(candidate),
			"is_valid": pr.isValidDataDir(candidate),
			"files":    pr.listBinFiles(candidate),
		})
	}
	diag["data_dir_candidates"] = candidateTests

	// Test config path resolution
	configPath, err := pr.GetConfigPath("typer-config.toml")
	diag["config_path_resolution"] = map[string]interface{}{
		"resolved_path": configPath,
		"error":         err,
		"dir_exists":    pr.pathExists(filepath.Dir(configPath)),
		"dir_writable":  pr.ensureConfigDir(filepath.Dir(configPath)),
	}

	return diag
}

// Helper functions for diagnostics
func (pr *PathResolver) pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (pr *PathResolver) isDirectory(path string) bool {
	stat, err := os.Stat(path)
	return err == nil && stat.IsDir()
}

func (pr *PathResolver) listBinFiles(path string) []string {
	pattern := filepath.Join(path, "dict_*.bin")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return []string{}
	}
	return matches
}

func (pr *PathResolver) getDataDirCandidates(userSpecifiedPath string) []string {
	var candidates []string

	if filepath.IsAbs(userSpecifiedPath) {
		candidates = append(candidates, userSpecifiedPath)
	}

	execRelativePath := filepath.Join(pr.executableDir, userSpecifiedPath)
	candidates = append(candidates, execRelativePath)

	if cwd, err := os.Getwd(); err == nil {
		cwdRelativePath := filepath.Join(cwd, userSpecifiedPath)
		candidates = append(candidates, cwdRelativePath)
	}

	commonPaths := []string{
		filepath.Join(pr.executableDir, "data"),
		filepath.Join(filepath.Dir(pr.executableDir), "data"),
		filepath.Join(pr.configDir, "data"),
	}
	candidates = append(candidates, commonPaths...)

	return candidates
}
