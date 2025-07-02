package dictionary

import (
	"fmt"
	"sort"
	"sync"

	"github.com/charmbracelet/log"
)

// RuntimeLoader manages dynamic loading/unloading of dictionary chunks during runtime
type RuntimeLoader struct {
	chunkLoader     *ChunkLoader
	targetChunks    int
	availableChunks []ChunkInfo
	mu              sync.RWMutex
}

// NewRuntimeLoader creates a new runtime loader
func NewRuntimeLoader(chunkLoader *ChunkLoader) *RuntimeLoader {
	return &RuntimeLoader{
		chunkLoader:  chunkLoader,
		targetChunks: 5, // Default to 5 chunks (50K words)
	}
}

// GetAvailableChunkCount returns the total number of available chunk files
func (rl *RuntimeLoader) GetAvailableChunkCount() (int, error) {
	chunks, err := rl.chunkLoader.GetAvailableChunks()
	if err != nil {
		return 0, err
	}

	rl.mu.Lock()
	rl.availableChunks = chunks
	rl.mu.Unlock()

	return len(chunks), nil
}

// GetMaxWordsAvailable returns the maximum number of words that can be loaded
func (rl *RuntimeLoader) GetMaxWordsAvailable() (int, error) {
	chunks, err := rl.chunkLoader.GetAvailableChunks()
	if err != nil {
		return 0, err
	}

	totalWords := 0
	for _, chunk := range chunks {
		totalWords += chunk.WordCount
	}

	return totalWords, nil
}

// SetDictionarySize updates the dictionary to load the specified number of chunks
// chunks should be between 1 and the maximum available chunks
func (rl *RuntimeLoader) SetDictionarySize(targetChunks int) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if targetChunks < 1 {
		return fmt.Errorf("minimum dictionary size is 1 chunk (10K words)")
	}

	chunks, err := rl.chunkLoader.GetAvailableChunks()
	if err != nil {
		return fmt.Errorf("failed to get available chunks: %w", err)
	}

	if targetChunks > len(chunks) {
		return fmt.Errorf("requested %d chunks but only %d are available", targetChunks, len(chunks))
	}

	currentStats := rl.chunkLoader.GetStats()
	currentChunks := currentStats.LoadedChunks

	log.Debugf("Setting dictionary size: current=%d chunks, target=%d chunks", currentChunks, targetChunks)

	if targetChunks > currentChunks {
		// Load more chunks
		return rl.loadAdditionalChunks(targetChunks - currentChunks)
	} else if targetChunks < currentChunks {
		// Unload excess chunks
		return rl.unloadExcessChunks(currentChunks - targetChunks)
	}

	// Already at target size
	rl.targetChunks = targetChunks
	return nil
}

// loadAdditionalChunks loads the specified number of additional chunks
func (rl *RuntimeLoader) loadAdditionalChunks(additionalChunks int) error {
	chunks, err := rl.chunkLoader.GetAvailableChunks()
	if err != nil {
		return err
	}

	// Sort chunks by ID to load in order
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].ChunkID < chunks[j].ChunkID
	})

	// Find next unloaded chunks to load
	currentStats := rl.chunkLoader.GetStats()
	currentChunks := currentStats.LoadedChunks
	targetTotal := currentChunks + additionalChunks

	loadedCount := 0
	for _, chunk := range chunks {
		if loadedCount >= additionalChunks {
			break
		}

		// Check if this chunk is already loaded
		if err := rl.chunkLoader.LoadSpecificChunk(chunk.ChunkID); err != nil {
			log.Warnf("Failed to load chunk %d: %v", chunk.ChunkID, err)
			continue
		}

		loadedCount++
	}

	rl.targetChunks = targetTotal
	log.Debugf("Loaded %d additional chunks", loadedCount)
	return nil
}

// unloadExcessChunks unloads the specified number of chunks from the highest numbers first
func (rl *RuntimeLoader) unloadExcessChunks(excessChunks int) error {
	// Get currently loaded chunk IDs
	loadedChunkIDs := rl.chunkLoader.GetLoadedChunkIDs()

	if len(loadedChunkIDs) == 0 {
		return nil // Nothing to unload
	}

	// Sort loaded chunks by ID in descending order (highest first)
	sort.Sort(sort.Reverse(sort.IntSlice(loadedChunkIDs)))

	// Unload chunks starting from the highest IDs
	unloadedCount := 0
	for _, chunkID := range loadedChunkIDs {
		if unloadedCount >= excessChunks {
			break
		}

		if err := rl.unloadChunk(chunkID); err != nil {
			log.Warnf("Failed to unload chunk %d: %v", chunkID, err)
			continue
		}

		unloadedCount++
	}

	rl.targetChunks -= excessChunks
	log.Debugf("Unloaded %d chunks", unloadedCount)
	return nil
}

// unloadChunk removes a specific chunk from memory
func (rl *RuntimeLoader) unloadChunk(chunkID int) error {
	return rl.chunkLoader.UnloadChunk(chunkID)
}

// GetCurrentDictionaryInfo returns information about the currently loaded dictionary
func (rl *RuntimeLoader) GetCurrentDictionaryInfo() (int, int, error) {
	stats := rl.chunkLoader.GetStats()
	chunks, err := rl.chunkLoader.GetAvailableChunks()
	if err != nil {
		return 0, 0, err
	}

	return stats.LoadedChunks, len(chunks), nil
}

// GetDictionarySizeOptions returns the available dictionary size options
// Returns array of chunk counts and their corresponding word counts
func (rl *RuntimeLoader) GetDictionarySizeOptions() ([]DictionarySizeOption, error) {
	chunks, err := rl.chunkLoader.GetAvailableChunks()
	if err != nil {
		return nil, err
	}

	options := make([]DictionarySizeOption, 0, len(chunks))
	totalWords := 0

	for i, chunk := range chunks {
		totalWords += chunk.WordCount
		options = append(options, DictionarySizeOption{
			ChunkCount: i + 1,
			WordCount:  totalWords,
			SizeLabel:  fmt.Sprintf("%dK words", totalWords/1000),
		})
	}

	return options, nil
}

// DictionarySizeOption represents a dictionary size option
type DictionarySizeOption struct {
	ChunkCount int    `json:"chunkCount"`
	WordCount  int    `json:"wordCount"`
	SizeLabel  string `json:"sizeLabel"`
}
