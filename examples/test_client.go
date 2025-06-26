package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/bastiangx/typr-lib/pkg/server"
	"github.com/vmihailenco/msgpack/v5"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run test_client.go <prefix> [limit]")
		os.Exit(1)
	}

	prefix := os.Args[1]
	limit := 10
	if len(os.Args) > 2 {
		fmt.Sscanf(os.Args[2], "%d", &limit)
	}

	// Create request
	request := server.CompletionRequest{
		Prefix: prefix,
		Limit:  limit,
	}

	// Encode request
	requestData, err := msgpack.Marshal(request)
	if err != nil {
		fmt.Printf("Failed to encode request: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Encoded request (%d bytes): %x\n", len(requestData), requestData)

	// Start typer server
	cmd := exec.Command("./typer", "-d") // debug mode
	cmd.Stderr = os.Stderr // Keep stderr for debug logs

	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Printf("Failed to get stdin pipe: %v\n", err)
		os.Exit(1)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("Failed to get stdout pipe: %v\n", err)
		os.Exit(1)
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		fmt.Printf("Failed to start typer: %v\n", err)
		os.Exit(1)
	}

	// Send request
	fmt.Printf("Sending request...\n")
	if _, err := stdin.Write(requestData); err != nil {
		fmt.Printf("Failed to write request: %v\n", err)
		os.Exit(1)
	}
	// Close stdin to signal end of input
	stdin.Close()

	// Read complete response
	fmt.Printf("Reading response...\n")
	var responseData []byte
	buffer := make([]byte, 1024)
	for {
		n, err := stdout.Read(buffer)
		if n > 0 {
			responseData = append(responseData, buffer[:n]...)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Printf("Failed to read response: %v\n", err)
			os.Exit(1)
		}
	}
	fmt.Printf("Received complete response (%d bytes): %x\n", len(responseData), responseData)

	// Decode response
	var response server.CompletionResponse
	if err := msgpack.Unmarshal(responseData, &response); err != nil {
		// Try error response
		var errorResponse server.CompletionError
		if err2 := msgpack.Unmarshal(responseData, &errorResponse); err2 == nil {
			fmt.Printf("Error: %s (code: %d)\n", errorResponse.Error, errorResponse.Code)
			os.Exit(1)
		}
		
		// Try to decode as generic interface to see what we got
		var genericResponse interface{}
		if err3 := msgpack.Unmarshal(responseData, &genericResponse); err3 == nil {
			fmt.Printf("Raw decoded response: %+v\n", genericResponse)
		}
		
		fmt.Printf("Failed to decode response: %v\n", err)
		os.Exit(1)
	}

	// Display results
	fmt.Printf("Completion Results for '%s':\n", prefix)
	fmt.Printf("Count: %d\n", response.Count)
	fmt.Printf("Time: %d microseconds\n", response.TimeTaken)
	fmt.Println("Suggestions:")
	for i, suggestion := range response.Suggestions {
		fmt.Printf("  %d. %s (rank: %d)\n", 
			i+1, suggestion.Word, suggestion.Rank)
	}

	// Wait for process to finish
	cmd.Wait()
}
