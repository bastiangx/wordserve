package suggest

import (
	"runtime"
	"sort"

	"github.com/bastiangx/typr-lib/internal/utils"
	"github.com/bastiangx/typr-lib/pkg/config"
	"github.com/bastiangx/typr-lib/pkg/dictionary"

	"github.com/tchap/go-patricia/v2/patricia"
)

var defaultConfig = &config.Config{Server: config.ServerConfig{MaxLimit: 64, MinPrefix: 1, MaxPrefix: 60, EnableFilter: true}, Dict: config.DictConfig{
	MaxWords:               50000,
	ChunkSize:              10000,
	MinFreqThreshold:       20,
	MinFreqShortPrefix:     24,
	MaxWordCountValidation: 1000000,
}, CLI: config.CliConfig{DefaultLimit: 24, DefaultMinLen: 1, DefaultMaxLen: 24, DefaultNoFilter: false}}

// Suggestion represents a word completion result with its frequency ranking.
type Suggestion struct {
	Word      string `msgpack:"w"`
	Frequency int    `msgpack:"f"`
}

// Completer provides trie-based word completion with lazy loading support.
//
// Completer can operate in two modes: static mode where words are added
// individually via [AddWord], or lazy mode where words are loaded from
// chunked binary files as needed. The lazy mode is particularly useful
// for large dictionaries that exceed memory constraints.
//
// The completer automatically manages a fallback trie when the chunk loader
// cannot provide an active trie, ensuring consistent operation across
// different dictionary states.
type Completer struct {
	trie               *patricia.Trie
	totalWords         int
	maxFrequency       int
	wordFreqs          map[string]int
	chunkLoader        *dictionary.Loader
	cachedFallbackTrie *patricia.Trie
	fallbackBuilt      bool
}

// NewCompleter creates a new completer for static word addition.
//
// The returned completer starts with an empty dictionary and words must be
// added individually using [AddWord]. This mode is suitable for smaller
// dictionaries or when words are generated dynamically.
func NewCompleter() *Completer {
	return &Completer{
		trie:      patricia.NewTrie(),
		wordFreqs: make(map[string]int),
	}
}

// NewLazyCompleter creates a completer with lazy loading from chunked binary files.
//
// NewLazyCompleter sets up a completer that loads dictionary data incrementally
// from binary files in the specified directory. The maxWords parameter controls
// the total dictionary size, while actual loading is deferred until [Initialize]
// is called.
//
// This mode is recommended for large dictionaries where loading all words
// into memory at startup would be prohibitive. The chunk loader manages
// memory usage by loading only the most relevant portions of the dictionary.
func NewLazyCompleter(dirPath string, chunkSize, maxWords int) *Completer {
	return &Completer{
		trie:        patricia.NewTrie(),
		wordFreqs:   make(map[string]int),
		chunkLoader: dictionary.NewLoader(dirPath, maxWords),
	}
}

//go:inline
func (c *Completer) AddWord(word string, frequency int) {
	c.trie.Insert(patricia.Prefix(word), frequency)
	c.wordFreqs[word] = frequency
	c.totalWords++
	if frequency > c.maxFrequency {
		c.maxFrequency = frequency
	}
}

// Complete returns word suggestions for a given prefix.
//
// Complete searches the completer's dictionary for words beginning with the
// specified prefix, applying frequency thresholds and capitalization patterns.
// Results are sorted by frequency in descending order and limited to the
// requested number of suggestions.
//
// The prefix parameter preserves the original capitalization, which is applied
// to all returned suggestions. For example, searching for "HEL" will return
// suggestions like "HELLO" and "HELP" with matching capitalization.
//
// If the completer uses a chunk loader and no active trie is available,
// Complete builds and caches a fallback trie from loaded word frequencies.
// This cached trie is reused across subsequent calls for efficiency.
//
// Frequency thresholds are automatically adjusted based on prefix length:
// shorter prefixes (≤2 characters) use a higher threshold to reduce noise,
// while longer prefixes use the standard threshold for broader results.
//
// Complete returns an empty slice if no matches are found or if an error
// occurs during trie traversal.
func (c *Completer) Complete(prefix string, limit int) []Suggestion {
	return c.complete(prefix, limit)
}

