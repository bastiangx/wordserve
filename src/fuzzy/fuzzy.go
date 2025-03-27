package fuzzy

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// FuzzyMatcher handles approximate string matching
type FuzzyMatcher struct {
	words    []string
	wordFreq map[string]int
}

// NewFuzzyMatcher creates a new fuzzy matcher with a predefined dictionary
func NewFuzzyMatcher(words map[string]int) *FuzzyMatcher {
	// Extract all words into a slice for easier processing
	wordList := make([]string, 0, len(words))
	for word := range words {
		wordList = append(wordList, word)
	}

	return &FuzzyMatcher{
		words:    wordList,
		wordFreq: words,
	}
}

// SuggestCorrection returns the most likely correction for a potentially misspelled word
func (fm *FuzzyMatcher) SuggestCorrection(input string) (string, bool) {
	// For very short inputs, don't attempt correction
	if len(input) < 2 {
		return input, false
	}

	// Convert to lowercase for case-insensitive comparison
	lowerInput := strings.ToLower(input)

	// Check for exact match first (case insensitive)
	for _, word := range fm.words {
		if strings.ToLower(word) == lowerInput {
			return strings.ToLower(word), false // Found exact match
		}
	}

	// Get matches
	matches := fm.findMatches(lowerInput)

	// Adjust scores based on frequency and length difference
	for i := range matches {
		// Add frequency bonus
		if freq, ok := fm.wordFreq[matches[i].Str]; ok && freq > 0 {
			// Log scale to prevent frequency from dominating
			matches[i].Score += min(freq/10, 30)
		}

		// Penalize length difference
		lengthDiff := abs(len(matches[i].Str) - len(input))
		matches[i].Score -= lengthDiff * 2
	}

	// Sort matches by score
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	// Return the best match if we have one
	if len(matches) > 0 {
		return strings.ToLower(matches[0].Str), true
	}

	// No good match found
	return input, false
}

// Constants for scoring
const (
	firstCharMatchBonus            = 15
	adjacentMatchBonus             = 10
	separatorMatchBonus            = 12
	camelCaseMatchBonus            = 12
	unmatchedLeadingCharPenalty    = -3
	maxUnmatchedLeadingCharPenalty = -9
)

// Match represents a matched string with score
type Match struct {
	Str            string
	Score          int
	MatchedIndexes []int
}

// findMatches is the core fuzzy matching algorithm adapted from the example_good.go
func (fm *FuzzyMatcher) findMatches(pattern string) []Match {
	if len(pattern) == 0 {
		return nil
	}

	var matches []Match
	patternRunes := []rune(pattern)

	for _, candidate := range fm.words {
		candidateLower := strings.ToLower(candidate)

		// Skip if first character doesn't match and pattern is long enough
		if len(pattern) > 1 && len(candidateLower) > 0 &&
			pattern[0] != candidateLower[0] {
			continue
		}

		match := Match{
			Str:            candidate,
			Score:          0,
			MatchedIndexes: make([]int, 0, len(patternRunes)),
		}

		// Try to match the pattern against the candidate
		if fm.runFuzzyMatch(patternRunes, candidateLower, &match) {
			// Apply additional penalties
			penalty := len(match.MatchedIndexes) - len(candidateLower)
			match.Score += penalty

			matches = append(matches, match)
		}
	}

	return matches
}

// runFuzzyMatch tests if pattern matches the candidate string
// and calculates a score. Returns true if there's a match.
func (fm *FuzzyMatcher) runFuzzyMatch(pattern []rune, candidate string, match *Match) bool {
	candidateRunes := []rune(candidate)

	var last rune
	var lastIndex int
	var currAdjacentMatchBonus int
	patternIndex := 0
	bestScore := -1
	matchedIndex := -1

	// Scan the candidate string
	for i := 0; i < len(candidateRunes); i++ {
		curr := candidateRunes[i]

		// Check if current rune matches the current pattern rune
		if equalFold(curr, pattern[patternIndex]) {
			score := 0

			// First character match bonus
			if i == 0 {
				score += firstCharMatchBonus
			}

			// Camel case bonus (lowercase to uppercase transition)
			if i > 0 && unicode.IsLower(last) && unicode.IsUpper(curr) {
				score += camelCaseMatchBonus
			}

			// Separator bonus (match after a separator like space, dash, etc.)
			if i > 0 && isSeparator(last) {
				score += separatorMatchBonus
			}

			// Adjacent match bonus
			if len(match.MatchedIndexes) > 0 {
				lastMatch := match.MatchedIndexes[len(match.MatchedIndexes)-1]
				bonus := 0
				if lastIndex == lastMatch {
					bonus = currAdjacentMatchBonus*2 + adjacentMatchBonus
					currAdjacentMatchBonus = bonus
				} else {
					currAdjacentMatchBonus = 0
				}
				score += bonus
			}

			// Update best score if this match is better
			if score > bestScore {
				bestScore = score
				matchedIndex = i
			}

			// Check if we should commit this match
			var nextPatternRune rune
			if patternIndex < len(pattern)-1 {
				nextPatternRune = pattern[patternIndex+1]
			}

			var nextCandidateRune rune
			if i < len(candidateRunes)-1 {
				nextCandidateRune = candidateRunes[i+1]
			}

			if equalFold(nextPatternRune, nextCandidateRune) || nextCandidateRune == 0 {
				if matchedIndex > -1 {
					// Apply penalty for unmatched leading characters
					if len(match.MatchedIndexes) == 0 {
						penalty := matchedIndex * unmatchedLeadingCharPenalty
						bestScore += max(penalty, maxUnmatchedLeadingCharPenalty)
					}

					match.Score += bestScore
					match.MatchedIndexes = append(match.MatchedIndexes, matchedIndex)
					bestScore = -1
					patternIndex++
				}
			}
		}

		last = curr
		lastIndex = i

		// If we've matched all pattern characters, we have a full match
		if patternIndex >= len(pattern) {
			return true
		}
	}

	// Return true if we've matched all pattern characters
	return patternIndex >= len(pattern)
}

// Helper function to check if a rune is a separator
func isSeparator(r rune) bool {
	return r == ' ' || r == '_' || r == '-' || r == '.' || r == '/'
}

// Helper function for case-insensitive rune equality
func equalFold(a, b rune) bool {
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

// abs returns the absolute value of x
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
