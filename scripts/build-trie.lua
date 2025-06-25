-- Builds a trie directly from a word frequency file
-- Input: ../data/words.txt (format: "word <tab> frequency")
-- Output: ../data/word_trie.bin
-- Requires LuaJIT with FFI support

local ffi = require("ffi")

local verbose = false
local show_help = false

for i = 1, #arg do
    if arg[i] == "-v" or arg[i] == "--verbose" then
        verbose = true
        break
    elseif arg[i] == "-s" then
        verbose = false
        break
    elseif arg[i] == "-h" or arg[i] == "--help" then
        show_help = true
        break
    end
end

if show_help then
    print("")
    print("Usage: luajit build-trie.lua [ OPTIONS ]")
    print("")
    print("Builds a trie directly from a word frequency file.")
    print("Input: ../data/words.txt (format: 'word <tab> frequency')")
    print("")
    print("Options:")
    print("  -h, --help      Show this help message")
    print("  -v, --verbose   Verbose output")
    print("  -s              Silent mode (default)")
    print("")
    print("Find out more at the source: https://github.com/bastiangx/typr-lib")
    os.exit(0)
end

local function dbg_print(...)
    if verbose then
        print(...)
    end
end

local function err_print(...)
    print(...)
end

local Trie = {}
Trie.__index = Trie

function Trie.new()
    return setmetatable({
        children = {},
        is_word = false,
        frequency = 0,
    }, Trie)
end

function Trie:insert(word, freq)
    local node = self
    for i = 1, #word do
        local char = word:sub(i, i)
        node.children[char] = node.children[char] or Trie.new()
        node = node.children[char]
    end
    node.is_word = true
    node.frequency = (node.frequency or 0) + freq
end

-- Counts table entries by iterating through the table
function Count_table_entries(t)
    local count = 0
    for _ in pairs(t) do
        count = count + 1
    end
    return count
end


-- Serialize the trie structure by traversing it
function Save_trie(trie, filename, word_data)
    local file = io.open(filename, "wb")
    if not file then
        err_print("Error: Could not create file " .. filename)
        return
    end

    -- Write the number of words
    local count = Count_table_entries(word_data)
    file:write(ffi.string(ffi.new("int32_t[1]", count), 4))
    dbg_print("Serializing trie with " .. count .. " words...")

    -- Serialize the trie structure using DFS
    local nodes_processed = 0
    local function serialize_node(node, prefix)
        if node.is_word then
            -- Write this word and its frequency
            local word_len = #prefix
            file:write(ffi.string(ffi.new("uint16_t[1]", word_len), 2))
            file:write(prefix)
            file:write(ffi.string(ffi.new("uint32_t[1]", node.frequency), 4))

            nodes_processed = nodes_processed + 1
            if nodes_processed % 10000 == 0 then
                dbg_print(string.format("Serialized %d/%d words", nodes_processed, count))
            end
        end

        -- Sort children for deterministic output
        local keys = {}
        for k in pairs(node.children) do
            table.insert(keys, k)
        end
        table.sort(keys)

        -- each child node
        for _, char in ipairs(keys) do
            serialize_node(node.children[char], prefix .. char)
        end
    end

    -- empty prefix for root node
    serialize_node(trie, "")

    file:close()
    dbg_print("Trie successfully serialized at " .. filename)
end

-- Load frequencies and words from the new format
-- Format: "word <tab> frequency"
function Load_frequencies(file)
    local freq_data = {}
    for line in io.lines(file) do
        local word, freq = line:match("(.-)\t(%d+)")
        if word and freq then
            freq_data[word] = tonumber(freq)
        end
    end
    return freq_data
end

-- Build trie from direct frequency file
dbg_print("Building trie from frequencies file...")
local words_with_frequencies = Load_frequencies("../data/words.txt")

if not words_with_frequencies or Count_table_entries(words_with_frequencies) == 0 then
    err_print("Error: Failed to load frequencies from ../data/words.txt")
    os.exit(1)
end

-- Create and populate trie
local word_trie = Trie.new()
dbg_print("Building word trie...")
local word_count = 0

for word, freq in pairs(words_with_frequencies) do
    word_trie:insert(word, freq)
    word_count = word_count + 1
    if word_count % 10000 == 0 then
        dbg_print(string.format("Added %d words to the trie", word_count))
        collectgarbage("step")
    end
end

Save_trie(word_trie, "../data/word_trie.bin", words_with_frequencies)
