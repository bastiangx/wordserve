/*
Package dictionary manages chunked binary dictionary files with lazy loading and runtime memory management.

The dictionary package provides infrastructure for handling large word frequency datasets through a chunked file system. Words are stored in binary files with a specific format:
>> Each chunk contains a header with word count followed by word entries with their frequency rankings. The package supports both validation of file formats and dynamic loading/unloading of chunks during runtime.

Core functionality revolves around the Loader type, which manages concurrent access to multiple dictionary chunks. Each chunk file follows the naming pattern

	dict_XXXX.bin
	(dict_0001.bin, dict_0002.bin, etc)

and contains a subset of the total dictionary. Enables applications to load only the most relevant words based on frequency ranking, and mem usage controlled.

The binary format stores words with their rank values rather than raw frequencies. During init, words are ranked by frequency

	(rank 1 = most freq)

and stored as uint16 values.

The loader converts these ranks back to frequency scores using the formula:

	score = 65535 - rank + 1

higher freq words receive higher scores for sorting.

# Chunk

The loader operates with a goroutine that processes loading requests from a buffered channel. Prevents blocking the main thread.
Error handling includes automatic retry with exponential backoff for failed chunk loads.

	loader := dictionary.NewLoader("data/", 50000)
	err := loader.StartLoading()
	trie := loader.GetTrie()

# Runtime

RuntimeLoader gives control over loaded dictionary size during execution.
Works with the base Loader to add or remove chunks based on target word counts or chunk counts.

	runtimeLoader := dictionary.NewRuntimeLoader(loader)
	err := runtimeLoader.SetDictionarySize(3)
	options, err := runtimeLoader.GetDictionarySizeOptions()
*/
package dictionary

import (
	"archive/zip"
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bastiangx/wordserve/internal/utils"
	"github.com/bastiangx/wordserve/pkg/config"
	"github.com/charmbracelet/log"
	"github.com/tchap/go-patricia/v2/patricia"
)

const (
	// GHReleaseURL is the placeholder URL for downloading pre-built dictionary files
	GHReleaseURL = "https://github.com/bastiangx/wordserve/releases/latest/download"
	// MaxRetries for luajit script execution
	MaxRetries = 3
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

	if err := cl.checkDictFiles(); err != nil {
		return nil, err
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

// checkDictFiles checks if enough dictionary files exist, creates them if not
func (cl *Loader) checkDictFiles() error {
	if err := cl.checkWordFile(); err != nil {
		return err
	}
	if cl.checkDictNum() {
		return nil
	}
	log.Info("not enough dictionary files found, attempting to generate them...")
	if err := cl.buildLocalDict(); err != nil {
		log.Warnf("Local generation failed: %v", err)
		if err := cl.dlReleaseDict(); err != nil {
			log.Errorf("Remote download failed: %v", err)
			cl.logInitError()
			return err
		}
	}
	return nil
}

// checkWordFile checks for the existence of words.txt and downloads it if needed
func (cl *Loader) checkWordFile() error {
	wordsPath := filepath.Join(cl.dirPath, "words.txt")
	if _, err := os.Stat(wordsPath); os.IsNotExist(err) {
		log.Info("words.txt not found, attempting to download...")
		url := GHReleaseURL + "/words.txt"
		if err := cl.dlFile(url, wordsPath); err != nil {
			log.Errorf("Failed to download words.txt: %v", err)
			return fmt.Errorf("failed to download words.txt: %w", err)
		}
		log.Infof("Successfully downloaded words.txt")
	}
	return nil
}

// findScriptPath attempts to locate the build-data.lua
func (cl *Loader) findScriptPath() (string, error) {
	possiblePaths := []string{
		filepath.Join("scripts", "build-data.lua"),
		filepath.Join("..", "scripts", "build-data.lua"),
	}
	if execDir, err := utils.GetExecutableDir(); err == nil {
		projectRoot := filepath.Dir(execDir)
		if filepath.Base(execDir) == "cmd" || filepath.Base(execDir) == "bin" {
			projectRoot = filepath.Dir(execDir)
		}
		possiblePaths = append(possiblePaths, []string{
			filepath.Join(execDir, "scripts", "build-data.lua"),
			filepath.Join(projectRoot, "scripts", "build-data.lua"),
			filepath.Join(filepath.Dir(projectRoot), "scripts", "build-data.lua"),
		}...)
	}
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			log.Debugf("Found script at: %s", path)
			return path, nil
		}
	}
	return "", fmt.Errorf("build-data.lua not found in any of the expected locations: %v", possiblePaths)
}

// checkDictNum checks if the needed number of .bin files exist
// If requiredChunks is 0, uses config
func (cl *Loader) checkDictNum(requiredChunks ...int) bool {
	neededChunks := 0
	if len(requiredChunks) > 0 && requiredChunks[0] > 0 {
		neededChunks = requiredChunks[0]
	} else {
		cfg, _, err := config.LoadConfigWithPriority("")
		if err != nil {
			log.Warnf("Failed to load config, using defaults: %v", err)
			cfg = config.DefaultConfig()
		}
		neededChunks = cl.computeChunkAmount(cfg)
	}

	pattern := filepath.Join(cl.dirPath, "dict_*.bin")
	existingFiles, err := filepath.Glob(pattern)
	if err != nil {
		log.Errorf("Failed to check existing files: %v", err)
		return false
	}
	log.Debugf("Found %d existing files, need %d chunks", len(existingFiles), neededChunks)
	return len(existingFiles) >= neededChunks
}

