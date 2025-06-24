package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/bastiangx/typr-lib/pkg/completion"
	"github.com/charmbracelet/log"
)

// ResponseSuggestion is the format for each suggestion in the API response
type ResponseSuggestion struct {
	Word string  `json:"word"`
	Rank float64 `json:"rank"`
	Freq int     `json:"freq,omitempty"`
}

// CompletionResponse is the overall API response format
type CompletionResponse struct {
	Suggestions     []ResponseSuggestion `json:"suggestions"`
	Count           int                  `json:"count"`
	Prefix          string               `json:"prefix"`
	TimeTaken       int64                `json:"time_ms"`
	WasCorrected    bool                 `json:"was_corrected,omitempty"`
	CorrectedPrefix string               `json:"corrected_prefix,omitempty"`
}

// ErrorResponse represents an API error
type ErrorResponse struct {
	Error  string `json:"error"`
	Status int    `json:"status"`
}

// Request represents an incoming request from the client
type Request struct {
	Command string `json:"command"`
	Prefix  string `json:"prefix"`
	Limit   int    `json:"limit,omitempty"`
	Fuzzy   bool   `json:"fuzzy,omitempty"`
}

// Server handles the IPC for word completions
type Server struct {
	completer *completion.Completer
	reader    *bufio.Reader
	writer    io.Writer
}

// Creates a new completion server using stdin/stdout for IPC
func NewServer(completer *completion.Completer) *Server {
	return &Server{
		completer: completer,
		reader:    bufio.NewReader(os.Stdin),
		writer:    os.Stdout,
	}
}

// Start begins listening for IPC requests
func (s *Server) Start() error {
	log.Debug("Starting Server.")

	// Signal that the server is ready
	s.sendResponse(map[string]string{"status": "ready"})

	// incoming requests stdin
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			log.Errorf("Reading from stdin: %v", err)
			return err
		}

		line = strings.TrimSpace(line)
		s.handleRequest(line)
	}
}

// handleRequest processes an incoming request string
func (s *Server) handleRequest(requestStr string) {
	var request Request
	if err := json.Unmarshal([]byte(requestStr), &request); err != nil {
		s.sendError("Invalid JSON request", 400)
		log.Errorf("Unmarshaling request: %v", err)
		return
	}

	// based on command
	switch request.Command {
	case "complete":
		s.handleComplete(request)
	case "health":
		s.sendResponse(map[string]string{"status": "ok"})
	default:
		s.sendError(fmt.Sprintf("Unknown command: %s", request.Command), 400)
	}
}

//	sendResponse function marshals the given response interface into JSON format and sends it to the client.
//
// The response is written to the server's writer, followed by a newline character.
func (s *Server) sendResponse(response interface{}) {
	data, err := json.Marshal(response)
	if err != nil {
		log.Errorf("Marshaling response: %v", err)
		s.sendError("Internal server error", 500)
		return
	}
	fmt.Fprintln(s.writer, string(data))
}

// sendError sends an error response
func (s *Server) sendError(message string, code int) {
	errResponse := ErrorResponse{
		Error:  message,
		Status: code,
	}
	s.sendResponse(errResponse)
}

// TODO: replace magic numbers with config defaults.
// handleComplete processes a completion request. It validates the request,
// retrieves suggestions from the completer, normalizes the rankings, and sends
// the response. It handles fuzzy matching, prefix validation, and sets a default
// limit if not specified in the request. It also includes correction information
// in the response if the prefix was corrected.
func (s *Server) handleComplete(request Request) {
	prefix := request.Prefix

	if prefix == "" {
		s.sendError("Missing 'prefix' parameter", 400)
		log.Debug("Prefix is empty in request")
		return
	}

	if len(prefix) < 1 {
		s.sendError("Prefix must be at least 1 characters", 400)
		log.Debug("Prefix is too short in request")
		return
	}

	if len(prefix) > 60 {
		s.sendError("Prefix exceeds maximum length of 60 characters", 400)
		log.Debug("Prefix is too long in request")
		return
	}

	limit := request.Limit
	if limit < 1 {
		limit = 10
	}

	start := time.Now()
	var suggestions []completion.Suggestion
	if request.Fuzzy {
		suggestions = s.completer.CompleteWithFuzzy(prefix, limit)
	} else {
		suggestions = s.completer.Complete(prefix, limit)
	}
	elapsed := time.Since(start)

	normalizedSuggestions := normalizeRankings(suggestions)

	wasCorrected := false
	correctedPrefix := ""
	if len(suggestions) > 0 && suggestions[0].WasCorrected {
		wasCorrected = true
		correctedPrefix = suggestions[0].CorrectedPrefix
	}

	response := CompletionResponse{
		Suggestions:     normalizedSuggestions,
		Count:           len(normalizedSuggestions),
		Prefix:          prefix,
		TimeTaken:       elapsed.Milliseconds(),
		WasCorrected:    wasCorrected,
		CorrectedPrefix: correctedPrefix,
	}

	s.sendResponse(response)
}

// TODO: ranking hit max 10, which is not what we want. users can have up to 64 or more suggestions.
// normalizeRankings converts raw frequency values to a 1-10 scale
// where 1 is the highest rank (highest frequency) and 10 is the lowest
func normalizeRankings(suggestions []completion.Suggestion) []ResponseSuggestion {
	result := make([]ResponseSuggestion, len(suggestions))

	// If no suggestions, return empty array
	if len(suggestions) == 0 {
		return result
	}

	// Find highest frequency for normalization
	highestFreq := suggestions[0].Frequency

	// Avoid div_by_zero
	if highestFreq == 0 {
		for i, s := range suggestions {
			result[i] = ResponseSuggestion{
				Word: s.Word,
				Rank: 10.0,
				Freq: s.Frequency,
			}
		}
		return result
	}

	for i, s := range suggestions {
		// Normalize linearly based on position in results
		normalizedRank := 1.0 + 9.0*float64(i)/float64(len(suggestions)-1)

		// For single result, assign rank 1
		if len(suggestions) == 1 {
			normalizedRank = 1.0
		}

		result[i] = ResponseSuggestion{
			Word: s.Word,
			Rank: normalizedRank,
			Freq: s.Frequency,
		}
	}

	return result
}
