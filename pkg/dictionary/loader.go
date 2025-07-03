package dictionary

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
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

// ChunkLoader manages lazy loading of dictionary chunks
type ChunkLoader struct {
	dirPath      string
	chunkSize    int
	maxWords     int
	loadedChunks map[int]bool
	chunkWords   map[int]map[string]int // Track which words belong to which chunk
	trie         *patricia.Trie
	wordFreqs    map[string]int
	totalWords   int
	maxFrequency int
	mu           sync.RWMutex
	loadingCh    chan int
	done         chan struct{}
	errorCount   map[int]int
	maxRetries   int
}

// ChunkInfo contains metadata about a chunk file
type ChunkInfo struct {
	ChunkID   int
	Filename  string
	WordCount int
	Exists    bool
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

// NewChunkLoader creates a new lazy chunk loader
func NewChunkLoader(dirPath string, chunkSize, maxWords int) *ChunkLoader {
	return &ChunkLoader{
		dirPath:      dirPath,
		chunkSize:    chunkSize,
		maxWords:     maxWords,
		loadedChunks: make(map[int]bool),
		chunkWords:   make(map[int]map[string]int),
		trie:         patricia.NewTrie(),
		wordFreqs:    make(map[string]int),
		totalWords:   0,
		maxFrequency: 0,
		loadingCh:    make(chan int, 10),
		done:         make(chan struct{}),
		errorCount:   make(map[int]int),
		maxRetries:   3,
	}
}

// GetAvailableChunks scans the directory for available chunk files
func (cl *ChunkLoader) GetAvailableChunks() ([]ChunkInfo, error) {
	pattern := filepath.Join(cl.dirPath, "dict_*.bin")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to scan for chunk files: %w", err)
	}

	var chunks []ChunkInfo
	for _, file := range files {
		basename := filepath.Base(file)
		// Extract chunk ID from filename (dict_0001.bin -> 1)
		if strings.HasPrefix(basename, "dict_") && strings.HasSuffix(basename, ".bin") {
			idStr := strings.TrimPrefix(basename, "dict_")
			idStr = strings.TrimSuffix(idStr, ".bin")
			if chunkID, err := strconv.Atoi(idStr); err == nil {
				// Get word count from file
				wordCount, err := cl.getChunkWordCount(file)
				if err != nil {
					log.Warnf("Failed to get word count for chunk %s: %v", file, err)
					wordCount = 0
				}
				chunks = append(chunks, ChunkInfo{
					ChunkID:   chunkID,
					Filename:  file,
					WordCount: wordCount,
					Exists:    true,
				})
			}
		}
	}

	// Sort chunks by ID
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].ChunkID < chunks[j].ChunkID
	})

	return chunks, nil
}

