package dictionary

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"
)

// FileFormat represents different dictionary file formats
type FileFormat int

const (
	FormatUnknown FileFormat = iota
	FormatTrie                // Binary trie format
	FormatChunk              // Chunked binary format
	FormatText               // Plain text format
)

// FormatInfo contains metadata about a dictionary file format
type FormatInfo struct {
	Format      FileFormat
	Description string
	Extensions  []string
	MinSize     int64 // Minimum expected file size in bytes
}

var supportedFormats = map[FileFormat]FormatInfo{
	FormatTrie: {
		Format:      FormatTrie,
		Description: "Binary Trie Dictionary",
		Extensions:  []string{".bin"},
		MinSize:     8, // At least header + one entry
	},
	FormatChunk: {
		Format:      FormatChunk,
		Description: "Chunked Binary Dictionary",
		Extensions:  []string{".bin"},
		MinSize:     4, // At least word count header
	},
	FormatText: {
		Format:      FormatText,
		Description: "Plain Text Dictionary",
		Extensions:  []string{".txt"},
		MinSize:     1, // At least one character
	},
}

// ValidateFileFormat checks if a file matches the expected format
func ValidateFileFormat(filename string, expectedFormat FileFormat) error {
	fileInfo, err := os.Stat(filename)
	if err != nil {
		return fmt.Errorf("failed to stat file %s: %w", filename, err)
	}

	formatInfo, exists := supportedFormats[expectedFormat]
	if !exists {
		return fmt.Errorf("unknown format: %v", expectedFormat)
	}

	// Check file size
	if fileInfo.Size() < formatInfo.MinSize {
		return fmt.Errorf("file %s is too small (%d bytes) for format %s (minimum: %d bytes)",
			filename, fileInfo.Size(), formatInfo.Description, formatInfo.MinSize)
	}

	// Check file extension
	ext := strings.ToLower(filepath.Ext(filename))
	validExt := false
	for _, validExtension := range formatInfo.Extensions {
		if ext == validExtension {
			validExt = true
			break
		}
	}
	if !validExt {
		return fmt.Errorf("file %s has invalid extension %s for format %s (expected: %v)",
			filename, ext, formatInfo.Description, formatInfo.Extensions)
	}

	// Format-specific validation
	switch expectedFormat {
	case FormatTrie, FormatChunk:
		return validateBinaryFormat(filename)
	case FormatText:
		return validateTextFormat(filename)
	}

	return nil
}

// validateBinaryFormat validates binary dictionary files
func validateBinaryFormat(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", filename, err)
	}
	defer file.Close()

	// Check if we can read the header (word count)
	var wordCount int32
	if err := binary.Read(file, binary.LittleEndian, &wordCount); err != nil {
		return fmt.Errorf("failed to read header from %s: %w", filename, err)
	}

	// Validate word count is reasonable
	if wordCount < 0 {
		return fmt.Errorf("invalid word count in %s: %d (negative)", filename, wordCount)
	}

	if wordCount > 1000000 { // Sanity check: more than 1M words seems suspicious
		return fmt.Errorf("suspicious word count in %s: %d (too large)", filename, wordCount)
	}

	log.Debugf("Binary file %s validated: %d words", filename, wordCount)
	return nil
}

// validateTextFormat validates text dictionary files
func validateTextFormat(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", filename, err)
	}
	defer file.Close()

	// Just check if it's readable - more specific validation can be added later
	buffer := make([]byte, 1024)
	_, err = file.Read(buffer)
	if err != nil {
		return fmt.Errorf("failed to read from text file %s: %w", filename, err)
	}

	log.Debugf("Text file %s validated", filename)
	return nil
}

// DetectFileFormat attempts to detect the format of a file
func DetectFileFormat(filename string) (FileFormat, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	basename := strings.ToLower(filepath.Base(filename))

	// Check for chunk files by naming pattern
	if strings.HasPrefix(basename, "dict_") && ext == ".bin" {
		if err := ValidateFileFormat(filename, FormatChunk); err == nil {
			return FormatChunk, nil
		}
	}

	// Check for regular trie files
	if ext == ".bin" {
		if err := ValidateFileFormat(filename, FormatTrie); err == nil {
			return FormatTrie, nil
		}
	}

	// Check for text files
	if ext == ".txt" {
		if err := ValidateFileFormat(filename, FormatText); err == nil {
			return FormatText, nil
		}
	}

	return FormatUnknown, fmt.Errorf("unable to detect format for file %s", filename)
}

// GetFormatInfo returns information about a specific format
func GetFormatInfo(format FileFormat) (FormatInfo, bool) {
	info, exists := supportedFormats[format]
	return info, exists
}

// ListSupportedFormats returns all supported formats
func ListSupportedFormats() []FormatInfo {
	var formats []FormatInfo
	for _, info := range supportedFormats {
		formats = append(formats, info)
	}
	return formats
}
