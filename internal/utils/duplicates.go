package utils

import (
	"strings"
)

// SuggestionFilter provides thread-safe filtering of duplicate suggestions
type SuggestionFilter struct {
	seenWords map[string]bool
	inputWord string
}

// NewSuggestionFilter creates a new filter instance that will exclude the given input word
func NewSuggestionFilter(input string) *SuggestionFilter {
	seenWords := make(map[string]bool)
	lowerInput := strings.ToLower(input)
	seenWords[lowerInput] = true

	return &SuggestionFilter{
		seenWords: seenWords,
		inputWord: lowerInput,
	}
}

// ShouldInclude checks if a word should be included in results (not a duplicate)
// Returns true if the word should be included, false if it's a duplicate
func (f *SuggestionFilter) ShouldInclude(word string) bool {
	lowerWord := strings.ToLower(word)
	if f.seenWords[lowerWord] {
		return false
	}
	f.seenWords[lowerWord] = true
	return true
}
