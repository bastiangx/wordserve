package suggest

import (
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/bastiangx/typr-lib/internal/utils"
	"github.com/bastiangx/typr-lib/pkg/config"
	"github.com/bastiangx/typr-lib/pkg/dictionary"
	"github.com/charmbracelet/log"

	"github.com/tchap/go-patricia/v2/patricia"
)

var (
	// Remove string pooling entirely - causes more leaks than it prevents
	wordBufferPool = sync.Pool{
		New: func() any {
			buf := make([]byte, 64)
			return &buf
		},
	}
	suggestionPool = sync.Pool{
		New: func() any {
			return make([]Suggestion, 0, 50)
		},
	}
	seenWordsPool = sync.Pool{
		New: func() any {
			return make(map[string]bool, 100)
		},
	}
	capitalPool = sync.Pool{
		New: func() any {
			buf := make([]bool, 64)
			return &buf
		},
	}
	// Cache config to avoid repeated allocations
	defaultConfig = config.DefaultConfig()
)

// Remove string interning entirely - it's causing memory leaks
// func internString(s string) string { return s }

// String pooling removed

type Suggestion struct {
	Word            string `msgpack:"w"`
	Frequency       int    `msgpack:"f"`
	WasCorrected    bool   `msgpack:"wc,omitempty"`
	OriginalPrefix  string `msgpack:"op,omitempty"`
	CorrectedPrefix string `msgpack:"cp,omitempty"`
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

	return &Completer{
		trie:         patricia.NewTrie(),
		hotCache:     nil, // Disable hot cache completely to prevent leaks
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

	// Get capital positions from pool to avoid allocation
	capitalBuf := capitalPool.Get().(*[]bool)
	defer capitalPool.Put(capitalBuf)
	capitalPositions := (*capitalBuf)[:0]
	if cap(*capitalBuf) < len(prefix) {
		*capitalBuf = make([]bool, len(prefix))
		capitalPositions = *capitalBuf
	} else {
		capitalPositions = (*capitalBuf)[:len(prefix)]
	}
	for i, r := range prefix {
		capitalPositions[i] = r >= 'A' && r <= 'Z'
	}

	// Use cached config to avoid allocations
	minFrequencyThreshold := defaultConfig.Dict.MinFreqThreshold
	if len(lowerPrefix) <= 2 || utils.IsRepetitive(lowerPrefix) {
		minFrequencyThreshold = defaultConfig.Dict.MinFreqShortPrefix
	}

	// Get suggestions slice from pool and reset it
	suggestions := suggestionPool.Get().([]Suggestion)
	suggestions = suggestions[:0]
	// Don't use defer - we return the slice directly

	// Get seen words map from pool
	seenWords := seenWordsPool.Get().(map[string]bool)
	defer func() {
		// Clear map before returning to pool
		for k := range seenWords {
			delete(seenWords, k)
		}
		seenWordsPool.Put(seenWords)
	}()
	
	// Primary trie search
	err := activeTrie.VisitSubtree(patricia.Prefix(lowerPrefix), func(p patricia.Prefix, item patricia.Item) error {
		// Stop early if we have enough results to avoid unnecessary work
		if len(suggestions) >= limit*2 {
			return nil
		}
		
		// CRITICAL FIX: Avoid string conversion unless absolutely necessary
		// Compare prefix directly as bytes to avoid allocation
		if len(p) == len(lowerPrefix) && string(p) == lowerPrefix {
			return nil
		}
		
		// Only convert to string when we actually need it
		prefixStr := string(p)
		
		// Check if we've already seen this word
		if seenWords[prefixStr] {
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
		
		// Mark as seen
		seenWords[prefixStr] = true
		
		// MINIMAL capitalization - avoid allocations when possible
		word := prefixStr
		// Skip capitalization for now to eliminate allocations
		// TODO: Implement byte-level capitalization if needed

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

	// NO SECONDARY HOT CACHE SEARCH - eliminated duplicate traversal
	// Hot cache is now redundant with the main trie search

	// Sort by frequency (highest first) - only sort what we need
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Frequency > suggestions[j].Frequency
	})

	// Limit results early to avoid copying unnecessary data
	if len(suggestions) > limit && limit > 0 {
		suggestions = suggestions[:limit]
	}

	// Create a copy to return since we can't return pooled slice
	result := make([]Suggestion, len(suggestions))
	copy(result, suggestions)
	
	// Return slice to pool
	suggestionPool.Put(suggestions)
	
	return result
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

		// Hot cache disabled to prevent memory leaks

		return nil
	}
	return nil
}

func (c *Completer) syncFromLoader() {
	if c.chunkLoader != nil {
		// Don't copy the entire wordFreqs map - just get stats
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
	// Clear hot cache to prevent memory leaks
	if c.hotCache != nil {
		c.hotCache.ClearAll()
	}
	// String pooling was removed
}

// ForceCleanup performs cleanup - call after every N completions
func (c *Completer) ForceCleanup() {
	// Force garbage collection to reclaim memory
	runtime.GC()
	
	// Don't clear hot cache - just trim if too large
	if c.hotCache != nil {
		c.hotCache.TrimToSize()
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

func (c *Completer) GetChunkLoader() *dictionary.ChunkLoader {
	return c.chunkLoader
}
