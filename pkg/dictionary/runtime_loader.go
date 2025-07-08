package dictionary

import (
	"fmt"
	"sort"
	"sync"

	"github.com/charmbracelet/log"
)

// RuntimeLoader manages dynamic loading/unloading of dictionary chunks during runtime
type RuntimeLoader struct {
	chunkLoader  *Loader
	targetChunks int
	mu           sync.RWMutex
}

// NewRuntimeLoader creates a new runtime loader
func NewRuntimeLoader(chunkLoader *Loader) *RuntimeLoader {
	return &RuntimeLoader{
		chunkLoader:  chunkLoader,
		targetChunks: 0,
	}
}

// GetAvailableChunkCount returns the total number of available chunk files
func (rl *RuntimeLoader) GetAvailableChunkCount() (int, error) {
	chunks, err := rl.chunkLoader.GetAvailable()
	if err != nil {
		return 0, err
	}
	return len(chunks), nil
}

// GetMaxWordsAvailable returns the maximum number of words that can be loaded
func (rl *RuntimeLoader) GetMaxWordsAvailable() (int, error) {
	chunks, err := rl.chunkLoader.GetAvailable()
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
// Automatically generates required chunks if not enough are available
func (rl *RuntimeLoader) SetDictionarySize(targetChunks int) error {
	if targetChunks < 1 {
		return fmt.Errorf("minimum dictionary size is 1 chunk")
	}

	// Check if we have enough chunks
	if !rl.chunkLoader.checkDictNum(targetChunks) {
		log.Infof("Insufficient chunks available. Generating missing chunks for a total of %d.", targetChunks)
		if err := rl.chunkLoader.checkChunkCount(targetChunks); err != nil {
			return fmt.Errorf("failed to generate required chunks: %w", err)
		}
	} else {
		log.Debugf("Sufficient chunks available for %d requirements.", targetChunks)
	}

	currentStats := rl.chunkLoader.GetStats()
	currentChunks := currentStats.LoadedChunks

	log.Debugf("Setting dictionary size: current=%d chunks, target=%d chunks", currentChunks, targetChunks)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	if targetChunks > currentChunks {
		err := rl.loadAdditionalChunks(targetChunks - currentChunks)
		if err != nil {
			return err
		}
	} else if targetChunks < currentChunks {
		err := rl.unloadExcessChunks(currentChunks - targetChunks)
		if err != nil {
			return err
		}
	}
	rl.targetChunks = targetChunks
	return nil
}

// loadAdditionalChunks loads the specified number of additional chunks
func (rl *RuntimeLoader) loadAdditionalChunks(additionalChunks int) error {
	chunks, err := rl.chunkLoader.GetAvailable()
	if err != nil {
		return err
	}
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].ID < chunks[j].ID
	})
	currentStats := rl.chunkLoader.GetStats()
	currentChunks := currentStats.LoadedChunks
	targetTotal := currentChunks + additionalChunks

	loadedCount := 0
	for _, chunk := range chunks {
		if loadedCount >= additionalChunks {
			break
		}
		if err := rl.chunkLoader.Load(chunk.ID); err != nil {
			log.Warnf("Failed to load chunk %d: %v", chunk.ID, err)
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
	loadedChunkIDs := rl.chunkLoader.GetLoadedIDs()
	if len(loadedChunkIDs) == 0 {
		return nil
	}
	sort.Sort(sort.Reverse(sort.IntSlice(loadedChunkIDs)))
	unloadedCount := 0
	for _, chunkID := range loadedChunkIDs {
		if unloadedCount >= excessChunks {
			break
		}
		if err := rl.chunkLoader.Evict(chunkID); err != nil {
			log.Warnf("Failed to unload chunk %d: %v", chunkID, err)
			continue
		}
		unloadedCount++
	}
	rl.targetChunks -= excessChunks
	log.Debugf("Unloaded %d chunks", unloadedCount)
	return nil
}

// GetDictionarySizeOptions returns the available dictionary size options
// Returns array of chunk counts and their corresponding word counts
func (rl *RuntimeLoader) GetDictionarySizeOptions() ([]DictionarySizeOption, error) {
	chunks, err := rl.chunkLoader.GetAvailable()
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
