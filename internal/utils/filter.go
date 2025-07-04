// Package utils implements internal functions for various ops, formats and checks.
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
	if a < utf8.RuneSelf && b < utf8.RuneSelf {
		if 'A' <= a && a <= 'Z' {
			a += 'a' - 'A'
		}
		if 'A' <= b && b <= 'Z' {
			b += 'a' - 'A'
		}
		return a == b
	}
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

// ContainsNumbers simply checks if a string contains any digits
func ContainsNumbers(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// IsOnlyNumbers checks if a string consists entirely of digits
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
// (non-alphanumeric chars excluding common separators)
func ContainsSpecialChars(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !IsSeparator(r) {
			return true
		}
	}
	return false
}

// IsRepetitive checks if a string consists of repetitive characters
func IsRepetitive(s string) bool {
	if len(s) <= 2 {
		return false
	}
	firstChar := s[0]
	for i := 1; i < len(s); i++ {
		if s[i] != firstChar {
			return false
		}
	}
	return true
}

// IsValidInput checks if input should be processed at all.
func IsValidInput(s string) bool {
	return len(s) > 0 && !IsOnlyNumbers(s) && !ContainsSpecialChars(s) && !IsRepetitive(s)
}
