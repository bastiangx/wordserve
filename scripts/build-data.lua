-- Builds trie binaries from a word freq file

-- Input: data/words.txt (format: "word <tab> frequency") - relative to script location
-- Output: Multiple binary files: data/dict_0001.bin, data/dict_0002.bin, etc.
-- Each chunk contains a specified number of words (default: 10,000)

-- REQUIRES LuaJIT with FFI

local ffi = require("ffi")

local function get_script_dir()
	local info = debug.getinfo(1, "S")
	local script_path = info.source:match("@(.*)")
	if script_path then
		local dir = script_path:match("(.*/)") or "./"
		return dir
	end
	return "./"
end

local script_dir = get_script_dir()
local data_dir = script_dir .. "../data/"
local words_file = data_dir .. "words.txt"

local verbose = false
local show_help = false
local chunk_size = 10000
local max_chunks = 0

for i = 1, #arg do
	if arg[i] == "-v" or arg[i] == "--verbose" then
		verbose = true
	elseif arg[i] == "-s" then
		verbose = false
	elseif arg[i] == "-h" or arg[i] == "--help" then
		show_help = true
		break
	elseif arg[i] == "--chunk-size" and arg[i + 1] then
		local num = tonumber(arg[i + 1])
		if not num or num <= 0 then
			print("Error: Invalid chunk size")
			os.exit(1)
		end
		chunk_size = num
	elseif arg[i] == "--max-chunks" and arg[i + 1] then
		local num = tonumber(arg[i + 1])
		if num == nil then
			print("Error: Invalid max chunks")
			os.exit(1)
		end
		max_chunks = num
		if max_chunks < 0 then
			print("Error: Invalid max chunks")
			os.exit(1)
		end
	end
end

if show_help then
	print("")
	print("Usage: luajit build-data.lua [ OPTIONS ]")
	print("")
	print("Builds trie binaries from a word freq file")
	print("")
	print("Input: data/words.txt (format: 'word <tab> frequency') - relative to script location")
	print("Output: Multiple binary files: data/dict_XXXX.bin - relative to script location")
	print("")
	print("Options:")
	print("  -h, --help         Show this help message")
	print("  -v, --verbose      Verbose output")
	print("  -s                 Silent mode (default)")
	print("  --chunk-size N     Words per chunk (default: 10000)")
	print("  --max-chunks N     Maximum number of chunks (default: nolimit)")
	print("")
	print("")
	print("Find out more: https://github.com/bastiangx/worserve")
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

-- Counts  entries by iterating through the table
function Count_table_entries(t)
	local count = 0
	for _ in pairs(t) do
		count = count + 1
	end
	return count
end

function Save_trie_chunk(trie, filename, word_data)
	local file = io.open(filename, "wb")
	if not file then
		err_print("Error: Could not create file " .. filename)
		return false
	end

	local count = Count_table_entries(word_data)
	file:write(ffi.string(ffi.new("int32_t[1]", count), 4))
	dbg_print("Serializing trie chunk with " .. count .. " words to " .. filename)

	-- Serialize trie using DFS
	local nodes_processed = 0
	local function serialize_node(node, prefix)
		if node.is_word then
			-- Write this word and its rank (2 bytes instead of 4 bytes for freq)
			local word_len = #prefix
			file:write(ffi.string(ffi.new("uint16_t[1]", word_len), 2))
			file:write(prefix)
			file:write(ffi.string(ffi.new("uint16_t[1]", node.frequency), 2))

			nodes_processed = nodes_processed + 1
			if verbose and nodes_processed % 5000 == 0 then
				dbg_print(string.format("  Serialized %d/%d words", nodes_processed, count))
			end
		end

		-- Sort children for deterministic output
		local keys = {}
		for k in pairs(node.children) do
			table.insert(keys, k)
		end
		table.sort(keys)

		-- Process each child node
		for _, char in ipairs(keys) do
			serialize_node(node.children[char], prefix .. char)
		end
	end

	-- Start with empty prefix for root node
	serialize_node(trie, "")

	file:close()
	dbg_print("Trie chunk successfully serialized: " .. filename)
	return true
end

-- Format: "word <tab> frequency"
function Load_frequencies(file)
	local freq_data = {}
	local word_list = {}

	for line in io.lines(file) do
		local word, freq = line:match("(.-)\t(%d+)")
		if word and freq then
			local frequency = tonumber(freq)
			freq_data[word] = frequency
			table.insert(word_list, { word = word, freq = frequency })
		end
	end

	return freq_data, word_list
end

-- Main processing
dbg_print("Building chunked tries from frequencies file...")
local words_with_frequencies, word_list = Load_frequencies(words_file)

if not words_with_frequencies or Count_table_entries(words_with_frequencies) == 0 then
	err_print("Error: Failed to load frequencies from " .. words_file)
	os.exit(1)
end

local total_words = #word_list
dbg_print(string.format("Loaded %d words total", total_words))

-- Since words.txt is already sorted by frequency, rank = position in list
dbg_print("Converting frequencies to ranks for memory optimization...")
for i, word_entry in ipairs(word_list) do
	-- Rank starts at 1 (most frequent word), uint16 supports up to 65K words
	local rank = math.min(i, 65535)
	word_entry.rank = rank
	words_with_frequencies[word_entry.word] = rank
end
dbg_print("Frequency to rank conversion complete")

local total_chunks = math.ceil(total_words / chunk_size)
if max_chunks > 0 and total_chunks > max_chunks then
	total_chunks = max_chunks
	dbg_print(string.format("Limiting to %d chunks as requested (for testing)", max_chunks))
else
	dbg_print(string.format("Building all chunks needed for %d words", total_words))
end

dbg_print(string.format("Creating %d chunks of ~%d words each", total_chunks, chunk_size))

-- Process chunks
for chunk_idx = 1, total_chunks do
	local start_idx = (chunk_idx - 1) * chunk_size + 1
	local end_idx = math.min(chunk_idx * chunk_size, total_words)

	if start_idx > total_words then
		break
	end

	dbg_print(string.format("Processing chunk %d/%d (words %d-%d)", chunk_idx, total_chunks, start_idx, end_idx))

	-- Create trie for this chunk
	local chunk_trie = Trie.new()
	local chunk_words = {}

	for i = start_idx, end_idx do
		local word_entry = word_list[i]
		if word_entry then
			-- Use rank instead of frequency for memory optimization
			chunk_trie:insert(word_entry.word, word_entry.rank)
			chunk_words[word_entry.word] = word_entry.rank
		end
	end

	local chunk_filename = string.format("%sdict_%04d.bin", data_dir, chunk_idx)
	if not Save_trie_chunk(chunk_trie, chunk_filename, chunk_words) then
		err_print("Failed to save chunk " .. chunk_idx)
		os.exit(1)
	end
	collectgarbage("collect")
end

dbg_print(string.format("Successfully created %d bin files!", total_chunks))
