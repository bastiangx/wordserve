# Typr Lib

A lightweight word completion engine based on prefix tree (trie) data structure, designed for real-time word suggestions as users type. The system prioritizes memory efficiency, speed, and relevance of suggestions.

#### Actual Usage

When a user types a partial word, the engine suggests the most likely completions based on the frequency of words in the dictionary. The suggestions are ranked by usage statistics to provide the most relevant options.
So, the engine needs to only provide the most likely completions based on the frequency of words in the dictionary ( 1 word suggestion is enough).

## Technical Design

### Core Components

1. **Trie Data Structure**
   - Character-indexed tree with each node representing a letter
   - Nodes contain:
     - HashMap of child nodes (character → node pointer)
     - Boolean flag for word termination
     - Frequency value for word ranking
   - Path from root to any node spells a word or prefix

2. **Suggestion Generator**
   - Accepts a word prefix (minimum 2 characters)
   - Traverses trie to find matching prefix node
   - Recursively collects all words sharing that prefix
   - Ranks suggestions by frequency/usage statistics
   - Returns top N matches (currently 5)

3. **Memory Management**
   - Custom allocation strategy with explicit memory handling
   - Compact node representation (u8 for frequency values)
   - Fixed-size buffer for word collection (64 bytes)
   - Proper cleanup routines to prevent memory leaks

### Algorithm Details

1. **Insertion (Dictionary Building)**
   - O(m) time complexity where m = word length
   - Each character creates/traverses a node
   - Word termination marked at final node
   - Frequency stored for ranking purposes

2. **Lookup (Suggestion Generation)**
   - O(p) for finding prefix node (p = prefix length)
   - O(k) for collecting words (k = total characters in all matching words)
   - O(n log n) for sorting suggestions by frequency
   - Overall bounded by dictionary size and prefix commonality

3. **Optimization Techniques**
   - Hash-based child lookup for O(1) traversal between nodes
   - Early termination when prefix not found
   - Frequency-based ranking to prioritize common words
   - Buffer reuse to minimize allocations during traversal

## Usage

The engine accepts partial word input (minimum 2 characters) and returns the most likely word completions based on frequency data from the preloaded dictionary.

### Input Requirements
- Partial word (2+ characters)
- Dictionary with word frequency data

### Output Format
- Array of suggestions, each containing:
  - Complete word
  - Frequency/usage score

## Performance Characteristics

- **Memory Footprint**: Scales with dictionary size, optimized through compact representation
- **Lookup Speed**: Sub-millisecond response for typical prefix queries
- **Accuracy**: Directly correlates with quality of frequency data

## Future Enhancements

1. **Memory Optimizations**
   - Compressed node representation
   - Path compression for common prefixes
   - Memory-mapped dictionary files

2. **Algorithm Improvements**
   - Context-aware suggestions (n-gram integration)
   - Edit distance calculation for spelling correction
   - Dynamic frequency adjustment based on user behavior

3. **Feature Extensions**
   - Multi-language support
   - Domain-specific dictionaries
   - User-specific custom dictionaries

## Implementation Notes

The current implementation provides a foundational word completion engine with particular attention to memory management, performance, and result quality. The design favors simplicity and efficiency over complex features, making it suitable for integration into text editors, input methods, and other text-processing applications where real-time performance is critical.
