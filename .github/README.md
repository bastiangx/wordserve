# Typr Lib

A lightweight word completion engine based on radix trie data structure, designed for real-time word suggestions as users type. The system prioritizes memory efficiency, speed, and relevance of suggestions.

## Technical Design

### Core Components

1. **Radix Trie Implementation**
   - Using the `radix_trie` crate for efficient prefix-based lookups
   - Compressed structure to minimize memory footprint
   - Fast retrieval of all words sharing a common prefix
   - Support for frequency-based suggestion ranking

2. **Suggestion Generator**
   - Accepts a word prefix (minimum 1-2 characters)
   - Traverses trie to find matching prefix node
   - Collects all words sharing that prefix
   - Ranks suggestions by frequency/usage statistics
   - Returns top N matches (configurable)

3. **Word Entry Metadata**
   - Frequency data for ranking completions
   - Optional domain tagging (e.g., medical, technical)
   - Optional descriptions for additional context
   - Support for abbreviations and custom shortcuts

### Algorithm Details

1. **Insertion (Dictionary Building)**
   - O(m) time complexity where m = word length
   - Words inserted with associated frequency and metadata
   - One-time operation during initialization

2. **Lookup (Suggestion Generation)**
   - O(p) for finding prefix node (p = prefix length)
   - O(k) for collecting words (k = total characters in all matching words)
   - O(n log n) for sorting suggestions by frequency
   - Demonstrated sub-microsecond performance for typical queries

## Todo Next Steps

### High Priority
1. **Load real frequency data** - Replace sample dictionary with real word frequency data from a corpus
2. **Add fuzzy matching** - Find a suitable crate or implement basic edit distance for typo tolerance
3. **Input validation** - Platform-agnostic character handling and normalization
4. **Cache frequent prefixes** - Optimize for common searches using a caching layer (cleanups, expiry)
5. **Handle special characters** - handle punctuation, whitespace, and special characters in input, ignore special characters in suggestions

### Medium Priority
6. **Multiple language support** - Common interface for switching between different language dictionaries
7. **User dictionary management** - Support for user-specific words and preferences

### Lower Priority
8. **Abbreviation expansion** - Custom shortcuts defined by users (e.g., "ty" → "thank you")
9. **Domain-specific dictionaries** - Specialized vocabularies (medical, legal, etc.)
10. **UI considerations** - Dictionary context indicators, themes, layouts

### Architecture

```txt
                     ┌───────────────────┐
                     │   Core Rust Code  │
                     │  (radix_trie etc) │
                     └─────────┬─────────┘
                               │
                 ┌─────────────┴──────────────┐
                 │                            │
        ┌────────▼──────────┐      ┌─────────▼─────────┐
        │    Native Build   │      │    WASM Build     │
        │  (dynamic library)│      │ (WebAssembly file)│
        └────────┬──────────┘      └─────────┬─────────┘
                 │                            │
        ┌────────▼──────────┐      ┌─────────▼─────────┐
        │     FFI Layer     │      │  JavaScript Glue  │
        │  (ts_binder.ts)   │      │    (auto-gen)     │
        └────────┬──────────┘      └─────────┬─────────┘
                 │                            │
                 └─────────────┬──────────────┘
                               │
                     ┌─────────▼─────────┐
                     │ Common TypeScript │
                     │    Interface      │
                     └───────────────────┘
```

## Usage

The engine accepts partial word input and returns the most likely word completions based on frequency data from the preloaded dictionary.

### Input Requirements
- Partial word (typically 1+ characters)
- Dictionary with word frequency data

### Output Format
- Array of suggestions, each containing:
  - Complete word
  - Frequency/usage score
  - Optional metadata (domain, description)

## Performance Characteristics

- **Memory Footprint**: Minimal due to compressed radix trie structure
- **Lookup Speed**: Microsecond response times for typical prefix queries
- **Accuracy**: Directly correlates with quality of frequency data