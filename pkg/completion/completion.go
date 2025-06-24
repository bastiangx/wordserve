package completion

import (
	"bufio"
	"encoding/binary"
	"io"
	"os"
	"sort"
	"sync"

	"github.com/bastiangx/typr-lib/internal/utils"
	"github.com/bastiangx/typr-lib/pkg/fuzzy"
	"github.com/charmbracelet/log"

	"github.com/tchap/go-patricia/v2/patricia"
)

// Suggestion represents a word completion suggestion with its frequency
type Suggestion struct {
	Word            string
	Frequency       int
	WasCorrected    bool   `json:",omitempty"`
	OriginalPrefix  string `json:",omitempty"`
	CorrectedPrefix string `json:",omitempty"`
}

// Completer provides word completion functionality
// Uses patricia api's liberally and can use some optimization
// as of 18 march, normal lookup times are around 200milliseconds. not goud enough
type Completer struct {
	trie         *patricia.Trie
	totalWords   int
	maxFrequency int
	fuzzyMatcher *fuzzy.FuzzyMatcher
	wordFreqs    map[string]int
}

// WordBufferPool is a pool of byte slices for word completions
var wordBufferPool = sync.Pool{
	New: func() any {
		buffer := make([]byte, 64)
		return &buffer
	},
}

// NewCompleter initializes a new word completer
func NewCompleter() *Completer {
	return &Completer{
		trie:         patricia.NewTrie(),
		totalWords:   0,
		maxFrequency: 0,
		wordFreqs:    make(map[string]int),
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
	lowerPrefix, capitalsChan := utils.ProcessCapitals(prefix)

	// TODO: should be config defualt const
	minFrequencyThreshold := 20

	if len(lowerPrefix) <= 2 || isRepetitive(lowerPrefix) {
		minFrequencyThreshold = 24
	}

	suggestionChan := make(chan Suggestion, 100)
	doneChan := make(chan struct{})
	filteredSuggestions := make([]Suggestion, 0)

	// goroutine filtering suggestions based on prefix and freq threshold
	filter := utils.NewSuggestionFilter(prefix)
	go func() {
		defer close(doneChan)

		for suggestion := range suggestionChan {
			if filter.ShouldInclude(suggestion.Word) {
				filteredSuggestions = append(filteredSuggestions, suggestion)
			}
		}
	}()

	// Visit subtree and collect
	err := c.trie.VisitSubtree(patricia.Prefix(lowerPrefix), func(p patricia.Prefix, item patricia.Item) error {
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

		// through channel to filter goroutine
		suggestionChan <- Suggestion{
			Word:      string(p),
			Frequency: freq,
		}
		return nil
	})

	close(suggestionChan)
	<-doneChan

	if err != nil {
		log.Errorf("Error visiting trie subtree: %v", err)
		return nil
	}

	// Sort suggestions by frequency (highest first)
	sort.Slice(filteredSuggestions, func(i, j int) bool {
		return filteredSuggestions[i].Frequency > filteredSuggestions[j].Frequency
	})

	// Limit results
	if len(filteredSuggestions) > limit && limit > 0 {
		filteredSuggestions = filteredSuggestions[:limit]
	}

	if capitalInfo := <-capitalsChan; capitalInfo != nil {
		// waitgroup waits on all goroutines to finish processing
		var wg sync.WaitGroup
		wg.Add(len(filteredSuggestions))

		for i := range filteredSuggestions {
			go func(idx int) {
				defer wg.Done()
				wordChan := utils.ApplyCapitals(filteredSuggestions[idx].Word, capitalInfo)
				filteredSuggestions[idx].Word = <-wordChan
			}(i)
		}
		wg.Wait()
	}
	return filteredSuggestions
}

// isRepetitive checks if a string consists of repetitive characters
// like "aaa", "bbb", "ababab", etc.
func isRepetitive(s string) bool {
	if len(s) <= 1 {
		return false
	}

	// Check for simple repetition (same character repeated)
	allSame := true
	for i := 1; i < len(s); i++ {
		if s[i] != s[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return true
	}

	// Check for pattern repetition
	if len(s) >= 4 {
		// Check if it's a repeating pattern (like "abababab")
		for patternLen := 1; patternLen <= len(s)/2; patternLen++ {
			pattern := s[:patternLen]
			isRepeating := true

			for i := patternLen; i < len(s); i += patternLen {
				end := i + patternLen
				if end > len(s) {
					end = len(s)
				}

				segment := s[i:end]
				if pattern[:len(segment)] != segment {
					isRepeating = false
					break
				}
			}

			if isRepeating {
				return true
			}
		}
	}
	return false
}

// LoadBinaryDictionary loads a binary n-gram dictionary file
// Format: 4 bytes count header + (2 bytes length + string word + 4 bytes frequency) repeated
func (c *Completer) LoadBinaryDictionary(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatalf("Failed to open binary dictionary file: %v", err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Errorf("closing file: %v", err)
		}
	}(file)

	fileInfo, err := file.Stat()
	if err != nil {
		log.Fatalf("Failed to get file info: %v", err)
	}

	if fileInfo.Size() < 4 {
		log.Fatalf("Binary dictionary file is too small: %s", filename)
	}

	// Create binary reader
	reader := bufio.NewReader(file)

	// Read the header (count of entries)
	var totalEntries int32
	if err := binary.Read(reader, binary.LittleEndian, &totalEntries); err != nil {
		log.Fatalf("Reading total entries from binary file: %v", err)
	}
	log.Debugf("Total entries in binary dictionary: %d", totalEntries)

	count := 0

	for count < int(totalEntries) {
		// Read word length (2 bytes)
		var wordLen uint16
		if err := binary.Read(reader, binary.LittleEndian, &wordLen); err != nil {
			if err == io.EOF {
				break
			}
			log.Errorf("Reading word length: %v", err)
		}

		// Read the word
		var wordBytes []byte
		bufPtr := wordBufferPool.Get().(*[]byte)
		buffer := *bufPtr

		if cap(buffer) >= int(wordLen) {
			wordBytes = buffer[:wordLen]
		} else {
			// If the pooled buffer is too small, allocate a new one
			wordBytes = make([]byte, wordLen)
		}

		_, err = io.ReadFull(reader, wordBytes)
		if err != nil {
			wordBufferPool.Put(bufPtr)
			log.Errorf("Reading word bytes: %v", err)
		}
		word := string(wordBytes)

		// Return the buffer to the pool
		wordBufferPool.Put(bufPtr)

		// Read frequency (4 bytes)
		var freq uint32
		if err := binary.Read(reader, binary.LittleEndian, &freq); err != nil {
			log.Errorf("Reading frequency for word %s: %v", word, err)
		}

		// Add word to trie with actual frequency, not the default 1
		if freq > 0 {
			c.AddWord(word, int(freq))
		} else {
			// Fallback if frequency is somehow zero
			c.AddWord(word, 1)
		}

		// Add word to trie
		c.AddWord(word, int(freq))
		count++
	}

	log.Debugf("Loaded %d entries from binary dictionary: %s", count, filename)
	return nil
}

// LoadAllBinaries loads all ngram and trie binaries
func (c *Completer) LoadAllBinaries(dirPath string) error {
	unigramPath := dirPath + "/unigrams.bin"
	if _, err := os.Stat(unigramPath); err == nil {
		if err := c.LoadBinaryDictionary(unigramPath); err != nil {
			log.Fatalf("failed to load unigrams: %v", err)
		}
	}

	bigramPath := dirPath + "/bigrams.bin"
	if _, err := os.Stat(bigramPath); err == nil {
		if err := c.LoadBinaryDictionary(bigramPath); err != nil {
			log.Fatalf("failed to load bigrams: %v", err)
		}
	}

	trigramPath := dirPath + "/trigrams.bin"
	if _, err := os.Stat(trigramPath); err == nil {
		if err := c.LoadBinaryDictionary(trigramPath); err != nil {
			log.Fatalf("failed to load trigrams: %v", err)
		}
	}

	wordTriePath := dirPath + "/word_trie.bin"
	if _, err := os.Stat(wordTriePath); err == nil {
		if err := c.LoadBinaryDictionary(wordTriePath); err != nil {
			log.Fatalf("failed to load word trie: %v", err)
		}
	}

	return nil
}

// SaveBinaryDictionary exports the trie content to a binary file for persistence
func (c *Completer) SaveBinaryDictionary(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		log.Errorf("Creating binary file: %v", err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Errorf("Closing binary file: %v", err)
		}
	}(file)

	// count entries first
	count := 0
	err = c.trie.Visit(func(prefix patricia.Prefix, item patricia.Item) error {
		count++
		return nil
	})
	if err != nil {
		log.Errorf("Counting entries in trie: %v", err)
	}

	writer := bufio.NewWriter(file)
	defer func(writer *bufio.Writer) {
		err := writer.Flush()
		if err != nil {
			log.Errorf("Flushing writer: %v", err)
		}
	}(writer)

	// Write header (count of entries)
	if err := binary.Write(writer, binary.LittleEndian, int32(count)); err != nil {
		log.Errorf("Writing header: %v", err)
	}

	// Write entries
	err = c.trie.Visit(func(prefix patricia.Prefix, item patricia.Item) error {
		word := string(prefix)
		wordLen := uint16(len(word))

		// Write word length (2 bytes)
		if err := binary.Write(writer, binary.LittleEndian, wordLen); err != nil {
			log.Errorf("Writing word length: %v", err)
		}

		// Write word
		if _, err := writer.WriteString(word); err != nil {
			log.Errorf("Writing word %s: %v", word, err)
		}

		// Write frequency (4 bytes)
		freq := uint32(0)
		if f, ok := item.(int); ok {
			freq = uint32(f)
		}
		if err := binary.Write(writer, binary.LittleEndian, freq); err != nil {
			log.Errorf("Writing frequency for word %s: %v", word, err)
		}
		return nil
	})
	return err
}

