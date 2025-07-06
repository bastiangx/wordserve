/*
Package server implements msgpack IPC for word completion services.

The server package provides a minimal interface for text completion using msgpack serialization over stdin/stdout.

The protocol uses binary msgpack encoding and supports completion requests, dictionary management ops, and config updates.
Messages are processed synchronously with timing info included in responses.

# IPC

The server operates on a request response model where clients send structured messages via stdin and receive responses through stdout.
Each message contains an ID field and other fields based on the operation type.

Completion requests use mainlty this structure:

	{"id": "req_001", "p": "ame", "l": 24}

The server responds with suggestions ranked by freq:

	{"id": "req_001", "s": [{"w": "amenity", "r": 1}, {"w": "america", "r": 2}], "c": 2, "t": 145}

Dict management enables runtime adjustment of loaded word sets:

	{"id": "dict_001", "action": "set_size", "chunk_count": 5}
	{"id": "dict_002", "action": "get_options"}

Response structures include status information and error details when an op fail.

The server maintains request counts for periodic cleanup and config reloading. -> (BETA ONLY)

# Message Types

CompletionRequest and CompletionResponse handle the main prefix suggestion.
Request includes a prefix string and optional limit for result count.
Responses contain suggestion arrays with word strings and rank information, plus timing data.

DictionaryRequest and DictionaryResponse manage runtime dictionary operations.
Supported actions include: getting current information, setting chunk count, and retrieving available size options.

config messages allow adjustment of server parameters without restart.

msgpack encoding has ~30 to 50% smaller message sizes compared to JSON.
binary format enables faster parsing and generation, less errors and reducing latency by ~40 to 70% in most cases.

you can find more about the retrieval and processing perf and timings in `pkg/suggest/interface`
*/
package server

// CompletionRequest - minimal completion request
type CompletionRequest struct {
	ID     string `msgpack:"id"`
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
	ID          string                 `msgpack:"id"`
	Suggestions []CompletionSuggestion `msgpack:"s"`
	Count       int                    `msgpack:"c"`
	TimeTaken   int64                  `msgpack:"t"`
}

// CONFIG MESSAGES - Settings updates (dictionary only, other configs via TOML)

// DictionaryRequest - dictionary management request
type DictionaryRequest struct {
	ID         string `msgpack:"id"`
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
	ID              string                 `msgpack:"id"`
	Status          string                 `msgpack:"status"`
	Error           string                 `msgpack:"error,omitempty"`
	CurrentChunks   int                    `msgpack:"current_chunks,omitempty"`
	AvailableChunks int                    `msgpack:"available_chunks,omitempty"`
	Options         []DictionarySizeOption `msgpack:"options,omitempty"`
}

// ConfigResponse - config operation response
type ConfigResponse struct {
	ID              string `msgpack:"id"`
	Status          string `msgpack:"status"`
	Error           string `msgpack:"error,omitempty"`
	AvailableChunks int    `msgpack:"available_chunks,omitempty"`
}

// CompletionError holds basic error information for completion requests
type CompletionError struct {
	ID    string `msgpack:"id"`
	Error string `msgpack:"e"`
	Code  int    `msgpack:"c"`
}
