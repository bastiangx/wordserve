-- Builds a trie from the unigrams.bin file
-- Run `build-ngrams.lua` first to generate unigrams.bin
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
    print("Builds a trie from the unigrams.bin file.")
    print("Run 'build-ngrams.lua' first to generate the unigrams.bin")
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

function Load_unigrams(filename)
    local unigrams = {}
    local file = io.open(filename, "rb")
    if not file then
        err_print("Error: Could not find unigram file " .. filename)
        err_print("Please run $ luajit build-ngram.lua first to generate unigrams.bin")
        return unigrams
    end

    -- header--first 4 bytes contains the number of unigrams
    local count_bytes = file:read(4)
    if not count_bytes or #count_bytes < 4 then
        err_print("Error: Invalid unigram file header or corrupted file!")
        file:close()
        return unigrams
    end

    local count_ptr = ffi.cast("int32_t*", ffi.new("char[4]", count_bytes))
    local count = count_ptr[0]

    dbg_print("Reading " .. count .. " unigrams from " .. filename)

    local entries_read = 0
    for i = 1, count do
        -- Read word length
        local len_bytes = file:read(2)
        if not len_bytes or #len_bytes < 2 then
            err_print("Error: Unexpected EOF in unigram file at entry " .. i)
            break
        end

        local len_ptr = ffi.cast("uint16_t*", ffi.new("char[2]", len_bytes))
        local word_len = len_ptr[0]

        -- Read word
        local word = file:read(word_len)
        if not word or #word < word_len then
            err_print("Error: Unexpected EOF in unigram file at entry " .. i)
            break
        end

        -- Read frequency
        local freq_bytes = file:read(4)
        if not freq_bytes or #freq_bytes < 4 then
            err_print("Error: Unexpected EOF in unigram file at entry " .. i)
            break
        end

        local freq_ptr = ffi.cast("uint32_t*", ffi.new("char[4]", freq_bytes))
        local freq = freq_ptr[0]

        unigrams[word] = freq
        entries_read = entries_read + 1

        if entries_read % 100000 == 0 then
            dbg_print(string.format("Read %d/%d entries", entries_read, count))
        end
    end

    file:close()
    dbg_print("Loaded " .. entries_read .. " unigrams")
    return unigrams
end

-- Serialize the trie structure by traversing it
function Save_trie(trie, filename, unigrams)
    local file = io.open(filename, "wb")
    if not file then
        err_print("Error: Could not create file " .. filename)
        return
    end

    -- Write the number of unigrams
    local count = Count_table_entries(unigrams)
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

dbg_print("Building trie from unigrams.bin...")
local unigrams = Load_unigrams("../data/unigrams.bin")
if not unigrams or Count_table_entries(unigrams) == 0 then
    err_print("Error: Failed to load unigrams from ../data/unigrams.bin")
    err_print("run $ luajit build-ngram.lua first to generate the unigrams file")
    os.exit(1)
end

-- Populate the word trie
dbg_print("Building word trie...")
local word_trie = Trie.new()
local word_count = 0
local total_words = Count_table_entries(unigrams)

for word, freq in pairs(unigrams) do
    word_trie:insert(word, freq)
    word_count = word_count + 1

    if word_count % 10000 == 0 then
        dbg_print(string.format("Added %d/%d words to trie", word_count, total_words))
        collectgarbage("step")
    end
end

Save_trie(word_trie, "../data/word_trie.bin", unigrams)
