// Package suggest is the core, providing the actual trie traversals and retrievals for prefix inserts and filtering them.
package suggest

// ICompleter defines the interface for word completion engines
type ICompleter interface {
	// Complete returns suggestions for a given prefix with a limit
	Complete(prefix string, limit int) []Suggestion
	
	// AddWord adds a word with its frequency to the completer
	AddWord(word string, frequency int)
	
	// Initialize initializes the completer (replaces LoadBinaryDictionary/LoadAllBinaries)
	Initialize() error
	
	// Stats returns statistics about the loaded dictionary
	Stats() map[string]int
	
	// Backward compatibility methods (delegated to Initialize)
	LoadBinaryDictionary(filename string) error
	LoadAllBinaries(dirPath string) error
}
