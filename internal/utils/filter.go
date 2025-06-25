package utils

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// IsSeparator checks if a rune is a separator character
func IsSeparator(r rune) bool {
	return r == ' ' || r == '_' || r == '-' || r == '.' || r == '/'
}

// EqualFold performs case-insensitive rune equality check
func EqualFold(a, b rune) bool {
	if a == b {
		return true
	}

	// Try simple ASCII case folding first (faster)
	if a < utf8.RuneSelf && b < utf8.RuneSelf {
		if 'A' <= a && a <= 'Z' {
			a += 'a' - 'A'
		}
		if 'A' <= b && b <= 'Z' {
			b += 'a' - 'A'
		}
		return a == b
	}

	// Use Unicode's more comprehensive case folding
	return strings.EqualFold(string(a), string(b))
}

// StringContainsIgnoreCase checks if string contains substring case-insensitively
func StringContainsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// HasPrefixIgnoreCase checks if string has prefix case-insensitively
func HasPrefixIgnoreCase(s, prefix string) bool {
	return strings.HasPrefix(strings.ToLower(s), strings.ToLower(prefix))
}

// ContainsNumbers checks if a string contains any numeric digits
func ContainsNumbers(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// IsOnlyNumbers checks if a string consists entirely of numeric digits
func IsOnlyNumbers(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// ContainsSpecialChars checks if a string contains special characters
// (non-alphanumeric characters excluding common separators)
func ContainsSpecialChars(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !IsSeparator(r) {
			return true
		}
	}
	return false
}

// IsValidInput checks if input should be processed for completions
// Returns false for strings that are only numbers, contain special characters, or are repetitive
func IsValidInput(s string) bool {
	// Reject empty strings
	if len(s) == 0 {
		return false
	}
	
	// Reject strings that are only numbers
	if IsOnlyNumbers(s) {
		return false
	}
	
	// Reject strings that contain special characters (except separators)
	if ContainsSpecialChars(s) {
		return false
	}
	
	// Reject repetitive strings like "dddd", "www", etc.
	if IsRepetitive(s) {
		return false
	}
	
	return true
}

// IsRepetitive checks if a string consists of repetitive characters
// Simple version that checks for repeated characters (e.g., "aaa", "bbb")
func IsRepetitive(s string) bool {
	if len(s) <= 2 {
		return false
	}
	
	// Check for simple repetition (same character repeated 3+ times)
	firstChar := s[0]
	for i := 1; i < len(s); i++ {
		if s[i] != firstChar {
			return false
		}
	}
	return true
}
