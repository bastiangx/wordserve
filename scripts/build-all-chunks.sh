#!/bin/bash

# Build all dictionary chunks for lazy loading
# This script generates chunked binary files from the words.txt file

set -euo pipefail  # Stricter error handling

# Get absolute path to script directory and data directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
DATA_DIR="$PROJECT_ROOT/data"

# Ensure we're in the script directory for execution
cd "$SCRIPT_DIR"

echo "Building all dictionary chunks..."
echo "Data directory: $DATA_DIR"

# Check if words.txt exists
if [[ ! -f "$DATA_DIR/words.txt" ]]; then
  echo "Error: $DATA_DIR/words.txt not found"
  echo "Please ensure the words.txt file is present in the data directory"
  exit 1
fi

# Check if luajit is available
if ! command -v luajit &>/dev/null; then
  echo "Error: luajit is required but not installed"
  echo "Please install luajit: brew install luajit"
  exit 1
fi

# Remove existing chunk files
echo "Removing existing chunk files..."
rm -f "$DATA_DIR"/dict_*.bin

# Run the chunk builder
echo "Generating chunks (this may take a few minutes)..."
cd "$SCRIPT_DIR"
luajit build-trie.lua --chunk-size 10000 --verbose

# Count generated chunks
CHUNK_COUNT=$(ls -1 "$DATA_DIR"/dict_*.bin 2>/dev/null | wc -l)
echo
echo "✅ Successfully generated $CHUNK_COUNT chunks"
echo "📊 Total words in dictionary: $(wc -l <"$DATA_DIR/words.txt")"
echo "📦 Each chunk contains ~10,000 words"
echo
echo "Usage examples:"
echo "  # Load default 50K words:"
echo "  ./typer -c"
echo
echo "  # Load 100K words:"
echo "  ./typer -c --words 100000"
echo
echo "  # Load all words:"
echo "  ./typer -c --words 0"
echo
echo "  # Debug mode with 200K words:"
echo "  ./typer -d -c --words 200000"