//go:inline
func (c *Completer) complete(prefix string, limit int) []Suggestion {
	activeTrie := c.getActiveTrie()
	lowerPrefix, capitalInfo := utils.GetCapitalDetails(prefix)
	minFrequencyThreshold := c.getFrequencyThreshold(lowerPrefix)

	suggestions := SearchTrie(activeTrie, lowerPrefix, minFrequencyThreshold, limit)
	c.sortAndLimitSuggestions(&suggestions, limit)
	c.applyCapitalization(suggestions, capitalInfo)

	return suggestions
}

//go:inline
func (c *Completer) getActiveTrie() *patricia.Trie {
	if c.chunkLoader == nil {
		return c.trie
	}
	if activeTrie := c.chunkLoader.GetTrie(); activeTrie != nil {
		return activeTrie
	}
	return c.getFallbackTrie()
}

//go:inline
func (c *Completer) getFallbackTrie() *patricia.Trie {
	if c.fallbackBuilt {
		return c.cachedFallbackTrie
	}
	return c.buildFallbackTrie()
}

func (c *Completer) buildFallbackTrie() *patricia.Trie {
	c.cachedFallbackTrie = patricia.NewTrie()
	wordFreqs := c.chunkLoader.GetWordFreqs()
	for word, freq := range wordFreqs {
		c.cachedFallbackTrie.Insert(patricia.Prefix(word), freq)
	}
	c.fallbackBuilt = true
	return c.cachedFallbackTrie
}

//go:inline
func (c *Completer) getFrequencyThreshold(lowerPrefix string) int {
	if len(lowerPrefix) <= 2 || utils.IsRepetitive(lowerPrefix) {
		return defaultConfig.Dict.MinFreqShortPrefix
	}
	return defaultConfig.Dict.MinFreqThreshold
}

func (c *Completer) sortAndLimitSuggestions(suggestions *[]Suggestion, limit int) {
	sort.Slice(*suggestions, func(i, j int) bool {
		return (*suggestions)[i].Frequency > (*suggestions)[j].Frequency
	})
	if len(*suggestions) > limit && limit > 0 {
		*suggestions = (*suggestions)[:limit]
	}
}

//go:inline
func (c *Completer) applyCapitalization(suggestions []Suggestion, capitalInfo *utils.CapitalInfo) {
	if capitalInfo == nil {
		return
	}
	for i := range suggestions {
		suggestions[i].Word = utils.CapitalizeAtPositions(suggestions[i].Word, capitalInfo)
	}
}

// CompleteWithCallback provides zero-copy completion using a callback.
//
// CompleteWithCallback offers the same functionality as [Complete] but uses
// a callback mechanism to deliver results incrementally, eliminating the final
// memory allocation for the returned slice. This makes it ideal for high-performance
// scenarios where memory efficiency is critical.
//
// The callback function receives each suggestion in frequency-sorted order
// (highest frequency first) and should return false to request early termination.
// Capitalization is applied before calling the callback, ensuring results match
// the original prefix casing.
//
// Unlike the callback-based trie functions, CompleteWithCallback sorts results
// by frequency before delivery, providing the same ordering guarantees as [Complete].
// However, this requires collecting all results before sorting, which uses some
// temporary memory.
//
// CompleteWithCallback returns an error if trie traversal fails, or nil on success.
// The number of suggestions delivered may be less than the limit if the callback
// returns false or if fewer matches are found.
func (c *Completer) CompleteWithCallback(prefix string, limit int, callback func(Suggestion) bool) error {
	return c.completeWithCallback(prefix, limit, callback)
}

