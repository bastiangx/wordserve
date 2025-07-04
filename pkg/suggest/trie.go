package suggest

import (
	"sync"

	"github.com/charmbracelet/log"
	"github.com/tchap/go-patricia/v2/patricia"
)

var (
	// Pools for mem reuse during trie traversal
	suggestionPool = sync.Pool{}
	seenWordsPool  = sync.Pool{}
)

func init() {
	suggestionPool.New = func() any {
		s := make([]Suggestion, 0, 75)
		return &s
	}
	seenWordsPool.New = func() any {
		m := make(map[string]bool, 150)
		return &m
	}
}

// SearchTrie performs trie traversal with early termination and deduplication.
//
// SearchTrie traverses the given trie to find words matching the specified prefix,
// applying frequency thresholds and result limits. The function uses memory pools
// for alloc management and implements early termination when enough
// results are collected.
//
// The lowerPrefix parameter should be a lowercase version of the desired prefix.
// Words in the trie matching this prefix are collected if their frequency meets
// or exceeds minThreshold. The search stops after collecting ~1.5x
// the requested limit to allow for better freq based sorting.
//
// The returned slice is a copy, and safe for the caller to modify.
//
// SearchTrie returns nil if an error occurs during trie traversal.
// The caller is responsible for ensuring the trie is properly initialized.
func SearchTrie(trie *patricia.Trie, lowerPrefix string, minThreshold, limit int) []Suggestion {
	if trie == nil {
		return []Suggestion{}
	}
	return searchTrieImpl(trie, lowerPrefix, minThreshold, limit)
}

//go:inline
func searchTrieImpl(trie *patricia.Trie, lowerPrefix string, minThreshold, limit int) []Suggestion {
	// Get pooled resources
	suggestionsPtr := suggestionPool.Get().(*[]Suggestion)
	suggestions := (*suggestionsPtr)[:0]
	defer func() {
		if cap(*suggestionsPtr) > 200 {
			*suggestionsPtr = make([]Suggestion, 0, 75)
		} else {
			*suggestionsPtr = (*suggestionsPtr)[:0]
		}
		suggestionPool.Put(suggestionsPtr)
	}()

	seenWordsPtr := seenWordsPool.Get().(*map[string]bool)
	seenWords := *seenWordsPtr
	defer func() {
		clear(seenWords)
		seenWordsPool.Put(seenWordsPtr)
	}()

	prefixBytes := patricia.Prefix(lowerPrefix)
	targetLen := limit + limit/2

	err := trie.VisitSubtree(prefixBytes, func(p patricia.Prefix, item patricia.Item) error {
		return processTrieNode(p, item, lowerPrefix, minThreshold, targetLen, &suggestions, seenWords)
	})

	if err != nil {
		log.Errorf("Error visiting trie subtree: %v", err)
		return nil
	}

	result := make([]Suggestion, len(suggestions))
	copy(result, suggestions)
	return result
}

//go:inline
func processTrieNode(p patricia.Prefix, item patricia.Item, lowerPrefix string, minThreshold, targetLen int, suggestions *[]Suggestion, seenWords map[string]bool) error {
	if len(*suggestions) >= targetLen {
		return nil
	}

	wordBytes := []byte(p)
	if len(wordBytes) == len(lowerPrefix) && string(wordBytes) == lowerPrefix {
		return nil
	}

	word := string(wordBytes)
	if seenWords[word] {
		return nil
	}

	freq := extractFrequency(item, word)
	if freq < minThreshold {
		return nil
	}

	seenWords[word] = true
	*suggestions = append(*suggestions, Suggestion{
		Word:      word,
		Frequency: freq,
	})
	return nil
}

// SearchTrieWithCallback performs zero-copy trie traversal using a callback.
//
// SearchTrieWithCallback provides a high perf alternative to [SearchTrie()]
// by eliminating mem allocations through callback result delivery.
//
// The callback receives each matching suggestion as it's found during
// traversal. The callback should return false to request early termination,
// or true to continue processing. Unlike [SearchTrie], this  does not
// sort results by frequency - sorting must be handled by the caller if needed.
//
// It stops when the limit is reached or when the callback returns false.
//
// SearchTrieWithCallback returns an error if trie traversal fails, or nil on success.
func SearchTrieWithCallback(trie *patricia.Trie, lowerPrefix string, minThreshold, limit int, callback func(Suggestion) bool) error {
	if trie == nil {
		return nil
	}
	return searchTrieWithCallbackImpl(trie, lowerPrefix, minThreshold, limit, callback)
}

//go:inline
func searchTrieWithCallbackImpl(trie *patricia.Trie, lowerPrefix string, minThreshold, limit int, callback func(Suggestion) bool) error {
	seenWordsPtr := seenWordsPool.Get().(*map[string]bool)
	seenWords := *seenWordsPtr
	defer func() {
		clear(seenWords)
		seenWordsPool.Put(seenWordsPtr)
	}()

	count := 0
	prefixBytes := patricia.Prefix(lowerPrefix)

	return trie.VisitSubtree(prefixBytes, func(p patricia.Prefix, item patricia.Item) error {
		return processCallbackNode(p, item, lowerPrefix, minThreshold, limit, &count, seenWords, callback)
	})
}

//go:inline
func processCallbackNode(p patricia.Prefix, item patricia.Item, lowerPrefix string, minThreshold, limit int, count *int, seenWords map[string]bool, callback func(Suggestion) bool) error {
	if *count >= limit {
		return nil
	}

	wordBytes := []byte(p)
	if len(wordBytes) == len(lowerPrefix) && string(wordBytes) == lowerPrefix {
		return nil
	}

	word := string(wordBytes)
	if seenWords[word] {
		return nil
	}

	freq := extractFrequency(item, word)
	if freq < minThreshold {
		return nil
	}

	seenWords[word] = true
	if !callback(Suggestion{Word: word, Frequency: freq}) {
		return nil
	}
	*count++
	return nil
}

// extractFrequency converts various numeric types to int frequency.
// Handles msgpack type conversions with common cases first.
//
// NOTE: the switch cases are not redundant as msgpack seemingly only
// returns floats for int32 and uint32 from ts client. (Obsidian)
//
//go:inline
func extractFrequency(item patricia.Item, word string) int {
	if freq, ok := item.(int); ok {
		return freq
	}
	if freq, ok := item.(int32); ok {
		return int(freq)
	}
	switch v := item.(type) {
	case uint32:
		return int(v)
	case float64:
		return int(v)
	default:
		log.Errorf("Unknown item type: %T for word %s", item, word)
		return 1
	}
}
