// Package suggest provides radix-trie-based word completion with frequency ranking.
//
// The suggest package implements prefix-based word completion using
// Patricia tries for lookups and traversal.
// Supports both immediate result delivery and callback-based streaming for
// different perf requirements.
//
// The core functionality is provided by the [Completer] type, which can operate
// in two modes: static completion with pre-loaded dictionaries, or lazy loading
// with chunk-based dictionary management for large datasets (still early work)
//
// For simple scenarios, create a completer and add words directly:
//
//	completer := suggest.NewCompleter()
//	completer.AddWord("hello", 100)
//	completer.AddWord("help", 50)
//	suggestions := completer.Complete("hel", 10)
//
// For large dictionaries, use lazy loading with chunked data:
//
//	completer := suggest.NewLazyCompleter("data/", 10000, 50000)
//	completer.Initialize()
//	suggestions := completer.Complete("prefix", 20)
//
// For maximum perf and lowest latency, use the callback API to eliminate allocations:
//
//	err := completer.CompleteWithCallback("prefix", 20, func(s suggest.Suggestion) bool {
//	    fmt.Printf("%s (%d)\n", s.Word, s.Frequency)
//	    return true
//	})
//
// The package automatically handles capitalization preservation, frequency-based
// sorting, and deduplication during traversal. Memory pools are used extensively
// to lower the GC pressure.
package suggest

// ICompleter defines the interface for word completion engines.
type ICompleter interface {
	Complete(prefix string, limit int) []Suggestion
	AddWord(word string, frequency int)
	Initialize() error
	Stats() map[string]int
	LoadBinaryDictionary(filename string) error
	LoadAllBinaries(dirPath string) error
}
