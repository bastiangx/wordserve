// Package dictionary handles loading, managing and validating  _data_ bin files and their formats/headers.
package dictionary

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/tchap/go-patricia/v2/patricia"
)

// Loader manages lazy loading of dictionary chunks
// It works with chunked binary files that contain words and their frequencies
// Each chunk is a separate file with a specific naming pattern (dict_0001.bin, dict_0002.bin, etc.)
// The loader supports lazy loading, unloading, and querying of words and their frequencies
// It uses a radix Patricia Trie for prefix searching and word frequency management
type Loader struct {
	chunkWords      map[int]map[string]int
	loadedChunks    map[int]bool
	errorCount      map[int]int
	wordFreqs       map[string]int
	availableChunks []ChunkInfo
	chunksCached    bool
	done            chan struct{}
	trie            *patricia.Trie
	mu              sync.RWMutex
	loadingCh       chan int
	dirPath         string
	maxWords        int
	totalWords      int
	maxFrequency    int
	maxRetries      int
}

// ChunkInfo contains metadata about a chunk file
type ChunkInfo struct {
	ID        int
	Filename  string
	WordCount int
}

// LoaderStats provides statistics about the loading process
type LoaderStats struct {
	TotalWords      int
	LoadedWords     int
	LoadedChunks    int
	AvailableChunks int
	MaxFrequency    int
	IsLoading       bool
}

// NewLoader creates a new default lazy loader
func NewLoader(dirPath string, maxWords int) *Loader {
	return &Loader{
		dirPath:      dirPath,
		maxWords:     maxWords,
		loadedChunks: make(map[int]bool),
		chunkWords:   make(map[int]map[string]int),
		trie:         patricia.NewTrie(),
		wordFreqs:    make(map[string]int),
		loadingCh:    make(chan int, 10),
		done:         make(chan struct{}),
		errorCount:   make(map[int]int),
		totalWords:   0,
		maxFrequency: 0,
		maxRetries:   3,
	}
}

// GetAvailable scans the directory for available chunk files
func (cl *Loader) GetAvailable() ([]ChunkInfo, error) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.chunksCached {
		return cl.availableChunks, nil
	}

	pattern := filepath.Join(cl.dirPath, "dict_*.bin")
	files, err := filepath.Glob(pattern)
	if err != nil {
		log.Errorf("failed to scan for chunk files: %v", err)
		return nil, err
	}

	var chunks []ChunkInfo
	for _, file := range files {
		basename := filepath.Base(file)
		// Extract ID from filename (dict_0001.bin -> 1)
		if strings.HasPrefix(basename, "dict_") && strings.HasSuffix(basename, ".bin") {
			idStr := strings.TrimPrefix(basename, "dict_")
			idStr = strings.TrimSuffix(idStr, ".bin")
			if chunkID, err := strconv.Atoi(idStr); err == nil {
				wordCount, err := cl.getWordCount(file)
				if err != nil {
					log.Warnf("Failed to get word count for block %s: %v", file, err)
					wordCount = 0
				}
				chunks = append(chunks, ChunkInfo{
					ID:        chunkID,
					Filename:  file,
					WordCount: wordCount,
				})
			}
		}
	}
	// Sort by ID
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].ID < chunks[j].ID
	})

	cl.availableChunks = chunks
	cl.chunksCached = true
	return chunks, nil
}

// getWordCount reads the word count from file's header
func (cl *Loader) getWordCount(filename string) (int, error) {
	file, err := os.Open(filename)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	var wordCount int32
	err = binary.Read(file, binary.LittleEndian, &wordCount)
	if err != nil {
		return 0, err
	}

	return int(wordCount), nil
}

