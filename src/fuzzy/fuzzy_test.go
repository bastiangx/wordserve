package fuzzy

import (
	"fmt"
	"testing"
)

// Tests if FuzzyMatcher has the algo with our expected preferences

// IMPORTANT to know:
// preference: `exact match > most frequent word > levenshtein distance`
func TestFuzzyMatcher(t *testing.T) {
	dictionary := map[string]int{
		"apple":      100,
		"banana":     90,
		"orange":     80,
		"pear":       70,
		"grape":      60,
		"strawberry": 50,
		"blueberry":  40,
		"raspberry":  30,

		// similar spellings
		"there":   1000,
		"their":   950,
		"they're": 900,

		// simpler short words
		"car": 500,
		"cat": 490,
		"dog": 480,
		"the": 2000,

		// longer words
		"university":      300,
		"international":   290,
		"congratulations": 100,
		"accessibility":   95,

		// numbers mixed in words
		"word2vec":   50,
		"utf8":       45,
		"3dprinting": 40,

		// special chars
		"email@example.com": 30,
		"user-name":         25,
		"under_score":       20,

		// edge cases - keywords
		"algorithm": 200,
		"function":  190,
		"variable":  180,
	}

	matcher := NewFuzzyMatcher(dictionary)

	// testCases defines the input and expected output for each test case
	// corrected is true if the input should be corrected, false if it's already correct
	testCases := []struct {
		input          string
		expectedOutput string
		corrected      bool
		description    string
	}{
		// exact matches
		{"apple", "apple", false, "Exact match"},
		{"banana", "banana", false, "Exact match"},

		// case insensitive
		{"Apple", "apple", false, "Case insensitive match"},
		{"ORANGE", "orange", false, "Uppercase word"},

		// 1 char typo
		{"appl", "apple", true, "Missing character at end"},
		{"aple", "apple", true, "Missing character in middle"},
		{"appel", "apple", true, "Character transposition"},
		{"appke", "apple", true, "Character substitution"},
		{"applez", "apple", true, "Extra character at end"},

		// 2 char typos
		{"appl3", "apple", true, "Number substitution"},
		{"aplpe", "apple", true, "Two errors"},
		{"orunge", "orange", true, "Vowel substitution"},

		// similar words
		// our pref: choose the most frequent word, might change later idk
		{"ther", "the", true, "Should choose highest frequency"},
		{"thelr", "their", true, "Similar to multiple words"},

		// short words
		// min 2 chars rule
		{"ca", "ca", false, "Too short to correct"},
		{"do", "do", false, "Too short to correct"},

		// special cases & numbers
		{"word2vec", "word2vec", false, "Word with numbers"},
		{"wrd2vec", "word2vec", true, "Word with numbers - correction"},
		{"utf7", "utf8", true, "Number correction"},
		{"3dpronting", "3dprinting", true, "Number at beginning"},
		{"email@exampl.com", "email@example.com", true, "Special chars"},
		{"user-nme", "user-name", true, "Hyphenated word"},

		// longer words
		{"univeristy", "university", true, "Transposition in longer word"},
		{"internationl", "international", true, "Missing letter"},
		{"congratilations", "congratulations", true, "Vowel substitution"},

		// max edit distance
		// should prefer exact match over max edit distance
		{"axxle", "apple", true, "Exactly MaxEditDistance (2) edits"},
		{"bananana", "banana", true, "MaxEditDistance boundary"},

		// above max edit distance
		// should not correct
		{"axxxle", "axxxle", false, "Beyond MaxEditDistance (3 edits)"},
		{"banananas", "banananas", false, "Too many edits"},

		// gibberish test
		// should not correct
		{"xyzabc", "xyzabc", false, "No match in dictionary"},
		{"zzzzzzzzz", "zzzzzzzzz", false, "No match"},

		// edge cases
		// first letter heuristic
		{"orange", "orange", false, "Correct word"},
		{"prange", "prange", false, "Different first letter - no match"},

		/// edge cases - keywords
		{"algrithm", "algorithm", true, "Missing vowel"},
		{"fnction", "function", true, "Missing vowel"},
		{"varriable", "variable", true, "Extra character"},
	}

	// runs all tests
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result, corrected := matcher.SuggestCorrection(tc.input)
			if result != tc.expectedOutput {
				t.Errorf("Input '%s': expected '%s', got '%s'", tc.input, tc.expectedOutput, result)
			}
			if corrected != tc.corrected {
				t.Errorf("Input '%s': expected corrected=%v, got %v", tc.input, tc.corrected, corrected)
			}
		})
	}
}

// check for empty dictionary
func TestEmptyDictionary(t *testing.T) {
	matcher := NewFuzzyMatcher(map[string]int{})
	result, corrected := matcher.SuggestCorrection("test")

	if result != "test" || corrected {
		t.Errorf("Empty dictionary should return original word uncorrected")
	}
}

// check for diff first letter
func TestFirstLetterHeuristic(t *testing.T) {
	dictionary := map[string]int{
		"apple":  100,
		"orange": 90,
	}
	matcher := NewFuzzyMatcher(dictionary)
	result, corrected := matcher.SuggestCorrection("opple")

	// should not correct since theyre diff
	if result == "apple" || corrected {
		t.Errorf("First letter heuristic not working: matched '%s'", result)
	}
}

// check if our algo prefers the most frequent or longest word
func TestTherCase(t *testing.T) {
	dictionary := map[string]int{
		"their": 950,
		"there": 500,
		"the":   2000,
	}
	matcher := NewFuzzyMatcher(dictionary)
	result, _ := matcher.SuggestCorrection("ther")

	if result != "the" {
		t.Errorf("Expected 'ther' to correct to 'the', got '%s'", result)
	}
}

// check if our lev distance impl returns correct distance int
func TestLevenshteinDistance(t *testing.T) {
	testCases := []struct {
		a        string
		b        string
		expected int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"kitten", "sitting", 3},
		{"saturday", "sunday", 3},
		{"book", "back", 2},
		{"book", "books", 1},
		{"hello", "hallo", 1},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s→%s", tc.a, tc.b), func(t *testing.T) {
			dist := levenshteinDistance(tc.a, tc.b)
			if dist != tc.expected {
				t.Errorf("Expected distance %d, got %d", tc.expected, dist)
			}
		})
	}
}

// 1000 words in dict
// 5 different inputs
// 1000 iterations
// should not take more than 1ms
func BenchmarkSuggestCorrection(b *testing.B) {
	dictionary := make(map[string]int, 1000)
	for i := 0; i < 1000; i++ {
		dictionary[fmt.Sprintf("word%d", i)] = i
	}
	matcher := NewFuzzyMatcher(dictionary)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		inputs := []string{"wrd123", "word1", "wordd2", "woord3", "wird4"}
		matcher.SuggestCorrection(inputs[i%len(inputs)])
	}
}
