#!/bin/bash
set -e

# Force use the correct Go version for TinyGo
export GOROOT=$(go env GOROOT)
export PATH="$GOROOT/bin:$PATH"

echo "Building WASM with TinyGo..."
echo "Using Go: $(which go)"
echo "Go version: $(go version)"
echo "TinyGo version: $(tinygo version)"

# Build WASM
tinygo build -o wordserve.wasm -target wasm -no-debug -opt=2 ./cmd/wasm

echo "WASM build complete: wordserve.wasm"
ls -la wordserve.wasm