// getChunkWordCount reads the word count from a chunk file's header
func (cl *ChunkLoader) getChunkWordCount(filename string) (int, error) {
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

// StartLazyLoading begins the lazy loading process
func (cl *ChunkLoader) StartLazyLoading() error {
	chunks, err := cl.GetAvailableChunks()
	if err != nil {
		return fmt.Errorf("failed to get available chunks: %w", err)
	}

	if len(chunks) == 0 {
		return fmt.Errorf("no chunk files found in %s", cl.dirPath)
	}

	log.Debugf("Found %d chunk files", len(chunks))

	// Start background loader goroutine
	go cl.backgroundLoader()

	// Calculate how many words to load based on maxWords limit
	wordsToLoad := cl.maxWords
	if wordsToLoad == 0 {
		// Load all available words
		for _, chunk := range chunks {
			wordsToLoad += chunk.WordCount
		}
	}

	// Queue initial chunks for loading
	loadedWords := 0
	for _, chunk := range chunks {
		if loadedWords >= wordsToLoad {
			break
		}

		select {
		case cl.loadingCh <- chunk.ChunkID:
			log.Debugf("Queued chunk %d for loading", chunk.ChunkID)
		case <-time.After(100 * time.Millisecond):
			log.Warnf("Loading queue full, chunk %d will be loaded later", chunk.ChunkID)
		}

		loadedWords += chunk.WordCount
	}

	return nil
}

// backgroundLoader runs in a goroutine and loads chunks from the queue
func (cl *ChunkLoader) backgroundLoader() {
	for {
		select {
		case chunkID := <-cl.loadingCh:
			if err := cl.loadChunk(chunkID); err != nil {
				log.Errorf("Failed to load chunk %d: %v", chunkID, err)

				// Retry logic
				cl.mu.Lock()
				cl.errorCount[chunkID]++
				errorCount := cl.errorCount[chunkID]
				cl.mu.Unlock()

				if errorCount < cl.maxRetries {
					log.Debugf("Retrying chunk %d (attempt %d/%d)", chunkID, errorCount+1, cl.maxRetries)
					// Retry after a short delay
					go func(id int) {
						time.Sleep(time.Duration(errorCount) * time.Second)
						select {
						case cl.loadingCh <- id:
						case <-cl.done:
						}
					}(chunkID)
				} else {
					log.Errorf("Chunk %d failed %d times, giving up", chunkID, cl.maxRetries)
				}
			} else {
				log.Debugf("Successfully loaded chunk %d", chunkID)
			}
		case <-cl.done:
			return
		}
	}
}

// loadChunk loads a specific chunk into memory
func (cl *ChunkLoader) loadChunk(chunkID int) error {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.loadedChunks[chunkID] {
		return nil // Already loaded
	}

	filename := filepath.Join(cl.dirPath, fmt.Sprintf("dict_%04d.bin", chunkID))
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open chunk file %s: %w", filename, err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)

	// Read word count header
	var totalEntries int32
	if err := binary.Read(reader, binary.LittleEndian, &totalEntries); err != nil {
		return fmt.Errorf("failed to read chunk header: %w", err)
	}

	log.Debugf("Loading chunk %d with %d words", chunkID, totalEntries)

	// Load words from this chunk
	count := 0
	for count < int(totalEntries) {
		// Read word length
		var wordLen uint16
		if err := binary.Read(reader, binary.LittleEndian, &wordLen); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to read word length: %w", err)
		}

		// Read word
		wordBytes := make([]byte, wordLen)
		if _, err := io.ReadFull(reader, wordBytes); err != nil {
			return fmt.Errorf("failed to read word: %w", err)
		}
		word := string(wordBytes)

		// Read rank (2 bytes instead of 4 bytes for frequency)
		var rank uint16
		if err := binary.Read(reader, binary.LittleEndian, &rank); err != nil {
			return fmt.Errorf("failed to read rank: %w", err)
		}

		// Convert rank to inverse score for sorting (rank 1 = highest score)
		// Use (max_uint16 + 1) - rank so rank 1 becomes 65535, rank 2 becomes 65534, etc.
		score := int(65535 - rank + 1)

		// Add to trie and frequency map
		cl.trie.Insert(patricia.Prefix(word), score)
		cl.wordFreqs[word] = score

		// Track which chunk this word belongs to
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
	log.Debugf("Chunk %d loaded: %d words", chunkID, count)
	return nil
}

// UnloadChunk removes a specific chunk from memory
func (cl *ChunkLoader) UnloadChunk(chunkID int) error {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	// Check if chunk is loaded
	if !cl.loadedChunks[chunkID] {
		return fmt.Errorf("chunk %d is not loaded", chunkID)
	}

	log.Debugf("Unloading chunk %d", chunkID)

	// Remove from loaded chunks
	delete(cl.loadedChunks, chunkID)

	// Remove words from trie and word frequencies
	chunkWords, exists := cl.chunkWords[chunkID]
	if !exists {
		return fmt.Errorf("chunk %d word data not found", chunkID)
	}

	// Remove words from word frequencies map
	for word := range chunkWords {
		delete(cl.wordFreqs, word)
		cl.totalWords--
	}

	// Remove chunk word tracking
	delete(cl.chunkWords, chunkID)

	// Rebuild the trie without the unloaded chunk
	cl.rebuildTrie()

	log.Debugf("Successfully unloaded chunk %d", chunkID)
	return nil
}

// rebuildTrie reconstructs the trie from currently loaded chunks
func (cl *ChunkLoader) rebuildTrie() {
	// Store reference to old trie for proper cleanup
	oldTrie := cl.trie
	// Create new trie
	cl.trie = patricia.NewTrie()

	// Recalculate max frequency
	cl.maxFrequency = 0

	// Add words from all loaded chunks
	for chunkID, loaded := range cl.loadedChunks {
		if !loaded {
			continue
		}

		chunkWords, exists := cl.chunkWords[chunkID]
		if !exists {
			continue
		}

		// Add all words from this chunk to the trie
		for word, freq := range chunkWords {
			cl.trie.Insert(patricia.Prefix(word), freq)
			if freq > cl.maxFrequency {
				cl.maxFrequency = freq
			}
		}
	}

	// Help GC clean up old trie by nulling the reference
	_ = oldTrie
	oldTrie = nil

	log.Debugf("Trie rebuilt with %d loaded chunks", len(cl.loadedChunks))
}

// GetTrie returns the loaded trie (thread-safe)
func (cl *ChunkLoader) GetTrie() *patricia.Trie {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.trie
}

// GetWordFreqs returns the word frequency map (thread-safe)
func (cl *ChunkLoader) GetWordFreqs() map[string]int {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	// Return a copy to avoid race conditions
	freqs := make(map[string]int, len(cl.wordFreqs))
	for k, v := range cl.wordFreqs {
		freqs[k] = v
	}
	return freqs
}

// GetStats returns current loading statistics
func (cl *ChunkLoader) GetStats() LoaderStats {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	chunks, _ := cl.GetAvailableChunks()
	availableChunks := len(chunks)
	loadedChunks := len(cl.loadedChunks)

	return LoaderStats{
		TotalWords:      cl.totalWords,
		LoadedWords:     cl.totalWords,
		LoadedChunks:    loadedChunks,
		AvailableChunks: availableChunks,
		MaxFrequency:    cl.maxFrequency,
		IsLoading:       len(cl.loadingCh) > 0,
	}
}

// Stop stops the background loading process
func (cl *ChunkLoader) Stop() {
	close(cl.done)
}

// RequestMoreChunks queues additional chunks for loading
func (cl *ChunkLoader) RequestMoreChunks(additionalWords int) error {
	chunks, err := cl.GetAvailableChunks()
	if err != nil {
		return err
	}

	wordsToLoad := 0
	for _, chunk := range chunks {
		cl.mu.RLock()
		alreadyLoaded := cl.loadedChunks[chunk.ChunkID]
		cl.mu.RUnlock()

		if !alreadyLoaded {
			select {
			case cl.loadingCh <- chunk.ChunkID:
				log.Debugf("Queued additional chunk %d for loading", chunk.ChunkID)
				wordsToLoad += chunk.WordCount
				if wordsToLoad >= additionalWords {
					break
				}
			default:
				log.Warnf("Loading queue full, cannot queue chunk %d", chunk.ChunkID)
			}
		}
	}

	return nil
}

// LoadSpecificChunk loads a specific chunk by ID
func (cl *ChunkLoader) LoadSpecificChunk(chunkID int) error {
	cl.mu.RLock()
	alreadyLoaded := cl.loadedChunks[chunkID]
	cl.mu.RUnlock()

	if alreadyLoaded {
		return nil // Already loaded
	}

	return cl.loadChunk(chunkID)
}

// GetLoadedChunkIDs returns a slice of currently loaded chunk IDs
func (cl *ChunkLoader) GetLoadedChunkIDs() []int {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	var loadedIDs []int
	for chunkID, loaded := range cl.loadedChunks {
		if loaded {
			loadedIDs = append(loadedIDs, chunkID)
		}
	}

	// Sort for consistent ordering
	sort.Ints(loadedIDs)
	return loadedIDs
}

// GetAvailableChunkCount returns the total number of available chunk files
func (cl *ChunkLoader) GetAvailableChunkCount() (int, error) {
	chunks, err := cl.GetAvailableChunks()
	if err != nil {
		return 0, err
	}
	return len(chunks), nil
}
