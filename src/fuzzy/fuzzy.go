package fuzzy

import (
	"strings"
)

// MaxEditDistance defines how many character edits are allowed for a match
const MaxEditDistance = 2

// FuzzyMatcher handles approximate string matching
type FuzzyMatcher struct {
	dictionary map[string]bool
	wordFreq   map[string]int
}

// NewFuzzyMatcher creates a new fuzzy matcher with a predefined dictionary
func NewFuzzyMatcher(words map[string]int) *FuzzyMatcher {
	fm := &FuzzyMatcher{
		dictionary: make(map[string]bool, len(words)),
		wordFreq:   words,
	}

	// Populate dictionary for O(1) lookups
	for word := range words {
		fm.dictionary[word] = true
	}

	return fm
}

// SuggestCorrection returns the most likely correction for a potentially misspelled word
func (fm *FuzzyMatcher) SuggestCorrection(input string) (string, bool) {

	// If the word is already in the dictionary, no correction needed
	// But normalize case for exact matches
	lowerInput := strings.ToLower(input)
	for word := range fm.dictionary {
		if strings.ToLower(word) == lowerInput {
			return strings.ToLower(word), false
		}
	}

	// For very short inputs, don't attempt correction as it's too unreliable
	if len(input) < 3 {
		return input, false
	}

	// Look for similar words in the dictionary
	bestMatch := ""
	bestDistance := MaxEditDistance + 1
	bestFreq := 0

	// Apply a length filter to reduce search space
	minLen := len(input) - MaxEditDistance
	if minLen < 3 {
		minLen = 3
	}
	maxLen := len(input) + MaxEditDistance

	for word, freq := range fm.wordFreq {
		// Quick length check to skip obviously different words
		wordLen := len(word)
		if wordLen < minLen || wordLen > maxLen {
			continue
		}

		// Check if first character matches (common heuristic to improve suggestions)
		if len(word) > 0 && len(input) > 0 &&
			!strings.EqualFold(string(word[0]), string(input[0])) {
			continue
		}

		distance := levenshteinDistance(input, word)

		// If we found an exact match, return it immediately
		if distance == 0 {
			return strings.ToLower(word), true
		}

		// Simplified priority logic
		if distance <= MaxEditDistance {
			// If we don't have a match yet or this has smaller distance
			if bestMatch == "" || distance < bestDistance {
				bestMatch = word
				bestDistance = distance
				bestFreq = freq
			} else if distance == bestDistance {
				// If distances are equal, always prefer more frequent words
				if freq > bestFreq {
					bestMatch = word
					bestFreq = freq
				}
			}
		}
	}

	// If we found a good match, return it
	if bestMatch != "" {
		return strings.ToLower(bestMatch), true
	}

	// No good match found, return the original input
	return input, false
}

// levenshteinDistance calculates the minimum edit distance between two strings
func levenshteinDistance(s1, s2 string) int {
	// Convert to lowercase for case-insensitive comparison
	s1 = strings.ToLower(s1)
	s2 = strings.ToLower(s2)

	// Create a matrix of distances
	rows := len(s1) + 1
	cols := len(s2) + 1

	// Initialize the matrix
	matrix := make([][]int, rows)
	for i := range matrix {
		matrix[i] = make([]int, cols)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	// Fill in the matrix
	for i := 1; i < rows; i++ {
		for j := 1; j < cols; j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}

			// Calculate minimum of three operations: insertion, deletion, substitution
			matrix[i][j] = min(
				matrix[i-1][j]+1,      // Deletion
				matrix[i][j-1]+1,      // Insertion
				matrix[i-1][j-1]+cost, // Substitution
			)
		}
	}

	return matrix[rows-1][cols-1]
}

// min returns the minimum of three integers
func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
