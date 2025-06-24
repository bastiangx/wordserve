package utils

import (
	"strings"
	"sync"
)

// Capital letter processing uses a pool to reduce allocations
var capitalInfoPool = sync.Pool{
	New: func() any {
		return &CapitalInfo{
			positions: make([]int, 0, 4), // Pre-allocate for typical cases
			chars:     make([]rune, 0, 4),
		}
	},
}

// CapitalInfo holds information about capitalization in a string
type CapitalInfo struct {
	positions []int
	chars     []rune
}

// Reset resets the CapitalInfo for reuse
func (ci *CapitalInfo) Reset() {
	ci.positions = ci.positions[:0]
	ci.chars = ci.chars[:0]
}

// ProcessCapitals extracts capital letter information from a string and returns
// both the lowercase version and a channel that will receive the capital info
func ProcessCapitals(s string) (string, chan *CapitalInfo) {
	resultChan := make(chan *CapitalInfo, 1) // Buffered to prevent blocking

	// Start processing in background
	go func() {
		info := capitalInfoPool.Get().(*CapitalInfo)
		info.Reset()

		// Process the string for capitals
		for i, r := range s {
			if r >= 'A' && r <= 'Z' {
				info.positions = append(info.positions, i)
				info.chars = append(info.chars, r)
			}
		}

		// If no capitals found, return the info to pool and send nil
		if len(info.positions) == 0 {
			capitalInfoPool.Put(info)
			resultChan <- nil
			close(resultChan)
			return
		}

		// Send the info and close channel
		resultChan <- info
		close(resultChan)
	}()

	// Return immediately with lowercase string and channel
	return strings.ToLower(s), resultChan
}

// ApplyCapitals applies capitalization info to a word asynchronously
// Returns a channel that will receive the processed word
func ApplyCapitals(word string, info *CapitalInfo) chan string {
	resultChan := make(chan string, 1) // Buffered to prevent blocking

	// If no info or no capitals, return original word immediately
	if info == nil {
		resultChan <- word
		close(resultChan)
		return resultChan
	}

	// Process in background
	go func() {
		runes := []rune(word)

		// Apply capitals only if the position exists in the word
		for i, pos := range info.positions {
			if pos < len(runes) {
				runes[pos] = info.chars[i]
			}
		}

		// Return info to pool after use
		capitalInfoPool.Put(info)

		// Send result and close channel
		resultChan <- string(runes)
		close(resultChan)
	}()

	return resultChan
}
