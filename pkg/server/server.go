// Package server implements mspack IPC for completion and config for any clients
package server

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/bastiangx/wordserve/internal/utils"
	"github.com/bastiangx/wordserve/pkg/config"
	"github.com/bastiangx/wordserve/pkg/dictionary"
	completion "github.com/bastiangx/wordserve/pkg/suggest"
	"github.com/charmbracelet/log"
	"github.com/vmihailenco/msgpack/v5"
)

// Server handles msgpack completion requests and runtime configuration
type Server struct {
	completer     completion.ICompleter
	config        *config.Config
	configPath    string
	runtimeLoader *dictionary.RuntimeLoader
	decoder       *msgpack.Decoder
	buffer        *bytes.Buffer
	encoder       *msgpack.Encoder
	writeMutex    sync.Mutex
	requestCount  int64
}

// NewServer creates a server instance with the given completer and configuration
func NewServer(completer completion.ICompleter, cfg *config.Config, configPath string) *Server {
	buffer := &bytes.Buffer{}
	server := &Server{
		completer:  completer,
		config:     cfg,
		configPath: configPath,
		buffer:     buffer,
		encoder:    msgpack.NewEncoder(buffer),
	}
	server.decoder = msgpack.NewDecoder(os.Stdin)

	if lazyCompleter, ok := completer.(*completion.Completer); ok {
		if chunkLoader := lazyCompleter.GetChunkLoader(); chunkLoader != nil {
			server.runtimeLoader = dictionary.NewRuntimeLoader(chunkLoader)
		}
	}
	return server
}

// reloadConfig refreshes configuration from the TOML file
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

// Start begins the main request processing loop
func (s *Server) Start() error {
	log.Debug("Starting server")
	for {
		if err := s.processCompletionRequest(); err != nil {
			if err == io.EOF {
				log.Debug("Client disconnected")
				return nil
			}
			continue
		}
	}
}

// processCompletionRequest handles a single incoming request
func (s *Server) processCompletionRequest() error {
	s.requestCount++
	if s.requestCount%100 == 0 {
		s.reloadConfig()
	}

	if s.requestCount%50 == 0 {
		if completer, ok := s.completer.(interface{ ForceCleanup() }); ok {
			completer.ForceCleanup()
		}
	}

	var rawRequest map[string]any
	if err := s.decoder.Decode(&rawRequest); err != nil {
		log.Debugf("Decode error: %v", err)
		return err
	}

	if action, exists := rawRequest["action"]; exists {
		actionStr := action.(string)
		// Check if it's a config management action
		if actionStr == "rebuild_config" || actionStr == "get_config_path" {
			return s.processConfigRequest(rawRequest, actionStr)
		}
		// Otherwise, it's a dictionary request
		return s.processDictionaryRequest(rawRequest, actionStr)
	}

	if _, hasDictSize := rawRequest["dictionary_size"]; hasDictSize {
		return s.processDictionaryRequest(rawRequest, "set_size")
	}
	if _, hasGetChunkCount := rawRequest["get_chunk_count"]; hasGetChunkCount {
		return s.processDictionaryRequest(rawRequest, "get_chunk_count")
	}

	if _, hasPrefix := rawRequest["p"]; hasPrefix {
		request := s.parseCompletionRequestFromMap(rawRequest)
		return s.handleCompletionRequest(request)
	}

	// Fallback
	request := s.parseCompletionRequest(rawRequest)
	return s.handleCompletionRequest(request)
}

// sendResponse encodes and writes a MessagePack response atomically
func (s *Server) sendResponse(response any) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	s.buffer.Reset()
	if err := s.encoder.Encode(response); err != nil {
		return fmt.Errorf("failed to encode response: %w", err)
	}

	if _, err := os.Stdout.Write(s.buffer.Bytes()); err != nil {
		return fmt.Errorf("failed to write response: %w", err)
	}

	os.Stdout.Sync()
	return nil
}

