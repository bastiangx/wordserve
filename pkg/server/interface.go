package server

// MESSAGE TYPES - Completion + Config Interface

// CompletionRequest - minimal completion request
type CompletionRequest struct {
	Id     string `msgpack:"id"`
	Prefix string `msgpack:"p"`
	Limit  int    `msgpack:"l,omitempty"`
}

// CompletionSuggestion - minimal suggestion response
type CompletionSuggestion struct {
	Word string `msgpack:"w"`
	Rank uint16 `msgpack:"r"`
}

// CompletionResponse - completion response
type CompletionResponse struct {
	Id          string                 `msgpack:"id"`
	Suggestions []CompletionSuggestion `msgpack:"s"`
	Count       int                    `msgpack:"c"`
	TimeTaken   int64                  `msgpack:"t"`
}

// CONFIG MESSAGES - Settings updates (dictionary only, other configs via TOML)

// DictionaryRequest - dictionary management request
type DictionaryRequest struct {
	Id         string `msgpack:"id"`
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
	Id              string                 `msgpack:"id"`
	Status          string                 `msgpack:"status"`
	Error           string                 `msgpack:"error,omitempty"`
	CurrentChunks   int                    `msgpack:"current_chunks,omitempty"`
	AvailableChunks int                    `msgpack:"available_chunks,omitempty"`
	Options         []DictionarySizeOption `msgpack:"options,omitempty"`
}

// ConfigResponse - config operation response
type ConfigResponse struct {
	Id              string `msgpack:"id"`
	Status          string `msgpack:"status"`
	Error           string `msgpack:"error,omitempty"`
	AvailableChunks int    `msgpack:"available_chunks,omitempty"`
}

// ERROR RESPONSES - Generic errors
type CompletionError struct {
	Id    string `msgpack:"id"`
	Error string `msgpack:"e"`
	Code  int    `msgpack:"c"`
}
