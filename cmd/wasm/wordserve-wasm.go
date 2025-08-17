//go:build js && wasm
// +build js,wasm

package main

import (
	"encoding/binary"
	"io"
	"strconv"
	"syscall/js"
	"unsafe"

	"github.com/bastiangx/wordserve/pkg/suggest"
	"github.com/vmihailenco/msgpack/v5"
)

type WASMCompleter struct {
	completer suggest.ICompleter
}

type CompletionRequest struct {
	Prefix string `msgpack:"p"`
	Limit  int    `msgpack:"l"`
}

type CompletionResponse struct {
	Suggestions []CompletionSuggestion `msgpack:"s"`
	Count       int                    `msgpack:"c"`
}

type CompletionSuggestion struct {
	Word string `msgpack:"w"`
	Rank int    `msgpack:"r"`
}

var globalCompleter *WASMCompleter

const (
	maxSuggestionLimit     = 128
	defaultSuggestionLimit = 20
)

func main() {
	c := make(chan struct{})

	// Initialize completer
	globalCompleter = &WASMCompleter{
		completer: suggest.NewCompleter(),
	}

	// Export functions to JavaScript
	js.Global().Set("wasmCompleter", js.ValueOf(map[string]any{
		"initWithData":        js.FuncOf(initWithData),
		"initWithBinaryData":  js.FuncOf(initWithBinaryData),
		"addWord":             js.FuncOf(addWord),
		"complete":            js.FuncOf(complete),
		"completeRaw":         js.FuncOf(completeRaw),
		"stats":               js.FuncOf(stats),
		"loadDictionaryChunk": js.FuncOf(loadDictionaryChunk),
	}))

	js.Global().Call("wasmReady")

	<-c
}

// initWithData initializes the completer with word data from a Uint8Array (word\tfrequency format)
func initWithData(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return js.ValueOf(map[string]any{"error": "missing data argument"})
	}

	uint8Array := args[0]
	length := uint8Array.Get("length").Int()

	data := make([]byte, length)
	js.CopyBytesToGo(data, uint8Array)
	lines := parseWordData(data)

	wordCount := 0
	for _, line := range lines {
		if word, freq := parseWordLine(line); word != "" {
			globalCompleter.completer.AddWord(word, freq)
			wordCount++
		}
	}
	return js.ValueOf(map[string]any{
		"success":   true,
		"wordCount": wordCount,
	})
}

// initWithBinaryData initializes with multiple binary dictionary chunks
func initWithBinaryData(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return js.ValueOf(map[string]any{"error": "missing data argument"})
	}

	// args[0] should be an array of Uint8Arrays (one for each chunk)
	chunksArray := args[0]
	if !chunksArray.Get("length").Truthy() {
		return js.ValueOf(map[string]any{"error": "invalid chunks array"})
	}
	totalWords := 0
	chunkCount := chunksArray.Get("length").Int()

	for i := 0; i < chunksArray.Get("length").Int(); i++ {
		chunkData := chunksArray.Index(i)
		if !chunkData.Truthy() {
			continue
		}

		length := chunkData.Get("length").Int()
		data := make([]byte, length)
		js.CopyBytesToGo(data, chunkData)

		words, err := parseBinaryChunk(data)
		if err != nil {
			return js.ValueOf(map[string]any{
				"error": "failed to parse binary chunk " + strconv.Itoa(i) + ": " + err.Error(),
			})
		}
		for word, freq := range words {
			globalCompleter.completer.AddWord(word, freq)
			totalWords++
		}
	}
	return js.ValueOf(map[string]any{
		"success":   true,
		"wordCount": totalWords,
		"chunks":    chunkCount,
	})
}

// loadDictionaryChunk loads a single binary dictionary chunk
func loadDictionaryChunk(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return js.ValueOf(map[string]any{"error": "missing chunk data"})
	}

	uint8Array := args[0]
	length := uint8Array.Get("length").Int()
	data := make([]byte, length)
	js.CopyBytesToGo(data, uint8Array)

	words, err := parseBinaryChunk(data)
	if err != nil {
		return js.ValueOf(map[string]any{"error": "failed to parse chunk: " + err.Error()})
	}
	wordCount := 0
	for word, freq := range words {
		globalCompleter.completer.AddWord(word, freq)
		wordCount++
	}
	return js.ValueOf(map[string]any{
		"success":   true,
		"wordCount": wordCount,
	})
}

// addWord adds a single word with frequency
func addWord(this js.Value, args []js.Value) any {
	if len(args) < 2 {
		return js.ValueOf(map[string]any{"error": "missing word or frequency"})
	}
	word := args[0].String()
	freq := args[1].Int()
	globalCompleter.completer.AddWord(word, freq)
	return js.ValueOf(map[string]any{"success": true})
}