// StartLoading begins the lazy loading process
func (cl *Loader) StartLoading() error {
	fl, err := cl.GetAvailable()
	if err != nil {
		log.Errorf("failed to get available files: %v", err)
		return err
	}

	if len(fl) == 0 {
		log.Errorf("no files found in %s", cl.dirPath)
		return err
	}
	log.Debugf("Found %d files", len(fl))

	go cl.backgroundLoader()

	// calc how many words to load based on maxWords limit
	wordsToLoad := cl.maxWords
	if wordsToLoad == 0 {
		for _, chunk := range fl {
			wordsToLoad += chunk.WordCount
		}
	}
	// Queue initial chunk to load
	loadedWords := 0
	for _, chunk := range fl {
		if loadedWords >= wordsToLoad {
			break
		}
		select {
		case cl.loadingCh <- chunk.ID:
			log.Debugf("Queued  %d for loading", chunk.ID)
		case <-time.After(100 * time.Millisecond):
			log.Warnf("Loading queue full")
		}
		loadedWords += chunk.WordCount
	}
	return nil
}

// backgroundLoader runs in a goroutine and loads blocks from the queue
func (cl *Loader) backgroundLoader() {
	for {
		select {
		case chunkID := <-cl.loadingCh:
			if err := cl.Load(chunkID); err != nil {
				log.Errorf("Failed to load chunk %d: %v", chunkID, err)
				cl.mu.Lock()
				cl.errorCount[chunkID]++
				errorCount := cl.errorCount[chunkID]
				cl.mu.Unlock()

				if errorCount < cl.maxRetries {
					log.Debugf("Retrying %d (attempt %d/%d)", chunkID, errorCount+1, cl.maxRetries)
					go func(id int) {
						time.Sleep(time.Duration(errorCount) * time.Second)
						select {
						case cl.loadingCh <- id:
						case <-cl.done:
						}
					}(chunkID)
				} else {
					log.Errorf("Loading %d failed %d times, aborting.", chunkID, cl.maxRetries)
				}
			} else {
				log.Debugf("Loaded dict file %d", chunkID)
			}
		case <-cl.done:
			return
		}
	}
}

// Load loads a specific chunk into memory
func (cl *Loader) Load(chunkID int) error {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if cl.loadedChunks[chunkID] {
		return nil
	}

	filename := filepath.Join(cl.dirPath, fmt.Sprintf("dict_%04d.bin", chunkID))

	file, err := os.Open(filename)
	if err != nil {
		log.Errorf("failed to open chunk file %s: %v", filename, err)
		return err
	}
	defer file.Close()
	reader := bufio.NewReader(file)

	// word count header
	var totalEntries int32
	if err := binary.Read(reader, binary.LittleEndian, &totalEntries); err != nil {
		log.Errorf("failed to read chunk header: %v", err)
		return err
	}
	count := 0
	for count < int(totalEntries) {
		var wordLen uint16
		if err := binary.Read(reader, binary.LittleEndian, &wordLen); err != nil {
			if err == io.EOF {
				break
			}
			log.Errorf("failed to read word length: %v", err)
			return err
		}
		wordBytes := make([]byte, wordLen)
		if _, err := io.ReadFull(reader, wordBytes); err != nil {
			log.Errorf("failed to read word: %v", err)
			return err
		}
		word := string(wordBytes)
		var rank uint16
		if err := binary.Read(reader, binary.LittleEndian, &rank); err != nil {
			log.Errorf("failed to read rank: %v", err)
			return err
		}

		// Convert rank to inverse score for sorting (rank 1 = highest score)
		// Use (max_uint16 + 1) - rank so rank 1 becomes 65535, rank 2 becomes 65534, etc.
		score := int(65535 - rank + 1)
		cl.trie.Insert(patricia.Prefix(word), score)
		cl.wordFreqs[word] = score

		if cl.chunkWords[chunkID] == nil {
			cl.chunkWords[chunkID] = make(map[string]int)
		}
		cl.chunkWords[chunkID][word] = score
		cl.totalWords++
		if score > cl.maxFrequency {
			cl.maxFrequency = score
		}
		count++
	}
	cl.loadedChunks[chunkID] = true
	log.Debugf("dict file %d loaded: %d words", chunkID, count)
	return nil
}

