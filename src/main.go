package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	_ "path/filepath"
	"strings"
	"time"

	"github.com/bastiangx/typr-lib/src/completion"
)

func main() {
	// Define command line flags
	binaryDir := flag.String("binaries", "./binaries", "Directory containing binary dictionary files")
	textDict := flag.String("text", "", "Path to text dictionary file (e.g. 20k.txt)")
	exportBin := flag.String("export", "", "Export path for binary dictionary")
	interactive := flag.Bool("interactive", true, "Run in interactive mode")
	flag.Parse()

	// Create a new completer
	completer := completion.NewCompleter()
	start := time.Now()

	// Load binary dictionaries if directory exists
	if _, err := os.Stat(*binaryDir); err == nil {
		fmt.Printf("Loading binary dictionaries from %s...\n", *binaryDir)
		if err := completer.LoadAllBinaries(*binaryDir); err != nil {
			fmt.Printf("Error loading binary dictionaries: %v\n", err)
		}
	} else {
		fmt.Printf("Binary directory %s not found or inaccessible\n", *binaryDir)
	}

	// Load text dictionary if provided
	if *textDict != "" {
		fmt.Printf("Loading text dictionary from %s...\n", *textDict)
		if err := completer.LoadTextDictionary(*textDict); err != nil {
			fmt.Printf("Error loading text dictionary: %v\n", err)
		}
	}

	loadTime := time.Since(start)

	// Print statistics
	stats := completer.Stats()
	fmt.Printf("Dictionary loaded with %d words in %v. Max frequency: %d\n",
		stats["totalWords"], loadTime, stats["maxFrequency"])

	// Export binary dictionary if requested
	if *exportBin != "" {
		fmt.Printf("Exporting binary dictionary to %s...\n", *exportBin)
		if err := completer.SaveBinaryDictionary(*exportBin); err != nil {
			fmt.Printf("Error exporting binary dictionary: %v\n", err)
		} else {
			fmt.Println("Binary dictionary exported successfully")
		}
	}

	// Interactive completion loop
	if *interactive {
		runInteractive(completer)
	}
}

func runInteractive(completer *completion.Completer) {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("\nEnter a prefix to get completions (or 'quit' to exit):")

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "quit" || input == "exit" {
			break
		}

		if len(input) == 0 {
			continue
		}

		// Get and display suggestions
		start := time.Now()
		suggestions := completer.Complete(input, 5)
		elapsed := time.Since(start)

		if len(suggestions) == 0 {
			fmt.Println("No suggestions found.")
		} else {
			fmt.Printf("Found %d suggestions for '%s' in %v:\n", len(suggestions), input, elapsed)
			for i, s := range suggestions {
				fmt.Printf("%d. %s (freq: %d)\n", i+1, s.Word, s.Frequency)
			}
		}
	}

	fmt.Println("Goodbye!")
}
