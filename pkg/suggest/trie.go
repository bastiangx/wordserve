package suggest

import (
	"github.com/charmbracelet/log"
	"github.com/tchap/go-patricia/v2/patricia"
)

func SearchTrie(trie *patricia.Trie, lowerPrefix string, capitalPositions []bool, minThreshold int) []Suggestion {
	if trie == nil {
		return []Suggestion{}
	}

	var suggestions []Suggestion

	err := trie.VisitSubtree(patricia.Prefix(lowerPrefix), func(p patricia.Prefix, item patricia.Item) error {
		// Avoid string conversion unless necessary
		prefixStr := string(p)
		if prefixStr == lowerPrefix {
			return nil
		}
		
		word := prefixStr

		freq := 1

		switch v := item.(type) {
		case int:
			freq = v
		case int32:
			freq = int(v)
		case uint32:
			freq = int(v)
		case float64:
			freq = int(v)
		default:
			log.Errorf("Unknown item type: %T for word %s", item, p)
		}

		if freq < minThreshold {
			return nil
		}

		word = ApplyCapitalization(word, capitalPositions)

		suggestions = append(suggestions, Suggestion{
			Word:      word,
			Frequency: freq,
		})
		return nil
	})

	if err != nil {
		log.Errorf("Error visiting trie subtree: %v", err)
	}

	return suggestions
}

func SearchHotCache(hotCache *HotCache, lowerPrefix string, capitalPositions []bool, minThreshold int) []Suggestion {
	if hotCache == nil {
		return []Suggestion{}
	}

	prefixes := hotCache.Search(lowerPrefix, minThreshold)
	var suggestions []Suggestion

	for _, p := range prefixes {
		word := string(p)
		
		// Get score from hot cache trie
		trie := hotCache.GetTrie()
		item := trie.Get(p)
		if item == nil {
			continue
		}
		
		score := item.(int)
		word = ApplyCapitalization(word, capitalPositions)

		suggestions = append(suggestions, Suggestion{
			Word:      word,
			Frequency: score,
		})
	}

	return suggestions
}

func ApplyCapitalization(word string, capitalPositions []bool) string {
	if len(capitalPositions) == 0 {
		return word
	}

	wordRunes := []rune(word)
	for i := 0; i < len(wordRunes) && i < len(capitalPositions); i++ {
		if capitalPositions[i] && wordRunes[i] >= 'a' && wordRunes[i] <= 'z' {
			wordRunes[i] = wordRunes[i] - 'a' + 'A'
		}
	}
	return string(wordRunes)
}

// DeduplicateAndSort was removed - deduplication now happens inline during traversal