//go:inline
func (c *Completer) completeWithCallback(prefix string, limit int, callback func(Suggestion) bool) error {
	activeTrie := c.getActiveTrie()
	lowerPrefix, capitalInfo := utils.GetCapitalDetails(prefix)
	minFrequencyThreshold := c.getFrequencyThreshold(lowerPrefix)

	suggestions, err := c.collectSuggestions(activeTrie, lowerPrefix, minFrequencyThreshold, limit)
	if err != nil {
		return err
	}

	c.sortAndLimitSuggestions(&suggestions, limit)
	return c.deliverSuggestions(suggestions, capitalInfo, callback)
}

//go:inline
func (c *Completer) collectSuggestions(trie *patricia.Trie, lowerPrefix string, minFrequencyThreshold, limit int) ([]Suggestion, error) {
	suggestions := make([]Suggestion, 0, limit*2)
	err := SearchTrieWithCallback(trie, lowerPrefix, minFrequencyThreshold, limit*2, func(s Suggestion) bool {
		suggestions = append(suggestions, s)
		return true
	})
	return suggestions, err
}

//go:inline
func (c *Completer) deliverSuggestions(suggestions []Suggestion, capitalInfo *utils.CapitalInfo, callback func(Suggestion) bool) error {
	for _, s := range suggestions {
		if capitalInfo != nil {
			s.Word = utils.CapitalizeAtPositions(s.Word, capitalInfo)
		}
		if !callback(s) {
			break
		}
	}
	return nil
}

//go:inline
func (c *Completer) LoadBinaryDictionary(filename string) error {
	return c.Initialize()
}

func (c *Completer) Initialize() error {
	if c.chunkLoader != nil {
		if err := c.chunkLoader.StartLoading(); err != nil {
			return err
		}
		c.syncFromLoader()

		return nil
	}
	return nil
}

//go:inline
func (c *Completer) syncFromLoader() {
	if c.chunkLoader != nil {
		stats := c.chunkLoader.GetStats()
		c.totalWords = stats.TotalWords
		c.maxFrequency = stats.MaxFrequency
	}
}

//go:inline
func (c *Completer) LoadAllBinaries(dirPath string) error {
	return c.Initialize()
}

//go:inline
func (c *Completer) RequestMoreWords(additionalWords int) error {
	if c.chunkLoader != nil {
		return c.chunkLoader.RequestMore(additionalWords)
	}
	return nil
}

//go:inline
func (c *Completer) Stop() {
	if c.chunkLoader != nil {
		c.chunkLoader.Stop()
	}
}

// ForceCleanup forces GC to reclaim memory.
//
//go:inline
func (c *Completer) ForceCleanup() {
	runtime.GC()
}

//go:inline
func (c *Completer) Stats() map[string]int {
	return c.buildStatsMap()
}

//go:inline
func (c *Completer) buildStatsMap() map[string]int {
	stats := make(map[string]int, 6)
	stats["totalWords"] = c.totalWords
	stats["maxFrequency"] = c.maxFrequency
	c.addLoaderStats(stats)
	return stats
}

//go:inline
func (c *Completer) addLoaderStats(stats map[string]int) {
	if c.chunkLoader != nil {
		loaderStats := c.chunkLoader.GetStats()
		stats["loadedChunks"] = loaderStats.LoadedChunks
		stats["availableChunks"] = loaderStats.AvailableChunks
		stats["chunkLoader"] = 1
	} else {
		stats["chunkLoader"] = 0
	}
}

//go:inline
func (c *Completer) GetChunkLoader() *dictionary.Loader {
	return c.chunkLoader
}

// InvalidateFallbackCache clears the cached fallback trie when chunk loader state changes
//
//go:inline
func (c *Completer) InvalidateFallbackCache() {
	c.cachedFallbackTrie = nil
	c.fallbackBuilt = false
}
