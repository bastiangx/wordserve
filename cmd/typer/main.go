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
	// exportBin := flag.String("export", "", "Export path for binary dictionary file")
	binaryDir := flag.String("data", "data/", "Directory containing typer's resource binary files (default: data/)")
	debugMode := flag.Bool("d", false, "Toggle debug mode")
	cliMode := flag.Bool("c", false, "Run in CLI input handler mode")
	limit := flag.Int("limit", 24, "Number of suggestions to return (default: 24)")
	minPrefix := flag.Int("prmin", 1, "Minimum prefix length for suggestions (default: 1)")
	maxPrefix := flag.Int("prmax", 24, "Maximum prefix length for suggestions (default: 24)")
	noFilter := flag.Bool("no-filter", false, "Disable input filtering (for debugging - shows all dictionary entries)")

	flag.Parse()

	// debugmode wip -- neds logic checks in the other packages
	// needs to be gloabl var for package. read TODO on how
	if *debugMode {
		log.SetLevel(log.DebugLevel)
		log.SetReportTimestamp(false)
	} else {
		log.SetLevel(log.ErrorLevel)
	}

	// TODO: newCompleter doesnt return any errors too and should be modified.
	completer := completion.NewCompleter()

	if *binaryDir != "" {
		log.Debug("Loading tries | ngrams from", "dir", *binaryDir)
		err := completer.LoadAllBinaries(*binaryDir)
		if err != nil {
			log.Fatalf("Binaries not found: %v", err)
			os.Exit(1)
		}
		log.Debug("Binary dictionaries loaded successfully")
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

	// if *exportBin != "" {
	// 	fmt.Fprintf(os.Stderr, "Exporting binary dictionary to %s...\n", *exportBin)
	// 	if err := completer.SaveBinaryDictionary(*exportBin); err != nil {
	// 		fmt.Fprintf(os.Stderr, "Error exporting binary dictionary: %v\n", err)
	// 	} else {
	// 		fmt.Fprintf(os.Stderr, "Binary dictionary exported successfully\n")
	// 	}
	// }

}