// Evict removes a specific chunk from memory
func (cl *Loader) Evict(chunkID int) error {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if !cl.loadedChunks[chunkID] {
		log.Errorf("%d is not loaded", chunkID)
		return errors.New("file not loaded")
	}
	log.Debugf("Unloading %d", chunkID)
	delete(cl.loadedChunks, chunkID)
	chunkWords, exists := cl.chunkWords[chunkID]

	if !exists {
		log.Errorf("%d word data not found", chunkID)
		return errors.New("file's word data not found")
	}

	for word := range chunkWords {
		delete(cl.wordFreqs, word)
		cl.totalWords--
	}
	delete(cl.chunkWords, chunkID)
	cl.rebuildTrie()
	log.Debugf("Successfully unloaded %d", chunkID)
	return nil
}

// rebuildTrie reconstructs the trie from currently loaded chunks
func (cl *Loader) rebuildTrie() {
	cl.trie = patricia.NewTrie()
	cl.maxFrequency = 0

	for chunkID, loaded := range cl.loadedChunks {
		if !loaded {
			continue
		}
		chunkWords, exists := cl.chunkWords[chunkID]
		if !exists {
			continue
		}
		for word, freq := range chunkWords {
			cl.trie.Insert(patricia.Prefix(word), freq)
			if freq > cl.maxFrequency {
				cl.maxFrequency = freq
			}
		}
	}

	log.Debugf("Trie rebuilt with %d loaded chunks", len(cl.loadedChunks))
}

// GetTrie returns the loaded trie
func (cl *Loader) GetTrie() *patricia.Trie {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.trie
}

// GetWordFreqs returns the word frequency map
func (cl *Loader) GetWordFreqs() map[string]int {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	freqs := make(map[string]int, len(cl.wordFreqs))
	maps.Copy(freqs, cl.wordFreqs)
	return freqs
}

// GetStats returns current loading statistics
func (cl *Loader) GetStats() LoaderStats {
	cl.mu.RLock()

	var availableChunks int
	if cl.chunksCached {
		availableChunks = len(cl.availableChunks)
	} else {
		cl.mu.RUnlock()
		chunks, _ := cl.GetAvailable()
		availableChunks = len(chunks)
		cl.mu.RLock()
	}

	loadedChunks := len(cl.loadedChunks)
	stats := LoaderStats{
		TotalWords:      cl.totalWords,
		LoadedWords:     cl.totalWords,
		LoadedChunks:    loadedChunks,
		AvailableChunks: availableChunks,
		MaxFrequency:    cl.maxFrequency,
		IsLoading:       len(cl.loadingCh) > 0,
	}

	cl.mu.RUnlock()
	return stats
}

// Stop kills the background loading process
func (cl *Loader) Stop() {
	close(cl.done)
}

// RequestMore queues additional files for loading
func (cl *Loader) RequestMore(additionalWords int) error {
	chunks, err := cl.GetAvailable()
	if err != nil {
		return err
	}
	wordsToLoad := 0
	for _, chunk := range chunks {
		cl.mu.RLock()
		alreadyLoaded := cl.loadedChunks[chunk.ID]
		cl.mu.RUnlock()

		if !alreadyLoaded {
			select {
			case cl.loadingCh <- chunk.ID:
				log.Debugf("Queued additional %d for loading", chunk.ID)
				wordsToLoad += chunk.WordCount
				if wordsToLoad >= additionalWords {
					break
				}
			default:
				log.Warnf("Loading queue full, cannot queue %d", chunk.ID)
			}
		}
	}
	return nil
}

// GetLoadedIDs returns a slice of currently loaded chunk IDs
func (cl *Loader) GetLoadedIDs() []int {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	var loadedIDs []int
	for chunkID, loaded := range cl.loadedChunks {
		if loaded {
			loadedIDs = append(loadedIDs, chunkID)
		}
	}
	sort.Ints(loadedIDs)
	return loadedIDs
}
