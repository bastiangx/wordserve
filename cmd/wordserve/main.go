// Copyright 2025 The WordServe Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

/*
Package main implements word completion server and CLI [DBG] application.

Note: This is a BETA release. APIs and functionality may rapidly change.

WordServe provides fast prefix-based word completion using Patricia tries with
frequency ranking. It can operate as a MessagePack IPC server for integration
with text editors, or as a CLI application for testing and debugging.

The server mode uses lazy-loaded chunked dictionaries to efficiently handle
large word datasets while maintaining low memory usage. Words are ranked by
frequency and filtered based on configurable thresholds to provide relevant
suggestions.

# Usage

Start the server with default settings:

	wserve

Use custom data directory and enable debug mode:

	wserve -data /path/to/chunks -d

Run in CLI mode for interactive testing:

	wserve -c -limit 10 -prmin 2

The data directory should contain chunked binary files named dict_0001.bin,
dict_0002.bin, etc. These files are generated from word frequency data and
loaded on-demand based on the configured limits.

# Configuration

Runtime configuration is managed through a TOML file that supports server
parameters, dictionary settings, and CLI defaults:

	[server]
	max_limit = 64
	min_prefix = 1
	max_prefix = 60
	enable_filter = true

	[dict]
	max_words = 50000
	chunk_size = 10000
	min_frequency_threshold = 20

The config file is automatically created with defaults if it doesn't exist.
Server mode reloads configuration periodically without restart.

# IPC Protocol

The server communicates via MessagePack over stdin/stdout. Completion requests
are processed synchronously with microsecond timing information included in
responses.

Send a completion request:

	{"id": "req1", "p": "hello", "l": 20}

Receive suggestions with frequency ranking:

	{"id": "req1", "s": [{"w": "hello", "r": 1}, {"w": "help", "r": 2}], "c": 2, "t": 145}

Dictionary management requests allow runtime adjustment of loaded chunks:

	{"id": "dict1", "action": "get_info"}
	{"id": "dict2", "action": "set_size", "chunk_count": 5}

# Server Mode

The default mode starts a MessagePack IPC server that processes completion
requests from stdin and writes responses to stdout. This design enables
integration with text editors and other applications through process
communication.

	server := server.NewServer(completer, config, configPath)
	err := server.Start()

The server automatically handles request parsing, validation, and response
formatting. It includes built-in rate limiting, configuration reloading,
and memory management features for long-running sessions.

# CLI Mode

CLI mode provides an interactive interface for testing and debugging
completion functionality. It reads prefixes from stdin and displays
suggestions with frequency information.

	inputHandler := cli.NewInputHandler(completer, minLen, maxLen, limit, noFilter)
	err := inputHandler.Start()

This mode is primarily intended for development and testing new features
before deploying to server mode. It supports the same filtering and
threshold logic as the server but with human-readable output.

# Completion Engine

The core completion functionality is provided by the suggest package,
which implements Patricia trie-based prefix matching with frequency ranking.

	completer := suggest.NewLazyCompleter(dataDir, chunkSize, maxWords)
	err := completer.Initialize()
	suggestions := completer.Complete("prefix", 20)

The completer supports both static word addition and lazy loading from
chunked binary files. Memory pools are used extensively to reduce garbage
collection pressure during high-frequency operations.

# Command Line Flags

The following flags control application behavior:

	-data string
	    Directory containing binary chunk files (default "data/")
	-d  Enable debug mode with detailed logging
	-c  Run in CLI mode instead of server mode
	-limit int
	    Number of suggestions to return (default from config)
	-prmin int
	    Minimum prefix length for suggestions
	-prmax int
	    Maximum prefix length for suggestions
	-no-filter
	    Disable input filtering for debugging
	-words int
	    Maximum words to load (0 for all)
	-chunk int
	    Words per chunk for lazy loading

The application automatically resolves data and config paths relative to the
executable location, supporting both development and production deployments.

# Mem

The lazy loader manages memory usage by loading dictionary chunks on demand
and providing cleanup mechanisms. The server periodically triggers garbage
collection and reloads configuration to maintain optimal performance during
long-running sessions.

Input filtering removes non-alphabetic prefixes by default to improve
suggestion relevance, though this can be disabled for debugging purposes.
Frequency thresholds are automatically adjusted based on prefix length to
balance result quality and quantity.
*/
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/bastiangx/wordserve/internal/cli"
	"github.com/bastiangx/wordserve/internal/utils"
	"github.com/bastiangx/wordserve/pkg/config"
	"github.com/bastiangx/wordserve/pkg/server"
	completion "github.com/bastiangx/wordserve/pkg/suggest"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
)

const (
	Version = "0.9.0-beta"
	AppName = "wordserve"
	gh      = "https://github.com/bastiangx/wordserve"
)

// sigHandler is a simple handler for OS signals to exit normally.
func sigHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Fprintf(os.Stderr, "\nExiting...\n")
		os.Exit(0)
	}()
}

