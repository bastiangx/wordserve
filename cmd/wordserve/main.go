// Copyright 2025 The WordServe Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

/*
Package main implements the WordServe server and commandline interface.

WordServe has prefix based word completion using a radix trie data structure
with frequency ranking. It can operate as a MessagePack IPC server
for editor/generic client integrations or as a standalone CLI for interactive testing.

# Server Mode

The server uses lazy, chunked dictionaries to manage large word
datasets with low memory overhead. Words are ranked by frequency and filtered
using configurable thresholds to deliver relevant suggestions.

# CLI Mode

The CLI provides an interactive shell for debugging and testing the completion
engine's functionality.

# Data Files

The data directory must contain dictionary files named `dict_0001.bin`,
`dict_0002.bin`, etc., along with a `words.txt` file. If these files are
missing, the application will attempt to generate them locally or download them
from the project's GitHub releases page.

# Config

Runtime configuration is managed via a `config.toml` file, which supports
settings for the server, dictionary, and CLI. A default configuration is
created automatically if one does not exist.
*/
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/bastiangx/wordserve/internal/cli"
	"github.com/bastiangx/wordserve/pkg/config"
	"github.com/bastiangx/wordserve/pkg/server"
	completion "github.com/bastiangx/wordserve/pkg/suggest"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
)

const (
	Version = "0.1.0-beta"
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

	showVersion := flag.Bool("version", false, "Show current version")
	configFile := flag.String("config", "", "Path to custom config.toml file")
	binaryDir := flag.String("data", "data/", "Directory containing the binary files")
	debugMode := flag.Bool("v", false, "Toggle verbose mode")
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
		logger.Print("[WordServe] Serves really Fast word completions!")
		logger.Print("", "version", Version)
		logger.Print("")
		logger.Print("use --help to see available options")
		logger.Print("")
		logger.Print("Find out more at", "gh", gh)

		os.Exit(0)
	}

	if *debugMode {
		log.SetLevel(log.DebugLevel)
		log.SetReportTimestamp(true)
	} else {
		log.SetLevel(log.WarnLevel)
	}

	resolvedDataDir := *binaryDir

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

	appConfig, configPath, err := config.LoadConfigWithPriority(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
		os.Exit(1)
	}
	log.Debugf("Using config file: %s", configPath)
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
