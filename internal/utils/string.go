package utils

import (
	"strings"
)

// CapitalInfo holds information about capitalization in a string
type CapitalInfo struct {
	positions []int
	chars     []rune
}

// ExtractCapitalInfo extracts capital letter information from a string
// Returns the lowercase version and capitalization info
func ExtractCapitalInfo(s string) (string, *CapitalInfo) {
	var info *CapitalInfo

	// First pass: check if there are any capitals to avoid allocation if not needed
	hasCapitals := false
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			hasCapitals = true
			break
		}
	}

	if !hasCapitals {
		return strings.ToLower(s), nil
	}

	// Only allocate if we have capitals
	info = &CapitalInfo{
		positions: make([]int, 0, 4),
		chars:     make([]rune, 0, 4),
	}

	// Extract capital positions and characters
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			info.positions = append(info.positions, i)
			info.chars = append(info.chars, r)
		}
	}

	return strings.ToLower(s), info
}

// ApplyCapitalization applies capitalization info to a word
func ApplyCapitalization(word string, info *CapitalInfo) string {
	if info == nil || len(info.positions) == 0 {
		return word
	}

	runes := []rune(word)

	// Apply capitals only if the position exists in the word
	for i, pos := range info.positions {
		if pos < len(runes) {
			runes[pos] = info.chars[i]
		}
	}

	return string(runes)
}

// ApplyCapitalizationToSuggestions applies capitalization to a slice of suggestions
func ApplyCapitalizationToSuggestions(suggestions []string, info *CapitalInfo) []string {
	if info == nil || len(info.positions) == 0 {
		return suggestions
	}

	result := make([]string, len(suggestions))
	for i, suggestion := range suggestions {
		result[i] = ApplyCapitalization(suggestion, info)
	}

	return result
}
