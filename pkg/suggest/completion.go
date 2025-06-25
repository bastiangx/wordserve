package suggest

import (
	"sort"
	"strings"
	"sync"

	"github.com/bastiangx/typr-lib/internal/utils"
	"github.com/bastiangx/typr-lib/pkg/dictionary"
	"github.com/charmbracelet/log"

	"github.com/tchap/go-patricia/v2/patricia"
)

// String interning for memory optimization
var (
	stringPool = sync.Map{} // intern strings to reduce memory
	wordBufferPool = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 64) // Pre-allocate 64 bytes
			return &buf
		},
	}
)

// internString reduces memory usage by reusing identical strings
func internString(s string) string {
	if cached, exists := stringPool.Load(s); exists {
		return cached.(string)
	}
	stringPool.Store(s, s)
	return s
}

// Suggestion represents a word completion suggestion with its frequency
type Suggestion struct {
	Word            string
	Frequency       int
	WasCorrected    bool   `json:",omitempty"`
	OriginalPrefix  string `json:",omitempty"`
	CorrectedPrefix string `json:",omitempty"`
}

// Completer provides word completion functionality
type Completer struct {
	trie         *patricia.Trie
	totalWords   int
	maxFrequency int
	wordFreqs    map[string]int
	chunkLoader  *dictionary.ChunkLoader
}

// NewCompleter creates a completer (legacy - use NewLazyCompleter instead)
func NewCompleter() *Completer {
	return &Completer{
		trie:         patricia.NewTrie(),
		totalWords:   0,
		maxFrequency: 0,
		wordFreqs:    make(map[string]int),
	}
}

// NewLazyCompleter creates a new completer with dictionary loading
func NewLazyCompleter(dirPath string, chunkSize, maxWords int) *Completer {
	loader := dictionary.NewChunkLoader(dirPath, chunkSize, maxWords)

	return &Completer{
		trie:         patricia.NewTrie(),
		totalWords:   0,
		maxFrequency: 0,
		wordFreqs:    make(map[string]int),
		chunkLoader:  loader,
	}
}

// AddWord adds a word with its frequency to the trie
func (c *Completer) AddWord(word string, frequency int) {
	c.trie.Insert(patricia.Prefix(word), frequency)
	c.wordFreqs[word] = frequency
	c.totalWords++
	if frequency > c.maxFrequency {
		c.maxFrequency = frequency
	}
}

// Complete returns suggestions for a given prefix with optional frequency threshold
func (c *Completer) Complete(prefix string, limit int) []Suggestion {
	// Get the active trie (either our own or from chunk loader)
	activeTrie := c.trie
	if c.chunkLoader != nil {
		activeTrie = c.chunkLoader.GetTrie()
	}
	
	// Extract lowercase prefix for trie lookup
	lowerPrefix := strings.ToLower(prefix)

	// Remember which positions were capitalized
	capitalPositions := make([]bool, len(prefix))
	for i, r := range prefix {
		capitalPositions[i] = r >= 'A' && r <= 'Z'
	}

	// TODO: should be config default const
	minFrequencyThreshold := 20

	if len(lowerPrefix) <= 2 || utils.IsRepetitive(lowerPrefix) {
		minFrequencyThreshold = 24
	}

	var suggestions []Suggestion

	// Visit subtree and collect
	err := activeTrie.VisitSubtree(patricia.Prefix(lowerPrefix), func(p patricia.Prefix, item patricia.Item) error {
		word := string(p)
		
		// Skip exact matches (both lowercase) to avoid duplicating the input
		if word == lowerPrefix {
			return nil
		}
		
		var freq int = 1

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

	// Remove duplicates (case-insensitive)
	seen := make(map[string]bool)
	var uniqueSuggestions []Suggestion
	for _, s := range suggestions {
		key := strings.ToLower(s.Word)
		if !seen[key] {
			seen[key] = true
			uniqueSuggestions = append(uniqueSuggestions, s)
		}
	}

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

// LoadBinaryDictionary - interface compatibility (no-op, use Initialize instead)
func (c *Completer) LoadBinaryDictionary(filename string) error {
	return c.Initialize()
}

// Initialize initializes the completer
func (c *Completer) Initialize() error {
	if c.chunkLoader != nil {
		// Start chunk loading
		if err := c.chunkLoader.StartLazyLoading(); err != nil {
			return err
		}
		// Update trie and word frequencies from the loader as chunks load
		c.syncFromLoader()
		return nil
	}
	return nil
}

// syncFromLoader synchronizes data from the chunk loader
func (c *Completer) syncFromLoader() {
	if c.chunkLoader != nil {
		c.trie = c.chunkLoader.GetTrie()
		c.wordFreqs = c.chunkLoader.GetWordFreqs()
		stats := c.chunkLoader.GetStats()
		c.totalWords = stats.TotalWords
		c.maxFrequency = stats.MaxFrequency
	}
}

// LoadAllBinaries - interface compatibility (no-op, use Initialize instead)
func (c *Completer) LoadAllBinaries(dirPath string) error {
	return c.Initialize()
}

// RequestMoreWords requests loading of additional words
func (c *Completer) RequestMoreWords(additionalWords int) error {
	if c.chunkLoader != nil {
		return c.chunkLoader.RequestMoreChunks(additionalWords)
	}
	return nil // No-op if no chunk loader
}

// Stop stops the lazy loading process
func (c *Completer) Stop() {
	if c.chunkLoader != nil {
		c.chunkLoader.Stop()
	}
}

// Stats returns statistics about the loaded dictionary
// TODO: move to interface.go so it can collect all stats from diff instances
func (c *Completer) Stats() map[string]int {
	stats := map[string]int{
		"totalWords":   c.totalWords,
		"maxFrequency": c.maxFrequency,
	}

	// Add chunk loader stats if available
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
