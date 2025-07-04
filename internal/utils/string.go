package utils

import (
	"fmt"
	"strings"
)

// CapitalInfo holds basic info on pos and chars of capital letters in a string
type CapitalInfo struct {
	positions []int
	chars     []rune
}

// GetCapitalDetails extracts capital letter positions and characters from a string.
// Returns the lowercase version and cap info since we need to apply it later.
// lowercase return is because of actual dictionary words being all lowercase at this point.
func GetCapitalDetails(s string) (string, *CapitalInfo) {
	var info *CapitalInfo
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
	info = &CapitalInfo{
		positions: make([]int, 0, 4),
		chars:     make([]rune, 0, 4),
	}
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			info.positions = append(info.positions, i)
			info.chars = append(info.chars, r)
		}
	}
	return strings.ToLower(s), info
}

// CapitalizeAtPositions applies capitalization info to a word
// Works by replacing characters at specified positions with the corresponding capital letters.
// If info is nil or has no positions, returns the original word.
func CapitalizeAtPositions(word string, info *CapitalInfo) string {
	if info == nil || len(info.positions) == 0 {
		return word // No allocation needed
	}

	runes := []rune(word)
	for i, pos := range info.positions {
		if pos < len(runes) {
			runes[pos] = info.chars[i]
		}
	}
	return string(runes)
}

// CapitalizeWords applies capitalization simply to a slice of words.
func CapitalizeWords(words []string, info *CapitalInfo) {
	if info == nil || len(info.positions) == 0 {
		return
	}
	for i, word := range words {
		words[i] = CapitalizeAtPositions(word, info)
	}
}

// FormatWithCommas formats an integer with comma separators
func FormatWithCommas(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	str := fmt.Sprintf("%d", n)
	result := ""
	for i, char := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result += ","
		}
		result += string(char)
	}
	return result
}
