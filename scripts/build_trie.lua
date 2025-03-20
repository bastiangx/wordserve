-- Build a trie from the unigrams.bin file
local ffi = require("ffi")

-- Trie implementation
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

-- Count table entries
function count_table_entries(t)
    local count = 0
    for _ in pairs(t) do
        count = count + 1
    end
    return count
end

-- Load unigrams from binary file
function load_unigrams(filename)
    local unigrams = {}
    local file = io.open(filename, "rb")
    if not file then
        print("Error: Could not open unigram file " .. filename)
        return unigrams
    end
    
    -- Read count
    local count_bytes = file:read(4)
    if not count_bytes or #count_bytes < 4 then
        print("Error: Invalid unigram file header")
        file:close()
        return unigrams
    end
    
    local count_ptr = ffi.cast("int32_t*", ffi.new("char[4]", count_bytes))
    local count = count_ptr[0]
    
    print("Reading " .. count .. " unigrams from " .. filename)
    
    -- Read all unigrams
    local entries_read = 0
    for i=1, count do
        -- Read word length
        local len_bytes = file:read(2)
        if not len_bytes or #len_bytes < 2 then
            print("Error: Unexpected EOF in unigram file at entry " .. i)
            break
        end
        
        local len_ptr = ffi.cast("uint16_t*", ffi.new("char[2]", len_bytes))
        local word_len = len_ptr[0]
        
        -- Read word
        local word = file:read(word_len)
        if not word or #word < word_len then
            print("Error: Unexpected EOF in unigram file at entry " .. i)
            break
        end
        
        -- Read frequency
        local freq_bytes = file:read(4)
        if not freq_bytes or #freq_bytes < 4 then
            print("Error: Unexpected EOF in unigram file at entry " .. i)
            break
        end
        
        local freq_ptr = ffi.cast("uint32_t*", ffi.new("char[4]", freq_bytes))
        local freq = freq_ptr[0]
        
        unigrams[word] = freq
        entries_read = entries_read + 1
        
        if entries_read % 100000 == 0 then
            print(string.format("Read %d/%d entries", entries_read, count))
        end
    end
    
    file:close()
    print("Loaded " .. entries_read .. " unigrams")
    return unigrams
end

-- Serialize the trie structure
function save_trie(trie, filename, unigrams)
    local file = io.open(filename, "wb")
    if not file then
        print("Error: Could not create file " .. filename)
        return
    end

    -- Write the number of unigrams
    local count = count_table_entries(unigrams)
    file:write(ffi.string(ffi.new("int32_t[1]", count), 4))
    print("Serializing trie with " .. count .. " words...")

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
                print(string.format("Serialized %d/%d words", nodes_processed, count))
            end
        end

        -- Sort children for deterministic output
        local keys = {}
        for k in pairs(node.children) do
            table.insert(keys, k)
        end
        table.sort(keys)

        -- Process each child
        for _, char in ipairs(keys) do
            serialize_node(node.children[char], prefix .. char)
        end
    end

    -- Start serialization from root with empty prefix
    serialize_node(trie, "")

    file:close()
    print("Trie serialized to " .. filename)
end

-- Main code execution
print("Building trie from unigrams.bin...")

-- Load unigrams
local unigrams = load_unigrams("unigrams.bin")

-- Create and populate word trie
print("Building word trie...")
local word_trie = Trie.new()
local word_count = 0
local total_words = count_table_entries(unigrams)

for word, freq in pairs(unigrams) do
    word_trie:insert(word, freq)
    word_count = word_count + 1

    if word_count % 10000 == 0 then
        print(string.format("Added %d/%d words to trie", word_count, total_words))
        collectgarbage("step")
    end
end

save_trie(word_trie, "word_trie.bin", unigrams)
print("All processing complete!")