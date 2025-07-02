// Package server implements MessagePack IPC for completion + config updates
package server

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/bastiangx/typr-lib/internal/utils"
	"github.com/bastiangx/typr-lib/pkg/config"
	"github.com/bastiangx/typr-lib/pkg/dictionary"
	completion "github.com/bastiangx/typr-lib/pkg/suggest"
	"github.com/charmbracelet/log"
	"github.com/vmihailenco/msgpack/v5"
)

// Server handles completion requests and config updates
type Server struct {
	completer     completion.ICompleter
	config        *config.Config
	configPath    string
	runtimeLoader *dictionary.RuntimeLoader
}

// NewServer creates server with configuration
func NewServer(completer completion.ICompleter, cfg *config.Config, configPath string) *Server {
	server := &Server{
		completer:  completer,
		config:     cfg,
		configPath: configPath,
	}

	log.Debugf("Creating server with completer type: %T", completer)

	// Initialize runtime loader if completer supports it
	if lazyCompleter, ok := completer.(*completion.Completer); ok {
		log.Debug("Successfully cast completer to *completion.Completer")
		// Access the chunk loader if available
		if chunkLoader := lazyCompleter.GetChunkLoader(); chunkLoader != nil {
			log.Debug("ChunkLoader is available, creating RuntimeLoader")
			server.runtimeLoader = dictionary.NewRuntimeLoader(chunkLoader)
		} else {
			log.Debug("ChunkLoader is nil")
		}
	} else {
		log.Debug("Failed to cast completer to *completion.Completer")
	}

	if server.runtimeLoader != nil {
		log.Debug("RuntimeLoader successfully initialized")
	} else {
		log.Debug("RuntimeLoader is nil after initialization")
	}

	return server
}

// reloadConfig reloads configuration from TOML file
func (s *Server) reloadConfig() error {
	newConfig, err := config.LoadConfig(s.configPath)
	if err != nil {
		log.Warnf("Failed to reload config, keeping current: %v", err)
		return err
	}

	s.config = newConfig
	log.Debugf("Config reloaded from: %s", s.configPath)
	return nil
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
	// Reload config from TOML file to get latest settings
	s.reloadConfig() // Ignore errors to avoid blocking requests

	// Read MessagePack data from stdin
	var rawRequest map[string]interface{}
	decoder := msgpack.NewDecoder(os.Stdin)
	log.Debug("Waiting for request...")
	if err := decoder.Decode(&rawRequest); err != nil {
		log.Debugf("Decode error: %v", err)
		return err
	}

	// Check if this is a dictionary request
	if action, exists := rawRequest["action"]; exists {
		return s.processDictionaryRequest(rawRequest, action.(string))
	}

	// Remove config update handling - now handled via TOML file
	// Dictionary size updates still use msgpack for runtime changes
	if _, hasDictSize := rawRequest["dictionary_size"]; hasDictSize {
		return s.processDictionaryRequest(rawRequest, "set_size")
	}
	if _, hasGetChunkCount := rawRequest["get_chunk_count"]; hasGetChunkCount {
		return s.processDictionaryRequest(rawRequest, "get_chunk_count")
	}

	// Handle as completion request
	var request CompletionRequest
	// Re-parse the raw data as completion request
	encoded, _ := msgpack.Marshal(rawRequest)
	if err := msgpack.Unmarshal(encoded, &request); err != nil {
		return err
	}

	log.Debugf("Received completion request: prefix='%s', limit=%d", request.Prefix, request.Limit)

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

// processDictionaryRequest handles dictionary management requests
func (s *Server) processDictionaryRequest(rawRequest map[string]interface{}, action string) error {
	log.Debugf("Processing dictionary request: action=%s", action)

	if s.runtimeLoader == nil {
		log.Debug("Dictionary management not available - runtimeLoader is nil")
		return s.sendResponse(&DictionaryResponse{
			Status: "error",
			Error:  "Dictionary management not available",
		})
	}

	log.Debugf("RuntimeLoader is available, processing action: %s", action)

	switch action {
	case "get_info":
		currentChunks, availableChunks, err := s.runtimeLoader.GetCurrentDictionaryInfo()
		if err != nil {
			return s.sendResponse(&DictionaryResponse{
				Status: "error",
				Error:  err.Error(),
			})
		}
		return s.sendResponse(&DictionaryResponse{
			Status:          "ok",
			CurrentChunks:   currentChunks,
			AvailableChunks: availableChunks,
		})

	case "get_options":
		options, err := s.runtimeLoader.GetDictionarySizeOptions()
		if err != nil {
			return s.sendResponse(&DictionaryResponse{
				Status: "error",
				Error:  err.Error(),
			})
		}
		// Convert dictionary options to server options
		serverOptions := make([]DictionarySizeOption, len(options))
		for i, opt := range options {
			serverOptions[i] = DictionarySizeOption{
				ChunkCount: opt.ChunkCount,
				WordCount:  opt.WordCount,
				SizeLabel:  opt.SizeLabel,
			}
		}
		return s.sendResponse(&DictionaryResponse{
			Status:  "ok",
			Options: serverOptions,
		})

	case "set_size":
		chunkCount, exists := rawRequest["chunk_count"]
		if !exists {
			return s.sendResponse(&DictionaryResponse{
				Status: "error",
				Error:  "chunk_count required for set_size action",
			})
		}

		var count int
		switch v := chunkCount.(type) {
		case int:
			count = v
		case int64:
			count = int(v)
		case float64:
			count = int(v)
		case string:
			if parsedCount, err := strconv.Atoi(v); err == nil {
				count = parsedCount
			} else {
				return s.sendResponse(&DictionaryResponse{
					Status: "error",
					Error:  "invalid chunk_count format",
				})
			}
		default:
			return s.sendResponse(&DictionaryResponse{
				Status: "error",
				Error:  fmt.Sprintf("invalid chunk_count type: %T", v),
			})
		}

		if err := s.runtimeLoader.SetDictionarySize(count); err != nil {
			return s.sendResponse(&DictionaryResponse{
				Status: "error",
				Error:  err.Error(),
			})
		}

		return s.sendResponse(&DictionaryResponse{
			Status: "ok",
		})

	case "get_chunk_count":
		_, availableChunks, err := s.runtimeLoader.GetCurrentDictionaryInfo()
		if err != nil {
			return s.sendResponse(&ConfigResponse{
				Status: "error",
				Error:  err.Error(),
			})
		}

		return s.sendResponse(&ConfigResponse{
			Status:          "ok",
			AvailableChunks: availableChunks,
		})

	default:
		return s.sendResponse(&DictionaryResponse{
			Status: "error",
			Error:  fmt.Sprintf("unknown action: %s", action),
		})
	}
}
