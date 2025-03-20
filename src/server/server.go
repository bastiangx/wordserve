package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
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

// Server handles the HTTP requests for word completions
type Server struct {
	completer *completion.Completer
	port      string
}

// NewServer creates a new completion server
func NewServer(completer *completion.Completer, port string) *Server {
	return &Server{
		completer: completer,
		port:      port,
	}
}

// Start begins listening for requests
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Add routes
	mux.HandleFunc("/complete", s.handleComplete)
	mux.HandleFunc("/health", s.handleHealth)

	// Configure server
	server := &http.Server{
		Addr:         ":" + s.port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("Starting completion server on port %s", s.port)
	return server.ListenAndServe()
}

// handleHealth is a simple health check endpoint
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleComplete handles completion requests
func (s *Server) handleComplete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Only accept GET requests
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error:  "Method not allowed",
			Status: http.StatusMethodNotAllowed,
		})
		return
	}

	// Get prefix from query parameter
	prefix := r.URL.Query().Get("prefix")
	if prefix == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error:  "Missing 'prefix' parameter",
			Status: http.StatusBadRequest,
		})
		return
	}

	// Validate prefix length
	if len(prefix) < 2 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error:  "Prefix must be at least 2 characters",
			Status: http.StatusBadRequest,
		})
		return
	}

	if len(prefix) > 60 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error:  "Prefix exceeds maximum length of 60 characters",
			Status: http.StatusBadRequest,
		})
		return
	}

	// Get limit parameter (default to 10)
	limitStr := r.URL.Query().Get("limit")
	limit := 10
	if limitStr != "" {
		var err error
		limit, err = strconv.Atoi(limitStr)
		if err != nil || limit < 1 {
			limit = 10
		}
	}

	// Get fuzzy parameter (default to true)
	fuzzyStr := r.URL.Query().Get("fuzzy")
	useFuzzy := true
	if fuzzyStr == "false" || fuzzyStr == "0" {
		useFuzzy = false
	}

	// Get suggestions with optional fuzzy matching
	start := time.Now()
	var suggestions []completion.Suggestion
	if useFuzzy {
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

	json.NewEncoder(w).Encode(response)
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