// computeChunkAmount determines how many bin files are needed based on config
func (cl *Loader) computeChunkAmount(cfg *config.Config) int {
	if cfg.Dict.ChunkSize <= 0 || cfg.Dict.MaxWords <= 0 {
		return 1
	}
	return (cfg.Dict.MaxWords + cfg.Dict.ChunkSize - 1) / cfg.Dict.ChunkSize
}

// buildLocalDict attempts to run the luajit script to generate dictionary files
func (cl *Loader) buildLocalDict() error {
	return cl.buildLocalDictWithConfig(nil)
}

// buildLocalDictWithConfig attempts to run the luajit script with specific config
func (cl *Loader) buildLocalDictWithConfig(cfg *config.Config) error {
	if _, err := exec.LookPath("luajit"); err != nil {
		return errors.New("luajit not found in PATH")
	}
	if cfg == nil {
		var err error
		cfg, _, err = config.LoadConfigWithPriority("")
		if err != nil {
			log.Warnf("Failed to load config, using defaults: %v", err)
			cfg = config.DefaultConfig()
		}
	}
	scriptPath, err := cl.findScriptPath()
	if err != nil {
		return fmt.Errorf("luajit script not found: %w", err)
	}
	maxChunks := cl.computeChunkAmount(cfg)
	args := []string{
		scriptPath,
		"--chunk-size", fmt.Sprintf("%d", cfg.Dict.ChunkSize),
		"--max-chunks", fmt.Sprintf("%d", maxChunks),
	}
	for attempt := 1; attempt <= MaxRetries; attempt++ {
		log.Infof("Running luajit script (attempt %d/%d)...", attempt, MaxRetries)
		cmd := exec.Command("luajit", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			log.Errorf("Luajit script failed (attempt %d): %v", attempt, err)
			if attempt < MaxRetries {
				time.Sleep(time.Duration(attempt) * time.Second)
				continue
			}
			return fmt.Errorf("luajit script failed after %d attempts: %w", MaxRetries, err)
		}
		log.Info("Dictionary files generated successfully")
		return nil
	}
	return errors.New("should not reach here")
}

// dlReleaseDict downloads dict files from GitHub release
func (cl *Loader) dlReleaseDict() error {
	return cl.dlReleaseDictWithConfig(nil)
}

// dlReleaseDictWithConfig downloads and extracts data.zip with dictionary files
func (cl *Loader) dlReleaseDictWithConfig(cfg *config.Config) error {
	log.Info("Attempting to download pre-built dictionary files...")

	// Download data.zip (config not needed since we download the full package)
	zipURL := fmt.Sprintf("%s/data.zip", GHReleaseURL)
	zipPath := filepath.Join(cl.dirPath, "data.zip")

	log.Infof("Downloading data.zip from %s", zipURL)
	if err := cl.dlFile(zipURL, zipPath); err != nil {
		return fmt.Errorf("failed to download data.zip: %w", err)
	}

	// Extract the zip file
	if err := cl.extractZip(zipPath, cl.dirPath); err != nil {
		return fmt.Errorf("failed to extract data.zip: %w", err)
	}

	// Clean up the zip file
	if err := os.Remove(zipPath); err != nil {
		log.Warnf("Failed to remove data.zip: %v", err)
	}

	log.Info("Successfully downloaded and extracted dictionary files")
	return nil
}

// dlFile downloads a file from a URL to a local path
func (cl *Loader) dlFile(url, localPath string) error {
	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}
	file, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	return err
}

// extractZip extracts a zip file to a destination directory
func (cl *Loader) extractZip(zipPath, destDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		log.Errorf("failed to open zip file: %v", err)
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		if strings.Contains(file.Name, "..") {
			log.Warnf("Skipping potentially dangerous file path: %s", file.Name)
			continue
		}

		// Only extract .bin files
		if !strings.HasSuffix(file.Name, ".bin") {
			continue
		}

		filePath := filepath.Join(destDir, filepath.Base(file.Name))
		log.Debugf("Extracting %s to %s", file.Name, filePath)

		rc, err := file.Open()
		if err != nil {
			log.Errorf("failed to open file in zip: %v", err)
			return err
		}

		outFile, err := os.Create(filePath)
		if err != nil {
			rc.Close()
			log.Errorf("failed to create output file: %v", err)
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			log.Errorf("failed to extract file: %v", err)
			return err
		}
	}

	log.Infof("Successfully extracted dictionary files")
	return nil
}

// checkChunkCount checks if the needed number of chunks exists
func (cl *Loader) checkChunkCount(rc int) error {
	if cl.checkDictNum(rc) {
		return nil
	}
	cfg, _, err := config.LoadConfigWithPriority("")
	if err != nil {
		log.Warnf("Failed to load config, using defaults: %v", err)
		cfg = config.DefaultConfig()
	}

	originalMaxWords := cfg.Dict.MaxWords
	cfg.Dict.MaxWords = rc * cfg.Dict.ChunkSize

	if err := cl.buildLocalDictWithConfig(cfg); err != nil {
		log.Warnf("Local generation failed: %v", err)
		if err := cl.dlReleaseDictWithConfig(cfg); err != nil {
			log.Errorf("Remote download failed: %v", err)
			cl.logInitError()
			return err
		}
	}
	cfg.Dict.MaxWords = originalMaxWords
	cl.mu.Lock()
	cl.chunksCached = false
	cl.availableChunks = nil
	cl.mu.Unlock()

	return nil
}

// logInitError logs a fatal with user guide
func (cl *Loader) logInitError() {
	log.Fatal(`
Failed to initialize dictionary files!
WordServe could not create or download the required data files.

To resolve this issue, you can:

1. Run the LuaJIT script manually:
   cd scripts && luajit build-data.lua --chunk-size 10000

2. Download pre-built files from:
   ` + GHReleaseURL + `

then place the downloaded .bin files in the 'data' directory.`)
}
