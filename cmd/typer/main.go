// Package main has the entry point for the Typr Server and CLI app.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/bastiangx/typr-lib/internal/cli"
	"github.com/bastiangx/typr-lib/internal/utils"
	"github.com/bastiangx/typr-lib/pkg/config"
	"github.com/bastiangx/typr-lib/pkg/server"
	completion "github.com/bastiangx/typr-lib/pkg/suggest"
	"github.com/charmbracelet/log"
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
	binaryDir := flag.String("data", "data/", "Directory containing the binary files")
	debugMode := flag.Bool("d", false, "Toggle debug mode")
	cliMode := flag.Bool("c", false, "Run in CLI input handler mode")
	limit := flag.Int("limit", defaultConfig.CLI.DefaultLimit, "Number of suggestions to return")
	minPrefix := flag.Int("prmin", defaultConfig.CLI.DefaultMinLen, "Minimum prefix length for suggestions (1 < n <= prmax)")
	maxPrefix := flag.Int("prmax", defaultConfig.CLI.DefaultMaxLen, "Maximum prefix length for suggestions")
	noFilter := flag.Bool("no-filter", defaultConfig.CLI.DefaultNoFilter, "Disable input filtering (DBG only) - shows all raw dictionary entries (numbers, symbols, etc)")
	wordLimit := flag.Int("words", defaultConfig.Dict.MaxWords, "Maximum number of words to load (use 0 for all words)")
	chunkSize := flag.Int("chunk", defaultConfig.Dict.ChunkSize, "Number of words per chunk for lazy loading")

	flag.Parse()

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
	var pid = os.Getpid()
	currentLevel := log.GetLevel()
	log.SetLevel(log.InfoLevel)

	println("=======")
	println(" TYPER  ")
	println("=======")
	log.Infof("Process ID: [ %d ]", pid)
	log.Info("init: OK")
	log.Infof("data dir: ( %s )", dataDir)
	log.Info("status: ready")
	println("=======")
	println("Press Ctrl+C to exit")

	log.SetLevel(currentLevel)
}
