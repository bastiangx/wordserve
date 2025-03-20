package completion

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/tchap/go-patricia/v2/patricia"
)

// Suggestion represents a word completion suggestion with its frequency
type Suggestion struct {
	Word      string
	Frequency int
}

// Completer provides word completion functionality
// Uses patricia api's liberally and can use some optimization
// as of 18 march, normal lookup times are around 200milliseconds. not goud enough
type Completer struct {
	trie         *patricia.Trie
	totalWords   int
	maxFrequency int
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
	}
}

// AddWord adds a word with its frequency to the trie
func (c *Completer) AddWord(word string, frequency int) {
	c.trie.Insert(patricia.Prefix(word), frequency)
	c.totalWords++
	if frequency > c.maxFrequency {
		c.maxFrequency = frequency
	}
}

// Complete returns suggestions for a given prefix with optional frequency threshold
func (c *Completer) Complete(prefix string, limit int) []Suggestion {
	// Set minimum frequency threshold to filter out gibberish
	// This is a reasonable starting value - adjust based on your corpus size
	// CONSTANT: all words
	// DEBUG: im doing higher threshold since i have the biggest corpus rn
	minFrequencyThreshold := 40

	// For short or repetitive prefixes, increase the threshold to avoid nonsense
	if len(prefix) <= 2 || isRepetitive(prefix) {
		// CONSTANT: small words
		// DEBUG: im doing higher threshold since i have the biggest corpus rn
		minFrequencyThreshold = 60
	}

	suggestions := make([]Suggestion, 0)

	// Visit subtree and collect suggestions
	err := c.trie.VisitSubtree(patricia.Prefix(prefix), func(p patricia.Prefix, item patricia.Item) error {
		var freq int = 1 // Default frequency

		// Try different type conversions
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
			fmt.Printf("Unknown item type: %T for word %s\n", item, p)
		}

		// Skip the exact match of the prefix
		if string(p) == prefix {
			return nil
		}

		// Skip low frequency words (likely garbage or typos in corpus)
		if freq < minFrequencyThreshold {
			return nil
		}

		suggestions = append(suggestions, Suggestion{
			Word:      string(p),
			Frequency: freq,
		})
		return nil
	})
	if err != nil {
		fmt.Printf("Error visiting subtree: %v\n", err)
		return nil
	}

	// Sort suggestions by frequency (highest first)
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Frequency > suggestions[j].Frequency
	})

	// Limit results
	if len(suggestions) > limit && limit > 0 {
		suggestions = suggestions[:limit]
	}

	return suggestions
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
		return fmt.Errorf("failed to open binary dictionary file: %v", err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			fmt.Printf("Error closing file: %v\n", err)
		}
	}(file)

	// Read file stats to check if it's empty
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %v", err)
	}

	if fileInfo.Size() < 4 {
		return fmt.Errorf("file is too small to be valid binary dictionary")
	}

	// Create binary reader
	reader := bufio.NewReader(file)

	// Read the header (count of entries)
	var totalEntries int32
	if err := binary.Read(reader, binary.LittleEndian, &totalEntries); err != nil {
		return fmt.Errorf("failed to read dictionary header: %v", err)
	}

	fmt.Printf("Binary dictionary contains %d entries\n", totalEntries)
	count := 0

	for count < int(totalEntries) {
		// Read word length (2 bytes)
		var wordLen uint16
		if err := binary.Read(reader, binary.LittleEndian, &wordLen); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading word length: %v", err)
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
			return fmt.Errorf("error reading word: %v", err)
		}
		word := string(wordBytes)

		// Return the buffer to the pool
		wordBufferPool.Put(bufPtr)

		// Read frequency (4 bytes)
		var freq uint32
		if err := binary.Read(reader, binary.LittleEndian, &freq); err != nil {
			return fmt.Errorf("error reading frequency: %v", err)
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

	fmt.Printf("Loaded %d entries from %s\n", count, filename)
	return nil
}