// Stats returns statistics about the loaded dictionary
func (c *Completer) Stats() map[string]int {
	return map[string]int{
		"totalWords":   c.totalWords,
		"maxFrequency": c.maxFrequency,
	}
}

// InitFuzzyMatcher initializes the fuzzy matcher after dictionary loading
func (c *Completer) InitFuzzyMatcher() {
	c.fuzzyMatcher = fuzzy.NewFuzzyMatcher(c.wordFreqs)
}

func (c *Completer) CompleteWithFuzzy(prefix string, limit int) []Suggestion {
	if len(prefix) < 3 {
		return c.Complete(prefix, limit)
	}
	lowerPrefix, capitalsChan := utils.ProcessCapitals(prefix)
	correctedPrefix, wasFixed := c.fuzzyMatcher.SuggestCorrection(lowerPrefix)

	if wasFixed && correctedPrefix != lowerPrefix {
		capitalInfo := <-capitalsChan
		var correctedPrefixCapped string

		if capitalInfo != nil {
			wordChan := utils.ApplyCapitals(correctedPrefix, capitalInfo)
			correctedPrefixCapped = <-wordChan
		} else {
			correctedPrefixCapped = correctedPrefix
		}

		suggestions := c.Complete(correctedPrefixCapped, limit)

		for i := range suggestions {
			suggestions[i].WasCorrected = true
			suggestions[i].OriginalPrefix = prefix
			suggestions[i].CorrectedPrefix = correctedPrefix
		}
		return suggestions
	}
	return c.Complete(prefix, limit)
}
