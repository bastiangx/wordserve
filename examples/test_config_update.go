package main

import (
	"fmt"
	"os"

	"github.com/bastiangx/typr-lib/pkg/config"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run test_config_update.go <test_name>")
		fmt.Println("Tests: read, update, verify")
		os.Exit(1)
	}

	test := os.Args[1]
	configPath := "typer-config.toml"

	switch test {
	case "read":
		testReadConfig(configPath)
	case "update":
		testUpdateConfig(configPath)
	case "verify":
		testVerifyConfig(configPath)
	default:
		fmt.Printf("Unknown test: %s\n", test)
		os.Exit(1)
	}
}

func testReadConfig(configPath string) {
	fmt.Println("=== Testing Config Read ===")
	
	config, err := config.LoadOrCreate(configPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Server Config:\n")
	fmt.Printf("  MaxLimit: %d\n", config.Server.MaxLimit)
	fmt.Printf("  MinPrefix: %d\n", config.Server.MinPrefix)
	fmt.Printf("  MaxPrefix: %d\n", config.Server.MaxPrefix)
	fmt.Printf("  EnableFilter: %t\n", config.Server.EnableFilter)

	fmt.Printf("Dict Config:\n")
	fmt.Printf("  MaxWords: %d\n", config.Dict.MaxWords)
	fmt.Printf("  ChunkSize: %d\n", config.Dict.ChunkSize)
	fmt.Printf("  MaxHotWords: %d\n", config.Dict.MaxHotWords)
	fmt.Printf("  MinFreqThreshold: %d\n", config.Dict.MinFreqThreshold)
}

func testUpdateConfig(configPath string) {
	fmt.Println("=== Testing Config Update ===")
	
	config, err := config.LoadOrCreate(configPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Before update: MaxLimit=%d, EnableFilter=%t\n", 
		config.Server.MaxLimit, config.Server.EnableFilter)

	// Test updating config
	newMaxLimit := 30
	newEnableFilter := false
	
	err = config.Update(configPath, &newMaxLimit, nil, nil, &newEnableFilter)
	if err != nil {
		fmt.Printf("Failed to update config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("After update: MaxLimit=%d, EnableFilter=%t\n", 
		config.Server.MaxLimit, config.Server.EnableFilter)
	fmt.Println("Config updated and saved to file")
}

func testVerifyConfig(configPath string) {
	fmt.Println("=== Testing Config Verification ===")
	
	// Reload from file to verify persistence
	config, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("Failed to reload config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Reloaded from file:\n")
	fmt.Printf("  MaxLimit: %d\n", config.Server.MaxLimit)
	fmt.Printf("  EnableFilter: %t\n", config.Server.EnableFilter)

	// Check if the values persisted correctly
	if config.Server.MaxLimit == 30 && !config.Server.EnableFilter {
		fmt.Println("✅ Config persistence verified - values match!")
	} else {
		fmt.Println("❌ Config persistence failed - values don't match")
		os.Exit(1)
	}
}
