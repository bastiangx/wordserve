// Package cli provides a simple CLI input handler for debugging in real-time and the completion functionality
package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bastiangx/typr-lib/internal/utils"
	completion "github.com/bastiangx/typr-lib/pkg/suggest"
	"github.com/charmbracelet/log"
)

// InputHandler handles CLI input for testing completion functionality
type InputHandler struct {
	completer       completion.ICompleter
	minPrefixLength int
	maxPrefixLength int
	suggestLimit    int
	noFilter        bool // If true, bypasses all input filtering for debugging
	requestCount    int  // Track requests for periodic cleanup
}

// NewInputHandler creates a new CLI input handler
func NewInputHandler(completer completion.ICompleter, minLength, maxLength, limit int, noFilter bool) *InputHandler {
	return &InputHandler{
		completer:       completer,
		minPrefixLength: minLength,
		maxPrefixLength: maxLength,
		suggestLimit:    limit,
		noFilter:        noFilter,
	}
}

// Start begins the CLI input loop
func (h *InputHandler) Start() error {
	log.Print("Typer CLI [BETA]")
	reader := bufio.NewReader(os.Stdin)
	log.Print("type something, press enter to see the suggestions (Ctrl+C to exit):")

	for {
		log.Print("> ")
		prefix, err := reader.ReadString('\n')
		if err != nil {
			return err
		}

		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}

		h.handleInput(prefix)
	}
}

// handleInput processes user input and displays completions
func (h *InputHandler) handleInput(prefix string) {
	// Increment request count and cleanup periodically
	h.requestCount++
	if h.requestCount%50 == 0 {
		// Force cleanup every 50 requests to prevent memory growth
		if completer, ok := h.completer.(interface{ ForceCleanup() }); ok {
			completer.ForceCleanup()
		}
	}

	if len(prefix) < h.minPrefixLength {
		log.Errorf("Prefix too short: %s", prefix)
		return
	}

	if len(prefix) > h.maxPrefixLength {
		log.Errorf("Prefix too long: %s", prefix)
		return
	}

	// Apply input filtering by default (unless --no-filter is used)
	if !h.noFilter {
		if !utils.IsValidInput(prefix) {
			log.Warnf("No suggestions found for prefix: '%s' (filtered out)", prefix)
			return
		}
	} else {
		log.Debug("Input filtering disabled - allowing all inputs")
	}

	start := time.Now()
	var suggestions []completion.Suggestion

	log.Debug("Processing completion request", "prefix", prefix)

	suggestions = h.completer.Complete(prefix, h.suggestLimit)

	elapsed := time.Since(start)

	log.Debugf("Took %v for prefix '%s'", elapsed, prefix)

	if len(suggestions) == 0 {
		log.Warnf("No suggestions found for prefix: '%s'", prefix)
		return
	}

	// Check if correction was applied
	correctedPrefix := ""
	if len(suggestions) > 0 && suggestions[0].WasCorrected {
		correctedPrefix = suggestions[0].CorrectedPrefix
		log.Debugf("Prefix '%s' was corrected to '%s'", prefix, correctedPrefix)
	}

	// Pretty print suggestions with details
	log.Printf("Found %d suggestions for prefix '%s':", len(suggestions), prefix)
	for i, s := range suggestions {
		fmtFreq := formatWithCommas(s.Frequency)
		clWord := fmt.Sprintf("\033[38;5;75m%s\033[0m", s.Word)

		log.Printf("%2d. %-40s (freq: %8s)", i+1, clWord, fmtFreq)
	}
}

// formatWithCommas formats an integer with comma separators
func formatWithCommas(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}

	// Convert to string and add commas
	str := fmt.Sprintf("%d", n)
	result := ""
	for i, char := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result += ","
		}
		result += string(char)
	}
	return result
}
