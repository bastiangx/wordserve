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

// CONFIG MESSAGES - Settings updates (dictionary only, other configs via TOML)

// DictionaryRequest - dictionary management request
type DictionaryRequest struct {
	Action     string `msgpack:"action"`                // "get_info", "set_size", "get_options", "get_chunk_count"
	ChunkCount *int   `msgpack:"chunk_count,omitempty"` // for "set_size"
}

// DictionarySizeOption - dictionary size option
type DictionarySizeOption struct {
	ChunkCount int    `msgpack:"chunk_count"`
	WordCount  int    `msgpack:"word_count"`
	SizeLabel  string `msgpack:"size_label"`
}

// DictionaryResponse - dictionary operation response
type DictionaryResponse struct {
	Status          string                 `msgpack:"status"` // "ok" or "error"
	Error           string                 `msgpack:"error,omitempty"`
	CurrentChunks   int                    `msgpack:"current_chunks,omitempty"`
	AvailableChunks int                    `msgpack:"available_chunks,omitempty"`
	Options         []DictionarySizeOption `msgpack:"options,omitempty"`
}

// ConfigResponse - config operation response
type ConfigResponse struct {
	Status          string `msgpack:"status"` // "ok" or "error"
	Error           string `msgpack:"error,omitempty"`
	AvailableChunks int    `msgpack:"available_chunks,omitempty"`
}

// ERROR RESPONSES - Generic errors
type CompletionError struct {
	Error string `msgpack:"e"`
	Code  int    `msgpack:"c"`
}
