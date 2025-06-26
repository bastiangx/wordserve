package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/bastiangx/typr-lib/internal/cli"
	"github.com/bastiangx/typr-lib/pkg/server"
	completion "github.com/bastiangx/typr-lib/pkg/suggest"
	"github.com/charmbracelet/log"
)

func sigHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Fprintf(os.Stderr, "\nExiting...\n")
		os.Exit(0)
	}()
}

func main() {
	sigHandler()
	binaryDir := flag.String("data", "data/", "Directory containing typer's resource binary files (default: data/)")
	debugMode := flag.Bool("d", false, "Toggle debug mode")
	cliMode := flag.Bool("c", false, "Run in CLI input handler mode")
	limit := flag.Int("limit", 24, "Number of suggestions to return (default: 24)")
	minPrefix := flag.Int("prmin", 1, "Minimum prefix length for suggestions (default: 1)")
	maxPrefix := flag.Int("prmax", 24, "Maximum prefix length for suggestions (default: 24)")
	noFilter := flag.Bool("no-filter", false, "Disable input filtering (for debugging - shows all dictionary entries)")
	// Lazy loading options
	wordLimit := flag.Int("words", 50000, "Maximum number of words to load (default: 50000, use 0 for all words)")
	chunkSize := flag.Int("chunk", 10000, "Number of words per chunk for lazy loading (default: 10000)")

	flag.Parse()

	// debugmode wip -- neds logic checks in the other packages
	// needs to be gloabl var for package. read TODO on how
	if *debugMode {
		log.SetLevel(log.DebugLevel)
		log.SetReportTimestamp(false)
	} else {
		log.SetLevel(log.ErrorLevel)
	}

	// Create completer with CompactTrie optimization
	log.Debugf("Initializing completer: maxWords=%d, chunkSize=%d", *wordLimit, *chunkSize)
	completer := completion.NewLazyCompleter(*binaryDir, *chunkSize, *wordLimit)

	if *binaryDir != "" {
		log.Debug("Initializing lazy loading from", "dir", *binaryDir, "maxWords", *wordLimit)
		err := completer.Initialize()
		if err != nil {
			log.Fatalf("Failed to initialize lazy completer: %v", err)
			os.Exit(1)
		}
		log.Debug("Lazy completer initialized successfully")
	} else {
		log.Warn("No binary directory specified, running with empty dict")
	}

	if *cliMode {
		log.Debug("Input info:",
			"minPrefix", *minPrefix,
			"maxPrefix", *maxPrefix,
			"limit", *limit,
			"noFilter", *noFilter)

		inputHandler := cli.NewInputHandler(completer, *minPrefix, *maxPrefix, *limit, *noFilter)
		if err := inputHandler.Start(); err != nil {
			log.Fatalf("CLI input handler error: %v", err)
			os.Exit(1)
		}
		return
	}

	log.Debug("spawning IPC processor")
	srv := server.NewServer(completer)
	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
		os.Exit(1)
	}
}
