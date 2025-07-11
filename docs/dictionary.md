
# Dictionary Design


WordServe uses a chunked binary dictionary system.

Instead of loading massive word lists all at once, the corpus gets splits into smaller pieces that can be loaded and unloaded on demand.

#### Chunks & binary files?

The design centers around practicality and performance:

- **Trie**: The trie structure allows for fast prefix matching without loading the entire dictionary into memory
- **Memory control**: Load only what you need via config (30K words vs 500K+ words)
- **Faster startup**: Skip the parsing overhead of text files
- **Runtime**: Adjust dictionary size without restarting the server
- **Storage**: Binary format ~40% smaller than equivalent text

Each chunk contains about **10,000 words** by default, stored as `dict_0001.bin`, `dict_0002.bin`, etc.

#### Building the dictionary

1. **Source data**: Starts with `data/words.txt` (word + frequency pairs)
2. **Ranking conversion**: Converts frequencies to ranks (1 = most frequent)
3. **Trie building**: Creates prefix tries in memory for each chunk
4. **Binary serialization**: Saves tries to `.bin` files

### Internal Format

Each `.bin` file uses a compact struc:

```
Header: [4 bytes] - Word count (int32)
Entries: For each word:
  [2 bytes] - Word length (uint16)  
  [N bytes] - Word string (UTF-8)
  [2 bytes] - Frequency rank (uint16)
```

> _ranks instead of raw frequencies?_ mem optimization. Instead of storing freq `234,567` (4+ bytes), we store rank `42` (2 bytes). The loader converts ranks back to scores using `score = 65535 - rank + 1`, so rank 1 becomes the highest score.

### Loading & tries

When chunks load into memory, WordServe builds **Patricia radix tries** for prefix matching:

- **O(k) lookups** where k = prefix length (not dict size)
- _mem sharing_: Common prefixes stored once in the trie
- _Background_: Chunks load in separate goroutines
- _Priority order_: Most frequent words (top used in english) (chunk 1) load first
- _Automatic fallback_: Missing chunks get downloaded from the repo or generated via luajit

#### Chunk Lifecycle

1. _Available_: Chunk file exists on disk
2. _Loading_: Background reading chunk data
3. _Loaded_: Words inserted into active trie
4. _Evicted_: Removed from memory, trie rebuilt