// main calls other packages to initialize the server or CLI inputs.
// main() does not implement logic for them and only manages the flow.
func main() {
	sigHandler()
	defaultConfig := config.DefaultConfig()

	// custom Flags
	showVersion := flag.Bool("version", false, "Show current version")
	binaryDir := flag.String("data", "data/", "Directory containing the binary files")
	debugMode := flag.Bool("d", false, "Toggle debug mode")
	cliMode := flag.Bool("c", false, "Run CLI -- useful for testing and debugging")
	limit := flag.Int("limit", defaultConfig.CLI.DefaultLimit, "Number of suggestions to return")
	minPrefix := flag.Int("prmin", defaultConfig.CLI.DefaultMinLen, "Minimum prefix length for suggestions (1 < n <= prmax)")
	maxPrefix := flag.Int("prmax", defaultConfig.CLI.DefaultMaxLen, "Maximum prefix length for suggestions")
	noFilter := flag.Bool("no-filter", defaultConfig.CLI.DefaultNoFilter, "Disable input filtering (DBG only) - shows all raw dictionary entries (numbers, symbols, etc)")
	wordLimit := flag.Int("words", defaultConfig.Dict.MaxWords, "Maximum number of words to load (use 0 for all words)")
	chunkSize := flag.Int("chunk", defaultConfig.Dict.ChunkSize, "Number of words per chunk for lazy loading")

	flag.Parse()

	if *showVersion {
		logger := log.NewWithOptions(os.Stderr, log.Options{
			ReportCaller:    false,
			ReportTimestamp: false,
			Prefix:          "",
		})

		styles := log.DefaultStyles()

		styles.Values["version"] = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#575279", Dark: "#e0def4"})
		styles.Values["version"] = lipgloss.NewStyle().
			Background(lipgloss.AdaptiveColor{Light: "#f2e9e1", Dark: "#26233a"})

		styles.Values["gh"] = lipgloss.NewStyle().Italic(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#575279", Dark: "#e0def4"})

		logger.SetStyles(styles)

		logger.Print("")
		logger.Print("[ WordServe ] Serves really Fast word completions!")
		logger.Print("", "version", Version)
		logger.Print("")
		logger.Print("use -h or --help to see available options")
		logger.Print("Github Repo", "gh", gh)

		os.Exit(0)
	}

	// Initialize path resolver for robust path handling
	pathResolver, err := utils.NewPathResolver()
	if err != nil {
		log.Fatalf("Failed to initialize path resolver: %v", err)
		log.Print("Either env is not set or system is not supported")
		log.Print("Did you forget to run the build or install scripts?")
		os.Exit(1)
	}

	if *debugMode {
		log.SetLevel(log.DebugLevel)
		log.SetReportTimestamp(true)
	} else {
		log.SetLevel(log.WarnLevel)
	}

	// Pathfinder for bin dir
	resolvedDataDir, err := pathResolver.GetDataDir(*binaryDir)
	if err != nil {
		log.Fatalf("Failed to resolve data dir:(%v)", err)
		os.Exit(1)
	}

	log.Debugf("Using data dir at: %s", resolvedDataDir)
	log.Debugf("Init completer: maxWords=[%d], chunkSize=[%d]", *wordLimit, *chunkSize)

	completer := completion.NewLazyCompleter(resolvedDataDir, *chunkSize, *wordLimit)

	if *binaryDir != "" {
		err := completer.Initialize()
		if err != nil {
			log.Fatalf("Failed to init completer: %v", err)
			os.Exit(1)
		}
		log.Debug("Completer init done")
	} else {
		log.Warn("No binary dir specified, running with empty dict...")
	}

	// CLI would be mainly used for testing and dbg purposes.
	// Any new features or changes should be tested in CLI mode first.
	// NOTE: Server interface has vastly different parameters compared to CLI and what it accepts.
	if *cliMode {
		log.SetReportTimestamp(false)
		log.Debug("Input info:",
			"minPrefix", *minPrefix,
			"maxPrefix", *maxPrefix,
			"limit", *limit,
			"noFilter", *noFilter)

		inputHandler := cli.NewInputHandler(completer, *minPrefix, *maxPrefix, *limit, *noFilter)
		if err := inputHandler.Start(); err != nil {
			log.Fatalf("CLI error: %v", err)
			os.Exit(1)
		}
		return
	}

	log.Debug("spawning IPC")
	configPath, err := pathResolver.GetConfigPath("typer-config.toml")
	if err != nil {
		log.Fatalf("Failed to determine config path: (%v)", err)
		os.Exit(1)
	}
	log.Debugf("Using config file: (%s)", configPath)

	appConfig, err := config.InitConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
		os.Exit(1)
	}
	srv := server.NewServer(completer, appConfig, configPath)

	showStartupInfo(resolvedDataDir)

	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
		os.Exit(1)
	}
}

// showStartupInfo displays some basic info about the init process.
func showStartupInfo(dataDir string) {
	pid := os.Getpid()
	currentLevel := log.GetLevel()
	log.SetLevel(log.InfoLevel)

	println("===========")
	println(" WordServe ")
	println("===========")
	log.Infof("Version: %s", Version)
	log.Infof("Process ID: [ %d ]", pid)
	log.Info("init: OK")
	log.Infof("data dir: ( %s )", dataDir)
	log.Info("status: ready")
	println("===========")
	println("Press Ctrl+C to exit")

	log.SetLevel(currentLevel)
}
