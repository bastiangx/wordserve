package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/bastiangx/typr-lib/internal/cli"
	"github.com/bastiangx/typr-lib/pkg/config"
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
	// Load config to get defaults
	defaultConfig := config.DefaultConfig()
	
	limit := flag.Int("limit", defaultConfig.CLI.DefaultLimit, "Number of suggestions to return")
	minPrefix := flag.Int("prmin", defaultConfig.CLI.DefaultMinLen, "Minimum prefix length for suggestions")
	maxPrefix := flag.Int("prmax", defaultConfig.CLI.DefaultMaxLen, "Maximum prefix length for suggestions")
	noFilter := flag.Bool("no-filter", defaultConfig.CLI.DefaultNoFilter, "Disable input filtering (for debugging - shows all dictionary entries)")
	// Lazy loading options
	wordLimit := flag.Int("words", defaultConfig.Dict.MaxWords, "Maximum number of words to load (use 0 for all words)")
	chunkSize := flag.Int("chunk", defaultConfig.Dict.ChunkSize, "Number of words per chunk for lazy loading")

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
	
	// Load or create configuration file
	configPath := "typer-config.toml" // In working directory
	appConfig, err := config.LoadOrCreate(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
		os.Exit(1)
	}
	
	srv := server.NewServer(completer, appConfig, configPath)
	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
		os.Exit(1)
	}
}
