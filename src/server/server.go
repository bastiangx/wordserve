package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/bastiangx/typr-lib/src/completion"
)

// ResponseSuggestion is the format for each suggestion in the API response
type ResponseSuggestion struct {
	Word string  `json:"word"`
	Rank float64 `json:"rank"`
	Freq int     `json:"freq,omitempty"` // Original frequency, could be omitted in production
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

// Server handles the IPC communication for word completions
type Server struct {
	completer *completion.Completer
	reader    *bufio.Reader
	writer    io.Writer
}

// NewServer creates a new completion server using stdin/stdout for IPC
func NewServer(completer *completion.Completer) *Server {
	return &Server{
		completer: completer,
		reader:    bufio.NewReader(os.Stdin),
		writer:    os.Stdout,
	}
}

// Start begins listening for IPC requests
func (s *Server) Start() error {
	log.Printf("Starting completion server using IPC")

	// Signal that the server is ready
	s.sendResponse(map[string]string{"status": "ready"})

	// Process incoming requests
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				log.Println("Client disconnected (EOF)")
				return nil
			}
			log.Printf("Error reading from stdin: %v", err)
			return err
		}

		// Trim the newline character
		line = strings.TrimSpace(line)

		// Process the request
		s.handleRequest(line)
	}
}

// handleRequest processes an incoming request string
func (s *Server) handleRequest(requestStr string) {
	// Parse the request JSON
	var request Request
	if err := json.Unmarshal([]byte(requestStr), &request); err != nil {
		s.sendError("Invalid JSON request", 400)
		return
	}

	// Process based on command
	switch request.Command {
	case "complete":
		s.handleComplete(request)
	case "health":
		s.sendResponse(map[string]string{"status": "ok"})
	default:
		s.sendError(fmt.Sprintf("Unknown command: %s", request.Command), 400)
	}
}

// sendResponse sends a JSON response to stdout
func (s *Server) sendResponse(response interface{}) {
	data, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshaling response: %v", err)
		s.sendError("Internal server error", 500)
		return
	}

	// Write the response followed by a newline
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

// handleComplete handles completion requests
func (s *Server) handleComplete(request Request) {
	prefix := request.Prefix

	// Validate prefix
	if prefix == "" {
		s.sendError("Missing 'prefix' parameter", 400)
		return
	}

	// Validate prefix length
	if len(prefix) < 2 {
		s.sendError("Prefix must be at least 2 characters", 400)
		return
	}

	if len(prefix) > 60 {
		s.sendError("Prefix exceeds maximum length of 60 characters", 400)
		return
	}

	// Set default limit if not specified
	limit := request.Limit
	if limit < 1 {
		limit = 10
	}

	// Get suggestions with optional fuzzy matching
	start := time.Now()
	var suggestions []completion.Suggestion
	if request.Fuzzy {
		suggestions = s.completer.CompleteWithFuzzy(prefix, limit)
	} else {
		suggestions = s.completer.Complete(prefix, limit)
	}
	elapsed := time.Since(start)

	// Normalize rankings from 1 to 10
	normalizedSuggestions := normalizeRankings(suggestions)

	// Add 'corrected' field to response if needed
	wasCorrected := false
	correctedPrefix := ""
	if len(suggestions) > 0 && suggestions[0].WasCorrected {
		wasCorrected = true
		correctedPrefix = suggestions[0].CorrectedPrefix
	}

	// Prepare response
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

	// Handle case where highest frequency is 0 to avoid division by zero
	if highestFreq == 0 {
		for i, s := range suggestions {
			result[i] = ResponseSuggestion{
				Word: s.Word,
				Rank: 10.0, // Default to lowest rank if all frequencies are 0
				Freq: s.Frequency,
			}
		}
		return result
	}

	// Calculate normalized ranks from 1 (highest) to 10 (lowest)
	for i, s := range suggestions {
		// Calculate rank between 1 and 10
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