// LoadAllBinaries loads all binary dictionaries from a directory
func (c *Completer) LoadAllBinaries(dirPath string) error {
	// Load unigrams
	unigramPath := dirPath + "/unigrams.bin"
	if _, err := os.Stat(unigramPath); err == nil {
		if err := c.LoadBinaryDictionary(unigramPath); err != nil {
			return fmt.Errorf("failed to load unigrams: %v", err)
		}
	}

	// Load bigrams
	bigramPath := dirPath + "/bigrams.bin"
	if _, err := os.Stat(bigramPath); err == nil {
		if err := c.LoadBinaryDictionary(bigramPath); err != nil {
			return fmt.Errorf("failed to load bigrams: %v", err)
		}
	}

	// Load trigrams
	trigramPath := dirPath + "/trigrams.bin"
	if _, err := os.Stat(trigramPath); err == nil {
		if err := c.LoadBinaryDictionary(trigramPath); err != nil {
			return fmt.Errorf("failed to load trigrams: %v", err)
		}
	}

	// Load word trie (if it exists and has a different format)
	wordTriePath := dirPath + "/word_trie.bin"
	if _, err := os.Stat(wordTriePath); err == nil {
		// Assuming the same format for now
		if err := c.LoadBinaryDictionary(wordTriePath); err != nil {
			return fmt.Errorf("failed to load word trie: %v", err)
		}
	}

	return nil
}

// LoadTextDictionary loads words from a text corpus file
func (c *Completer) LoadTextDictionary(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open dictionary file: %v", err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			fmt.Printf("Error closing file: %v\n", err)
		}
	}(file)

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		parts := strings.Split(line, "\t")
		word := parts[0]
		freq := lineNum // Default: use line number as inverse frequency

		// If frequency is provided, use that instead
		if len(parts) > 1 {
			if f, err := strconv.Atoi(parts[1]); err == nil {
				freq = f
			}
		}

		// Invert the frequency if using line numbers
		// (for corpus files where earlier words are more frequent)
		if len(parts) <= 1 {
			// This is an approximation - improve as needed for your specific corpus
			freq = 1000000 - freq
			freq = max(freq, 1)
		}

		c.AddWord(word, freq)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading dictionary file: %v", err)
	}

	return nil
}

// SaveBinaryDictionary exports the trie content to a binary file for persistence
func (c *Completer) SaveBinaryDictionary(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create binary export file: %v", err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			fmt.Printf("Error closing file: %v\n", err)
		}
	}(file)

	// Count entries first
	count := 0
	err = c.trie.Visit(func(prefix patricia.Prefix, item patricia.Item) error {
		count++
		return nil
	})
	if err != nil {
		return fmt.Errorf("error counting entries: %v", err)
	}

	writer := bufio.NewWriter(file)
	defer func(writer *bufio.Writer) {
		err := writer.Flush()
		if err != nil {
			fmt.Printf("Error flushing writer: %v\n", err)
		}
	}(writer)

	// Write header (count of entries)
	if err := binary.Write(writer, binary.LittleEndian, int32(count)); err != nil {
		return fmt.Errorf("error writing header: %v", err)
	}

	// Write entries
	err = c.trie.Visit(func(prefix patricia.Prefix, item patricia.Item) error {
		word := string(prefix)
		wordLen := uint16(len(word))

		// Write word length (2 bytes)
		if err := binary.Write(writer, binary.LittleEndian, wordLen); err != nil {
			return fmt.Errorf("error writing word length: %v", err)
		}

		// Write word
		if _, err := writer.WriteString(word); err != nil {
			return fmt.Errorf("error writing word: %v", err)
		}

		// Write frequency (4 bytes)
		freq := uint32(0)
		if f, ok := item.(int); ok {
			freq = uint32(f)
		}
		if err := binary.Write(writer, binary.LittleEndian, freq); err != nil {
			return fmt.Errorf("error writing frequency: %v", err)
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
