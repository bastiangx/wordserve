// Package cli handles cmd line input and suggestions for DBG and testing various features
package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bastiangx/wordserve/internal/utils"
	completion "github.com/bastiangx/wordserve/pkg/suggest"
	"github.com/charmbracelet/log"
)

// InputHandler processes user input from stdin, providing
// suggestions. It accepts many flags to control behavior such as
// minimum and maximum prefix length, suggestion limits, and filtering options.
type InputHandler struct {
	completer       completion.ICompleter
	minPrefixLength int
	maxPrefixLength int
	suggestLimit    int
	requestCount    int
	noFilter        bool
}

// NewInputHandler handles initialization of the InputHandler with basic parameters
func NewInputHandler(completer completion.ICompleter, minLength, maxLength, limit int, noFilter bool) *InputHandler {
	return &InputHandler{
		completer:       completer,
		minPrefixLength: minLength,
		maxPrefixLength: maxLength,
		suggestLimit:    limit,
		noFilter:        noFilter,
	}
}

// Start begins the interface loop.
// It continuously prompts for input, reads a line from stdin,
// and passes the trimmed input to the handleInput() for processing.
// Loop terminates if an error occurs while reading from stdin
func (h *InputHandler) Start() error {
	log.Print("WordServe CLI [BETA]")
	reader := bufio.NewReader(os.Stdin)
	log.Print("type something and press Enter to see the suggestions (Ctrl+C to exit):")

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

// handleInput processes a single prefix to generate suggestions.
// It validates the prefix's length and content, then asks the completer for
// suggestions. Results are formatted and printed to the log.
// Also periodically triggers a memory cleanup for the Completer.
func (h *InputHandler) handleInput(prefix string) {
	h.requestCount++
	if h.requestCount%50 == 0 {
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

	// input filtering by default (unless --no-filter flag is used)
	if !h.noFilter {
		if !utils.IsValidInput(prefix) {
			log.Info("No results found for prefix: '%s'", prefix)
			return
		}
	} else {
		log.Debug("Input filtering disabled - indexed all entries")
	}

	start := time.Now()

	var suggestions []completion.Suggestion
	log.Debug("Processing request for", "prefix", prefix)

	suggestions = h.completer.Complete(prefix, h.suggestLimit)

	elapsed := time.Since(start)
	log.Debugf("Took [ %v ] for prefix '%s'", elapsed, prefix)

	if len(suggestions) == 0 {
		log.Warnf("No suggestions found for prefix: '%s'", prefix)
		return
	}

	log.Printf("Found %d suggestions for prefix '%s':", len(suggestions), prefix)
	for i, s := range suggestions {
		fmtFreq := utils.FormatWithCommas(s.Frequency)
		clWord := fmt.Sprintf("\033[38;5;75m%s\033[0m", s.Word)
		log.Printf("%2d. %-40s (freq: %8s)", i+1, clWord, fmtFreq)
	}
}
