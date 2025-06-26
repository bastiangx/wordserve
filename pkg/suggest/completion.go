package suggest

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bastiangx/typr-lib/internal/utils"
	"github.com/bastiangx/typr-lib/pkg/dictionary"
	"github.com/charmbracelet/log"

	"github.com/tchap/go-patricia/v2/patricia"
)

var (
	stringPool     = sync.Map{}
	wordBufferPool = sync.Pool{
		New: func() any {
			buf := make([]byte, 64)
			return &buf
		},
	}
)

func internString(s string) string {
	if cached, exists := stringPool.Load(s); exists {
		return cached.(string)
	}
	stringPool.Store(s, s)
	return s
}

type Suggestion struct {
	Word            string
	Frequency       int
	WasCorrected    bool   `json:",omitempty"`
	OriginalPrefix  string `json:",omitempty"`
	CorrectedPrefix string `json:",omitempty"`
}

type Completer struct {
	trie         *patricia.Trie
	hotCache     *HotCache
	totalWords   int
	maxFrequency int
	wordFreqs    map[string]int
	chunkLoader  *dictionary.ChunkLoader
}

func NewCompleter() *Completer {
	return &Completer{
		trie:         patricia.NewTrie(),
		totalWords:   0,
		maxFrequency: 0,
		wordFreqs:    make(map[string]int),
	}
}

func NewLazyCompleter(dirPath string, chunkSize, maxWords int) *Completer {
	loader := dictionary.NewChunkLoader(dirPath, chunkSize, maxWords)
	maxHotWords := 20000

	return &Completer{
		trie:         patricia.NewTrie(),
		hotCache:     NewHotCache(maxHotWords),
		totalWords:   0,
		maxFrequency: 0,
		wordFreqs:    make(map[string]int),
		chunkLoader:  loader,
	}
}

func (c *Completer) AddWord(word string, frequency int) {
	c.trie.Insert(patricia.Prefix(word), frequency)
	c.wordFreqs[word] = frequency
	c.totalWords++
	if frequency > c.maxFrequency {
		c.maxFrequency = frequency
	}
}

func (c *Completer) Complete(prefix string, limit int) []Suggestion {
	// Get the active trie (either our own or from chunk loader)
	activeTrie := c.trie
	if c.chunkLoader != nil {
		activeTrie = c.chunkLoader.GetTrie()
		if activeTrie == nil {
			// Fall back to building trie from word frequencies
			activeTrie = patricia.NewTrie()
			wordFreqs := c.chunkLoader.GetWordFreqs()
			for word, freq := range wordFreqs {
				activeTrie.Insert(patricia.Prefix(word), freq)
			}
		}
	}

	// Extract lowercase prefix for trie lookup
	lowerPrefix := strings.ToLower(prefix)

	// Remember which positions were capitalized
	capitalPositions := make([]bool, len(prefix))
	for i, r := range prefix {
		capitalPositions[i] = r >= 'A' && r <= 'Z'
	}

	minFrequencyThreshold := 20
	if len(lowerPrefix) <= 2 || utils.IsRepetitive(lowerPrefix) {
		minFrequencyThreshold = 24
	}

	var suggestions []Suggestion

	// Visit subtree and collect
	err := activeTrie.VisitSubtree(patricia.Prefix(lowerPrefix), func(p patricia.Prefix, item patricia.Item) error {
		word := internString(string(p))

		// Skip exact matches (both lowercase) to avoid duplicating the input
		if word == lowerPrefix {
			return nil
		}

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

		if freq < minFrequencyThreshold {
			return nil
		}

		// Apply original capitalization pattern to the word
		if len(capitalPositions) > 0 {
			wordRunes := []rune(word)
			for i := 0; i < len(wordRunes) && i < len(capitalPositions); i++ {
				if capitalPositions[i] && wordRunes[i] >= 'a' && wordRunes[i] <= 'z' {
					wordRunes[i] = wordRunes[i] - 'a' + 'A'
				}
			}
			word = string(wordRunes)
		}

		suggestions = append(suggestions, Suggestion{
			Word:      word,
			Frequency: freq,
		})
		return nil
	})
	if err != nil {
		log.Errorf("Error visiting trie subtree: %v", err)
		return nil
	}

	// Add hot cache results if needed
	if len(suggestions) < limit-1 {
		hotSuggestions := SearchHotCache(c.hotCache, lowerPrefix, capitalPositions, minFrequencyThreshold)
		suggestions = append(suggestions, hotSuggestions...)
	}

	// Remove duplicates and sort
	uniqueSuggestions := DeduplicateAndSort(suggestions)

	// Sort suggestions by frequency (highest first)
	sort.Slice(uniqueSuggestions, func(i, j int) bool {
		return uniqueSuggestions[i].Frequency > uniqueSuggestions[j].Frequency
	})

	// Limit results
	if len(uniqueSuggestions) > limit && limit > 0 {
		uniqueSuggestions = uniqueSuggestions[:limit]
	}

	return uniqueSuggestions
}


func (c *Completer) LoadBinaryDictionary(filename string) error {
	return c.Initialize()
}

func (c *Completer) Initialize() error {
	if c.chunkLoader != nil {
		if err := c.chunkLoader.StartLazyLoading(); err != nil {
			return err
		}
		c.syncFromLoader()

		// Populate hot cache after some initial loading
		time.Sleep(100 * time.Millisecond)
		if c.hotCache != nil {
			// For now, skip hot cache to avoid complexity
			// TODO: Re-implement hot cache with patricia.Trie
		}

		return nil
	}
	return nil
}

func (c *Completer) syncFromLoader() {
	if c.chunkLoader != nil {
		c.wordFreqs = c.chunkLoader.GetWordFreqs()
		stats := c.chunkLoader.GetStats()
		c.totalWords = stats.TotalWords
		c.maxFrequency = stats.MaxFrequency
	}
}

func (c *Completer) LoadAllBinaries(dirPath string) error {
	return c.Initialize()
}

func (c *Completer) RequestMoreWords(additionalWords int) error {
	if c.chunkLoader != nil {
		return c.chunkLoader.RequestMoreChunks(additionalWords)
	}
	return nil // No-op if no chunk loader
}

func (c *Completer) Stop() {
	if c.chunkLoader != nil {
		c.chunkLoader.Stop()
	}
}



func (c *Completer) Stats() map[string]int {
	stats := map[string]int{
		"totalWords":   c.totalWords,
		"maxFrequency": c.maxFrequency,
	}

	if c.hotCache != nil {
		cacheStats := c.hotCache.Stats()
		for k, v := range cacheStats {
			stats[k] = v
		}
	}

	if c.chunkLoader != nil {
		loaderStats := c.chunkLoader.GetStats()
		stats["loadedChunks"] = loaderStats.LoadedChunks
		stats["availableChunks"] = loaderStats.AvailableChunks
		stats["chunkLoader"] = 1
	} else {
		stats["chunkLoader"] = 0
	}


	return stats
}
