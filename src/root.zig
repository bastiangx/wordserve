//! A Trie implementation for fast prefix-based word suggestions.
//!
//! Provides a Trie data structure prefix-based word completion:
//!   - Init a Trie.
//!   - Inserts words with frequencies.
//!
const std = @import("std");
const ArrayList = std.ArrayList;
const AutoHashMap = std.AutoHashMap;
const Allocator = std.mem.Allocator;

const MAX_WORD_LENGTH: usize = 256;

/// Represents a node in the Trie.
const TrieNode = struct {
    /// Characters to child nodes.
    children: AutoHashMap(u8, *TrieNode),
    /// If this node is the end of a valid word.
    is_word: bool,
    /// Frequency of the word ending at this node.
    frequency: u8,

    /// Initializes a new TrieNode (default constructor).
    ///
    /// Allocates mem for the node and its children map.
    ///
    /// # Arguments
    ///
    /// * `allocator` - General purpose allocator.
    fn init(allocator: Allocator) !*TrieNode {
        const node = try allocator.create(TrieNode);
        node.* = .{
            .children = AutoHashMap(u8, *TrieNode).init(allocator),
            .is_word = false,
            .frequency = 0,
        };
        return node;
    }

    /// Deinitializes a TrieNode.
    ///
    /// Recursively deinit all child nodes and frees the mem.
    ///
    /// # Arguments
    ///
    /// * `allocator` - General purpose allocator.
    fn deinit(self: *TrieNode, allocator: Allocator) void {
        var it = self.children.iterator();
        while (it.next()) |entry| {
            entry.value_ptr.*.deinit(allocator);
            allocator.destroy(entry.value_ptr.*);
        }
        self.children.deinit();
    }
};

/// Suggestion for a given prefix.
const Suggestion = struct {
    word: []const u8,
    frequency: u8,
};

const Trie = struct {
    root: *TrieNode,
    allocator: Allocator,

    fn init(allocator: Allocator) !Trie {
        return Trie{
            .root = try TrieNode.init(allocator),
            .allocator = allocator,
        };
    }

    fn deinit(self: *Trie) void {
        self.root.deinit(self.allocator);
        self.allocator.destroy(self.root);
    }

    /// Inserts a word into the Trie.
    ///
    /// # Arguments
    ///
    /// * `word` - The actual word to insert.
    /// * `freq` - The frequency of the word.
    fn insert(self: *Trie, word: []const u8, freq: u8) !void {
        var node = self.root;
        for (word) |c| {
            // if char is not in child map, add a new node
            if (!node.children.contains(c)) {
                const new_node = try TrieNode.init(self.allocator);
                try node.children.put(c, new_node);
            }
            node = node.children.get(c).?;
        }
        node.is_word = true;
        node.frequency = freq;
    }

    /// Finds suggestions for a given prefix.
    ///
    /// # Arguments
    ///
    /// * `prefix` - The prefix to search for.
    /// * `results` - ArrayList to store the suggestions.

    // TODO: rn, the algo for sorting, copies all the results from the trie into a temp arraylist,
    // sorts it and then copies the top 5 results into the final arraylist.
    // current complexity is O(nlogn) where n is the number of words in the trie. not good
    // can be optimized by using a min heap of fixed size
    // reduces the complexity to O(nlogk) where k is the number of suggestions to return, better.
    fn findSuggestions(self: *Trie, prefix: []const u8, results: *ArrayList(Suggestion)) !void {
        var node = self.root;

        // default Trie traversal
        for (prefix) |c| {
            if (node.children.get(c)) |next| {
                node = next;
            } else return;
        }

        // store temp results to sort by frequency
        var temp_results = ArrayList(Suggestion).init(self.allocator);
        defer {
            for (temp_results.items) |item| {
                self.allocator.free(item.word);
            }
            temp_results.deinit();
        }

        var word_buf = ArrayList(u8).init(self.allocator);
        defer word_buf.deinit();

        // Initialize word_buf with the prefix
        try word_buf.appendSlice(prefix);

        try self.collectWords(node, &word_buf, &temp_results);

        // sort by freq
        std.sort.heap(Suggestion, temp_results.items, {}, struct {
            fn lessThan(_: void, a: Suggestion, b: Suggestion) bool {
                return a.frequency > b.frequency; // reverse order for highest
            }
        }.lessThan);

        // limiting
        const limit = @min(temp_results.items.len, 5);
        for (temp_results.items[0..limit]) |suggestion| {
            const word = try self.allocator.dupe(u8, suggestion.word);
            const suggestion_item = Suggestion{ .word = word, .frequency = suggestion.frequency };
            try results.append(suggestion_item);
        }
    }

    /// Recursively collects words from Trie starting from a given node.
    /// DFS traversal to collect.
    ///
    /// # Arguments
    ///
    /// * `node` - The node to start collecting words from.
    /// * `word_buf` - Buffer to store the word being built.
    /// * `results` - An ArrayList to store the collected words.
    fn collectWords(self: *Trie, node: *TrieNode, word_buf: *ArrayList(u8), results: *ArrayList(Suggestion)) !void {
        // early return if the word is complete
        if (node.is_word) {
            const word = try self.allocator.dupe(u8, word_buf.items);
            try results.append(.{
                .word = word,
                .frequency = node.frequency,
            });
        }

        var it = node.children.iterator();
        while (it.next()) |entry| {
            // collect words
            if (word_buf.items.len < MAX_WORD_LENGTH) {
                try word_buf.append(entry.key_ptr.*);
                try self.collectWords(entry.value_ptr.*, word_buf, results);
                _ = word_buf.pop(); // Backtrack: remove the last char for the next branch
            }
        }
    }
};

