package suggest

import (
	"bufio"
	"encoding/binary"
	"io"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/bastiangx/typr-lib/internal/utils"
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
type Completer struct {
	trie         *patricia.Trie
	totalWords   int
	maxFrequency int
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

		word := string(p)

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

// LoadBinaryDictionary loads a binary n-gram dictionary file
// Format: 4 bytes count header + (2 bytes length + string word + 4 bytes frequency) repeated
// TODO: move to `pkg/dictionary/loader.go` and rename the func
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
		count++
	}

	log.Debugf("Loaded %d entries from binary dictionary: %s", count, filename)
	return nil
}

// LoadAllBinaries loads the word trie binary dictionary
// TODO: move to `pkg/dictionary/loader.go` and rename the func
func (c *Completer) LoadAllBinaries(dirPath string) error {
	// Only load the word trie - it contains both words and frequencies
	wordTriePath := dirPath + "/word_trie.bin"
	if _, err := os.Stat(wordTriePath); err == nil {
		if err := c.LoadBinaryDictionary(wordTriePath); err != nil {
			log.Fatalf("failed to load word trie: %v", err)
		}
	} else {
		log.Warnf("Word trie file not found at: %s", wordTriePath)
		return err
	}

	return nil
}

// SaveBinaryDictionary exports the trie content to a binary file for persistence
// TODO: move to `pkg/dictionary/binaries.go` and rename the func
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
// TODO: move to interface.go so it can collect all stats from diff instances
func (c *Completer) Stats() map[string]int {
	return map[string]int{
		"totalWords":   c.totalWords,
		"maxFrequency": c.maxFrequency,
	}
}
