// Package server implements MessagePack IPC for completion + config updates
package server

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/bastiangx/typr-lib/pkg/config"
	"github.com/bastiangx/typr-lib/internal/utils"
	completion "github.com/bastiangx/typr-lib/pkg/suggest"
	"github.com/charmbracelet/log"
	"github.com/vmihailenco/msgpack/v5"
)

// Server handles completion requests and config updates
type Server struct {
	completer  completion.ICompleter
	config     *config.Config
	configPath string
	mu         sync.RWMutex
}

// NewServer creates server with configuration
func NewServer(completer completion.ICompleter, cfg *config.Config, configPath string) *Server {
	return &Server{
		completer:  completer,
		config:     cfg,
		configPath: configPath,
	}
}

// Start begins listening for completion requests
func (s *Server) Start() error {
	log.Debug("Starting MessagePack completion server")

	// Main completion loop - optimized for speed
	for {
		if err := s.processCompletionRequest(); err != nil {
			if err == io.EOF {
				log.Debug("Client disconnected")
				return nil
			}
			log.Errorf("Processing completion request: %v", err)
			return err
		}
	}
}

// processCompletionRequest handles a single completion request
func (s *Server) processCompletionRequest() error {
	// Read MessagePack data from stdin
	var request CompletionRequest
	decoder := msgpack.NewDecoder(os.Stdin)
	log.Debug("Waiting for request...")
	if err := decoder.Decode(&request); err != nil {
		log.Debugf("Decode error: %v", err)
		return err
	}
	log.Debugf("Received request: prefix='%s', limit=%d", request.Prefix, request.Limit)

	// Validate prefix using config
	if request.Prefix == "" {
		return s.sendError("empty prefix", 400)
	}
	if len(request.Prefix) < s.config.Server.MinPrefix {
		return s.sendError(fmt.Sprintf("prefix too short (min: %d)", s.config.Server.MinPrefix), 400)
	}
	if len(request.Prefix) > s.config.Server.MaxPrefix {
		return s.sendError(fmt.Sprintf("prefix too long (max: %d)", s.config.Server.MaxPrefix), 400)
	}
	
	// Apply input filtering if enabled
	if s.config.Server.EnableFilter && !utils.IsValidInput(request.Prefix) {
		// Return empty suggestions for invalid input
		return s.sendResponse(&CompletionResponse{
			Suggestions: []CompletionSuggestion{},
			Count:       0,
			TimeTaken:   0,
		})
	}

	// Apply limit using config
	if request.Limit <= 0 {
		request.Limit = s.config.Server.MaxLimit / 2 // reasonable default
	}
	if request.Limit > s.config.Server.MaxLimit {
		request.Limit = s.config.Server.MaxLimit
	}

	// Get completions with timing
	start := time.Now()
	suggestions := s.completer.Complete(request.Prefix, request.Limit)
	elapsed := time.Since(start)

	// Convert to response format
	ranks := utils.GeneratePositionalRanks(len(suggestions))
	responseSuggestions := make([]CompletionSuggestion, len(suggestions))
	for i, s := range suggestions {
		responseSuggestions[i] = CompletionSuggestion{
			Word: s.Word,
			Rank: ranks[i],
		}
	}

	response := &CompletionResponse{
		Suggestions: responseSuggestions,
		Count:       len(responseSuggestions),
		TimeTaken:   elapsed.Microseconds(),
	}

	return s.sendResponse(response)
}

// sendResponse encodes and sends MessagePack response to stdout
func (s *Server) sendResponse(response any) error {
	encoder := msgpack.NewEncoder(os.Stdout)
	return encoder.Encode(response)
}

// sendError sends MessagePack error response
func (s *Server) sendError(message string, code int) error {
	errorResponse := &CompletionError{
		Error: message,
		Code:  code,
	}
	return s.sendResponse(errorResponse)
}
