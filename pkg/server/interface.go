package server

// MESSAGE TYPES - Completion + Config Interface

// COMPLETION MESSAGES - Ultra fast prefix->suggestions

// CompletionRequest - minimal completion request
type CompletionRequest struct {
	Prefix string `msgpack:"p"`
	Limit  int    `msgpack:"l,omitempty"`
}

// CompletionSuggestion - minimal suggestion response 
type CompletionSuggestion struct {
	Word string `msgpack:"w"`
	Rank uint16 `msgpack:"r"`
}

// CompletionResponse - optimized completion response
type CompletionResponse struct {
	Suggestions []CompletionSuggestion `msgpack:"s"`
	Count       int                    `msgpack:"c"`
	TimeTaken   int64                  `msgpack:"t"` // microseconds
}

// CONFIG MESSAGES - Settings updates

// ConfigUpdateRequest - update server configuration
type ConfigUpdateRequest struct {
	MaxLimit     *int  `msgpack:"max_limit,omitempty"`
	MinPrefix    *int  `msgpack:"min_prefix,omitempty"`
	MaxPrefix    *int  `msgpack:"max_prefix,omitempty"`
	EnableFilter *bool `msgpack:"enable_filter,omitempty"`
}

// ConfigResponse - config operation response
type ConfigResponse struct {
	Status string `msgpack:"status"` // "ok" or "error"
	Error  string `msgpack:"error,omitempty"`
}

// ERROR RESPONSES - Generic errors
type CompletionError struct {
	Error string `msgpack:"e"`
	Code  int    `msgpack:"c"`
}