// complete returns word completions for a prefix using MessagePack
func complete(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return js.ValueOf(map[string]any{"error": "missing request data"})
	}
	uint8Array := args[0]
	length := uint8Array.Get("length").Int()
	requestData := make([]byte, length)
	js.CopyBytesToGo(requestData, uint8Array)

	// messagepack decode
	var request CompletionRequest
	if err := msgpack.Unmarshal(requestData, &request); err != nil {
		return js.ValueOf(map[string]any{"error": "failed to decode request: " + err.Error()})
	}

	// Clamp and validate
	if request.Limit <= 0 {
		request.Limit = defaultSuggestionLimit
	} else if request.Limit > maxSuggestionLimit {
		request.Limit = maxSuggestionLimit
	}
	suggestions := globalCompleter.completer.Complete(request.Prefix, request.Limit)
	responseSuggestions := make([]CompletionSuggestion, len(suggestions))
	for i, s := range suggestions {
		responseSuggestions[i] = CompletionSuggestion{
			Word: s.Word,
			Rank: i + 1,
		}
	}

	response := CompletionResponse{
		Suggestions: responseSuggestions,
		Count:       len(responseSuggestions),
	}

	// Encode as MessagePack
	responseData, err := msgpack.Marshal(response)
	if err != nil {
		return js.ValueOf(map[string]any{"error": "failed to encode response: " + err.Error()})
	}
	responseArray := js.Global().Get("Uint8Array").New(len(responseData))
	js.CopyBytesToJS(responseArray, responseData)

	return responseArray
}

// completeRaw returns completions as a simple JavaScript object (without MessagePack)
func completeRaw(this js.Value, args []js.Value) any {
	if len(args) < 2 {
		return js.ValueOf(map[string]any{"error": "missing prefix or limit"})
	}

	prefix := args[0].String()
	limit := args[1].Int()

	suggestions := globalCompleter.completer.Complete(prefix, limit)

	jsSuggestions := make([]any, len(suggestions))
	for i, s := range suggestions {
		jsSuggestions[i] = map[string]any{
			"word":      s.Word,
			"frequency": s.Frequency,
			"rank":      i + 1,
		}
	}

	return js.ValueOf(map[string]any{
		"suggestions": jsSuggestions,
		"count":       len(suggestions),
	})
}

// stats returns completer statistics
func stats(this js.Value, args []js.Value) any {
	stats := globalCompleter.completer.Stats()

	jsStats := make(map[string]any)
	for k, v := range stats {
		jsStats[k] = v
	}
	return js.ValueOf(jsStats)
}

// helpers
func parseWordData(data []byte) []string {
	str := *(*string)(unsafe.Pointer(&data))
	lines := make([]string, 0)

	start := 0
	for i, c := range data {
		if c == '\n' {
			if i > start {
				lines = append(lines, str[start:i])
			}
			start = i + 1
		}
	}
	// Handle last line if no trailing newline
	if start < len(data) {
		lines = append(lines, str[start:])
	}
	return lines
}

func parseWordLine(line string) (string, int) {
	// Find tab separator
	tabIndex := -1
	for i, c := range line {
		if c == '\t' {
			tabIndex = i
			break
		}
	}
	if tabIndex == -1 || tabIndex == 0 || tabIndex == len(line)-1 {
		return "", 0
	}

	word := line[:tabIndex]
	freqStr := line[tabIndex+1:]
	freq := 0

	for _, c := range freqStr {
		if c >= '0' && c <= '9' {
			freq = freq*10 + int(c-'0')
		} else {
			break
		}
	}
	return word, freq
}

// parseBinaryChunk parses a binary dictionary chunk
// Format: [4 bytes word count][word entries...]
// Each word entry: [2 bytes word length][word string][2 bytes rank]
func parseBinaryChunk(data []byte) (map[string]int, error) {
	if len(data) < 4 {
		return nil, js.Error{Value: js.ValueOf("chunk too small")}
	}
	reader := &byteReader{data: data, pos: 0}
	var wordCount int32
	if err := binary.Read(reader, binary.LittleEndian, &wordCount); err != nil {
		return nil, js.Error{Value: js.ValueOf("failed to read word count")}
	}
	words := make(map[string]int, wordCount)

	for i := int32(0); i < wordCount; i++ {
		var wordLen uint16
		if err := binary.Read(reader, binary.LittleEndian, &wordLen); err != nil {
			return nil, js.Error{Value: js.ValueOf("failed to read word length")}
		}
		wordBytes := make([]byte, wordLen)
		if _, err := reader.Read(wordBytes); err != nil {
			return nil, js.Error{Value: js.ValueOf("failed to read word")}
		}
		word := string(wordBytes)
		var rank uint16

		if err := binary.Read(reader, binary.LittleEndian, &rank); err != nil {
			return nil, js.Error{Value: js.ValueOf("failed to read rank")}
		}

		// Convert rank to frequency score (higher rank = lower frequency)
		// Using same formula as the original: score = 65535 - rank + 1
		score := int(65535 - rank + 1)
		words[word] = score
	}
	return words, nil
}

// byteReader implements io.Reader for binary parsing
type byteReader struct {
	data []byte
	pos  int
}

func (r *byteReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	if n < len(p) {
		err = io.EOF
	}
	return
}
