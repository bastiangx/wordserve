package dictionary

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bastiangx/typr-lib/pkg/config"
	"github.com/charmbracelet/log"
)

// FileFormat shows file format types for dictionaries
type FileFormat int

const (
	FormatUnknown FileFormat = iota
	FormatBinary
	FormatText
)

// FormatInfo has the metadata for each file format
type FormatInfo struct {
	Format      FileFormat
	Description string
	Extensions  []string
	MinSize     int64
}

var supportedFormats = map[FileFormat]FormatInfo{
	FormatBinary: {
		Format:      FormatBinary,
		Description: "Binary Dictionary",
		Extensions:  []string{".bin"},
		MinSize:     4, // At least word count header
	},
	FormatText: {
		Format:      FormatText,
		Description: "Plain Text Dictionary",
		Extensions:  []string{".txt"},
		MinSize:     1, // At least one char
	},
}

// ValidateFileFormat checks if a file matches our expected format
func ValidateFileFormat(filename string, expectedFormat FileFormat) error {
	fileInfo, err := os.Stat(filename)
	if err != nil {
		log.Errorf("failed to stat file %s: %v", filename, err)
		return err
	}
	formatInfo, exists := supportedFormats[expectedFormat]
	if !exists {
		log.Errorf("unknown format: %v", expectedFormat)
		return errors.New("unknown format")
	}
	// size
	if fileInfo.Size() < formatInfo.MinSize {
		log.Errorf("file %s is too small (%d bytes) for format %s (minimum: %d bytes)",
			filename, fileInfo.Size(), formatInfo.Description, formatInfo.MinSize)
		return errors.New("file too small")
	}
	// extension
	ext := strings.ToLower(filepath.Ext(filename))
	if !slices.Contains(formatInfo.Extensions, ext) {
		log.Errorf("file %s has invalid extension %s for format %s (expected: %v)",
			filename, ext, formatInfo.Description, formatInfo.Extensions)
		return errors.New("invalid file extension")
	}
	switch expectedFormat {
	case FormatBinary:
		return validateBinaryFormat(filename)
	case FormatText:
		return validateTextFormat(filename)
	}
	return nil
}

// validateBinaryFormat checks if binary files are in the expected format
func validateBinaryFormat(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		log.Errorf("failed to open file %s: %v", filename, err)
		return err
	}
	defer file.Close()

	// check if we can read the header (word count)
	var wordCount int32
	if err := binary.Read(file, binary.LittleEndian, &wordCount); err != nil {
		log.Errorf("failed to read header from %s: %v", filename, err)
		return err
	}

	// Validate word count is reasonable
	if wordCount < 0 {
		log.Errorf("invalid word count in %s: %d (negative)", filename, wordCount)
		return errors.New("invalid word count")
	}
	cfg := config.DefaultConfig()
	if wordCount > int32(cfg.Dict.MaxWordCountValidation) {
		log.Errorf("questionable word count in %s: %d (too large, max: %d)", filename, wordCount, cfg.Dict.MaxWordCountValidation)
		return errors.New("word count too large")
	}
	log.Debugf("Binary file %s validated: %d words", filename, wordCount)
	return nil
}

// validateTextFormat confirms text dictionary files
func validateTextFormat(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		log.Errorf("failed to open file %s: %v", filename, err)
		return err
	}
	defer file.Close()

	// Just checks if it's readable
	// TODO: more and specific validation can be added later
	buffer := make([]byte, 1024)
	_, err = file.Read(buffer)
	if err != nil {
		log.Errorf("failed to read from text file %s: %v", filename, err)
		return err
	}

	log.Debugf("Text file %s validated", filename)
	return nil
}

// DetectFileFormat attempts to detect the format of a file
func DetectFileFormat(filename string) (FileFormat, error) {
	ext := strings.ToLower(filepath.Ext(filename))

	if ext == ".bin" {
		if err := ValidateFileFormat(filename, FormatBinary); err == nil {
			return FormatBinary, nil
		}
	}
	if ext == ".txt" {
		if err := ValidateFileFormat(filename, FormatText); err == nil {
			return FormatText, nil
		}
	}
	return FormatUnknown, func() error {
		log.Errorf("unable to detect format for file %s", filename)
		return errors.New("unable to detect format")
	}()
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