// sendError sends an error response with the given message and code
func (s *Server) sendError(id string, message string, code int) error {
	errorResponse := &CompletionError{
		ID:    id,
		Error: message,
		Code:  code,
	}
	return s.sendResponse(errorResponse)
}

// processConfigRequest handles configuration management operations
func (s *Server) processConfigRequest(rawRequest map[string]any, action string) error {
	log.Debugf("Processing config request: action=%s", action)

	var id string
	if rawID, ok := rawRequest["id"]; ok {
		id = rawID.(string)
	}

	switch action {
	case "rebuild_config":
		if err := config.RebuildConfigFile(); err != nil {
			return s.sendResponse(&ConfigResponse{
				ID:     id,
				Status: "error",
				Error:  fmt.Sprintf("Failed to rebuild config file: %v", err),
			})
		}
		return s.sendResponse(&ConfigResponse{
			ID:     id,
			Status: "ok",
		})

	case "get_config_path":
		configPath := config.GetActiveConfigPath(s.configPath)
		return s.sendResponse(&ConfigResponse{
			ID:         id,
			Status:     "ok",
			ConfigPath: configPath,
		})

	default:
		return s.sendResponse(&ConfigResponse{
			ID:     id,
			Status: "error",
			Error:  fmt.Sprintf("unknown config action: %s", action),
		})
	}
}

// processDictionaryRequest handles dictionary management operations
func (s *Server) processDictionaryRequest(rawRequest map[string]any, action string) error {
	log.Debugf("Processing dictionary request: action=%s", action)

	var id string
	if rawID, ok := rawRequest["id"]; ok {
		id = rawID.(string)
	}

	if s.runtimeLoader == nil {
		log.Debug("Dictionary management not available - runtimeLoader is nil")
		return s.sendResponse(&DictionaryResponse{
			ID:     id,
			Status: "error",
			Error:  "Dictionary management not available",
		})
	}
	switch action {
	case "get_info":
		stats := s.completer.Stats()
		availableChunks, err := s.runtimeLoader.GetAvailableChunkCount()
		if err != nil {
			return s.sendResponse(&DictionaryResponse{
				ID:     id,
				Status: "error",
				Error:  err.Error(),
			})
		}
		return s.sendResponse(&DictionaryResponse{
			ID:              id,
			Status:          "ok",
			CurrentChunks:   stats["loadedChunks"],
			AvailableChunks: availableChunks,
		})

	case "get_options":
		options, err := s.runtimeLoader.GetDictionarySizeOptions()
		if err != nil {
			return s.sendResponse(&DictionaryResponse{
				ID:     id,
				Status: "error",
				Error:  err.Error(),
			})
		}
		serverOptions := make([]DictionarySizeOption, len(options))
		for i, opt := range options {
			serverOptions[i] = DictionarySizeOption{
				ChunkCount: opt.ChunkCount,
				WordCount:  opt.WordCount,
				SizeLabel:  opt.SizeLabel,
			}
		}
		return s.sendResponse(&DictionaryResponse{
			ID:      id,
			Status:  "ok",
			Options: serverOptions,
		})

	case "set_size":
		chunkCount, exists := rawRequest["chunk_count"]
		if !exists {
			return s.sendResponse(&DictionaryResponse{
				ID:     id,
				Status: "error",
				Error:  "chunk_count required for set_size action",
			})
		}

		count, err := parseChunkCount(chunkCount)
		if err != nil {
			return s.sendResponse(&DictionaryResponse{
				ID:     id,
				Status: "error",
				Error:  fmt.Sprintf("invalid chunk_count: %v", err),
			})
		}

		if err := s.runtimeLoader.SetDictionarySize(count); err != nil {
			return s.sendResponse(&DictionaryResponse{
				ID:     id,
				Status: "error",
				Error:  err.Error(),
			})
		}

		return s.sendResponse(&DictionaryResponse{
			ID:     id,
			Status: "ok",
		})

	case "get_chunk_count":
		availableChunks, err := s.runtimeLoader.GetAvailableChunkCount()
		if err != nil {
			return s.sendResponse(&DictionaryResponse{
				ID:     id,
				Status: "error",
				Error:  err.Error(),
			})
		}

		return s.sendResponse(&DictionaryResponse{
			ID:              id,
			Status:          "ok",
			AvailableChunks: availableChunks,
		})

	default:
		return s.sendResponse(&DictionaryResponse{
			ID:     id,
			Status: "error",
			Error:  fmt.Sprintf("unknown action: %s", action),
		})
	}
}