/// Loads words from a corpus file into the trie.
/// Format expected: one word per line, optionally with frequency.
/// If frequency is not provided, it will be derived from word length
/// (shorter words are generally more frequent in natural language).
fn loadCorpus(trie: *Trie, file_path: []const u8) !usize {
    const file = try std.fs.cwd().openFile(file_path, .{});
    defer file.close();

    var buf_reader = std.io.bufferedReader(file.reader());
    var in_stream = buf_reader.reader();

    var buf: [MAX_WORD_LENGTH]u8 = undefined;
    var count: usize = 0;

    // Read the file line by line
    while (try in_stream.readUntilDelimiterOrEof(&buf, '\n')) |line| {
        if (line.len == 0) continue;
        
        // Split on whitespace or tab if format is "word freq"
        var parts = std.mem.splitAny(u8, line, " \t");
        const word = parts.first();
        
        // parse freq
        var freq: u8 = undefined;
        if (parts.next()) |freq_str| {
            freq = try std.fmt.parseInt(u8, freq_str, 10);
        } else {
            // Simple frequency heuristic based on word length
            // Alternative simpler approach
            const word_len: u8 = @intCast(word.len);
            // Cast to u16 for intermediate calculation to avoid overflow
            const score: u16 = @as(u16, word_len) * 10;
            freq = 255 - @as(u8, @min(score, 200));
        }
        
        if (word.len > 0 and word.len < MAX_WORD_LENGTH) {
            try trie.insert(word, freq);
            count += 1;
        }
    }
    
    return count;
}

fn testQueries(trie: *Trie, queries: []const []const u8, allocator: Allocator) !void {
    std.debug.print("\n=== Testing Trie Suggestions ===\n\n", .{});
    
    for (queries) |query| {
        var results = ArrayList(Suggestion).init(allocator);
        defer {
            for (results.items) |suggestion| {
                allocator.free(suggestion.word);
            }
            results.deinit();
        }

        try trie.findSuggestions(query, &results);
        
        std.debug.print("Query: \"{s}\"\n", .{query});
        if (results.items.len == 0) {
            std.debug.print("  No suggestions found\n", .{});
        } else {
            for (results.items) |suggestion| {
                std.debug.print("  {s} ({d})\n", .{suggestion.word, suggestion.frequency});
            }
        }
        std.debug.print("\n", .{});
    }
}

pub fn main() !void {
    var gpa = std.heap.GeneralPurposeAllocator(.{}){};
    defer _ = gpa.deinit();
    const allocator = gpa.allocator();

    var trie = try Trie.init(allocator);
    defer trie.deinit();

    const corpus_path = "src/words"; 
    
    // Define test queries
    const queries = [_][]const u8{
        "th",
        "pro",
        "ex",
        "com",
        "z", 
    };

    std.debug.print("Loading corpus from {s}...\n", .{corpus_path});
    const word_count = loadCorpus(&trie, corpus_path) catch |err| {
        std.debug.print("Error loading corpus: {any}\n", .{err});
        
        // Fallback to hardcoded words
        std.debug.print("Falling back to hardcoded words\n", .{});
        try trie.insert("the", 100);
        try trie.insert("there", 80);
        try trie.insert("their", 75);
        try trie.insert("they", 70);
        try trie.insert("then", 65);
        try trie.insert("that", 95);
        try trie.insert("programming", 85);
        try trie.insert("problem", 82);
        try trie.insert("process", 78);
        try trie.insert("executive", 60);
        try trie.insert("example", 90);
        try trie.insert("expert", 75);
        try trie.insert("computer", 95);
        try trie.insert("complete", 80);
        try trie.insert("command", 75);
        try trie.insert("zero", 50);
        try trie.insert("zoo", 45);
        
        // Return hardcoded word count instead of an integer
        return testQueries(&trie, &queries, allocator);
    };
    
    std.debug.print("Loaded {d} words into the trie\n", .{word_count});
    
    try testQueries(&trie, &queries, allocator);
}
