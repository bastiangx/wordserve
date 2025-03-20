package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bastiangx/typr-lib/src/completion"
	"github.com/bastiangx/typr-lib/src/server"
)

func main() {
	// Define command line flags
	binaryDir := flag.String("binaries", "./src/binaries", "Directory containing binary dictionary files")
	textDict := flag.String("text", "", "Path to text dictionary file")
	corpusDir := flag.String("corpus", "./corpus", "Path to corpus directory")
	buildCorpus := flag.Bool("build", false, "Build dictionary from corpus")
	exportBin := flag.String("export", "", "Export path for binary dictionary")
	interactive := flag.Bool("interactive", false, "Run in interactive mode")
	serverMode := flag.Bool("server", false, "Run as HTTP server")
	port := flag.String("port", "8080", "Port for HTTP server")
	flag.Parse()

	// Check if we should build from corpus
	if *buildCorpus {
		fmt.Printf("Building dictionaries from corpus in %s...\n", *corpusDir)
		// You would call your Lua JIT builder here or use Go implementation
		// For example, using os/exec:
		/*
			cmd := exec.Command("luajit", "builder.lua", *corpusDir)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Printf("Error building dictionaries: %v\n", err)
				return
			}
		*/
		fmt.Println("To build dictionaries, please run 'luajit builder.lua' separately")
		return
	}

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
	// DEBUG: Print dictionary
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

	// Choose running mode
	if *serverMode {
		// Run as HTTP server
		srv := server.NewServer(completer, *port)
		fmt.Printf("Starting server on port %s...\n", *port)
		if err := srv.Start(); err != nil {
			fmt.Printf("Server error: %v\n", err)
			os.Exit(1)
		}
	} else if *interactive {
		// Run interactive CLI
		runInteractive(completer)
	} else {
		// Print usage if no mode is selected
		fmt.Println("Please specify either --interactive or --server mode")
		flag.Usage()
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
		suggestions := completer.Complete(input, 10)
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