// parseChunkCount converts interface{} values to integers for chunk counts
func parseChunkCount(value any) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case string:
		return strconv.Atoi(v)
	default:
		return 0, fmt.Errorf("unsupported type: %T", v)
	}
}

// parseCompletionRequestFromMap extracts completion parameters from the raw request
func (s *Server) parseCompletionRequestFromMap(rawRequest map[string]any) CompletionRequest {
	bytes, err := msgpack.Marshal(rawRequest)
	if err != nil {
		log.Debugf("Failed to marshal request map: %v", err)
		return s.parseCompletionRequest(rawRequest)
	}

	var request CompletionRequest
	if err := msgpack.Unmarshal(bytes, &request); err != nil {
		log.Debugf("Failed to unmarshal to CompletionRequest: %v", err)
		return s.parseCompletionRequest(rawRequest)
	}

	return request
}

// parseCompletionRequest extracts completion parameters from the raw request
func (s *Server) parseCompletionRequest(rawRequest map[string]any) CompletionRequest {
	var request CompletionRequest
	if id, ok := rawRequest["id"].(string); ok {
		request.ID = id
	}
	if prefix, ok := rawRequest["p"].(string); ok {
		request.Prefix = prefix
	}
	if limit, ok := rawRequest["l"].(int); ok {
		request.Limit = limit
	} else if limitFloat, ok := rawRequest["l"].(float64); ok {
		request.Limit = int(limitFloat)
	}
	return request
}

// handleCompletionRequest validates and processes a completion request
func (s *Server) handleCompletionRequest(request CompletionRequest) error {
	log.Debugf("Received completion request: prefix='%s', limit=%d", request.Prefix, request.Limit)
	// Validate prefix using config
	if request.Prefix == "" {
		return s.sendError(request.ID, "empty prefix", 400)
	}
	if len(request.Prefix) < s.config.Server.MinPrefix {
		return s.sendError(request.ID, fmt.Sprintf("prefix too short (min: %d)", s.config.Server.MinPrefix), 400)
	}
	if len(request.Prefix) > s.config.Server.MaxPrefix {
		return s.sendError(request.ID, fmt.Sprintf("prefix too long (max: %d)", s.config.Server.MaxPrefix), 400)
	}
	if s.config.Server.EnableFilter && !utils.IsValidInput(request.Prefix) {
		return s.sendResponse(&CompletionResponse{
			ID:          request.ID,
			Suggestions: []CompletionSuggestion{},
			Count:       0,
			TimeTaken:   0,
		})
	}
	if request.Limit <= 0 {
		request.Limit = s.config.Server.MaxLimit / 2
	}
	if request.Limit > s.config.Server.MaxLimit {
		request.Limit = s.config.Server.MaxLimit
	}
	// Get completions with timing
	start := time.Now()
	suggestions := s.completer.Complete(request.Prefix, request.Limit)
	elapsed := time.Since(start)

	responseSuggestions := make([]CompletionSuggestion, len(suggestions))
	for i, s := range suggestions {
		responseSuggestions[i] = CompletionSuggestion{
			Word: s.Word,
			Rank: uint16(i + 1),
		}
	}
	response := &CompletionResponse{
		ID:          request.ID,
		Suggestions: responseSuggestions,
		Count:       len(responseSuggestions),
		TimeTaken:   elapsed.Microseconds(),
	}
	return s.sendResponse(response)
}
