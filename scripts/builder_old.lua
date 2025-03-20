-- This script processes text files from a corpus and builds a unigram model.
-- It then saves the unigrams to binary files for later use.

-- Optimized for LUAJIT processing
local ffi = require("ffi")

-- Try to load LuaFileSystem, but don't fail if not available
local lfs_available = false
local lfs = nil
local status, err = pcall(function() lfs = require("lfs") end)
if status then
	lfs_available = true
	print("Using LuaFileSystem for directory traversal")
else
	print("LuaFileSystem not available: " .. tostring(err))
	print("Using fallback directory traversal method with shell commands")
	-- Check if we're on Unix-like system
	if package.config:sub(1, 1) == '/' then
		print("Detected Unix-like system")
	else
		print("WARNING: Fallback method works best on Unix-like systems")
	end
end

-- store unigrams
local unigrams = {}

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

-- Helper function to clean and extract valid words
function extract_words(text)
	local words = {}
	-- Remove special markers like @@31618941 and HTML tags
	text = text:gsub("@@%d+", " "):gsub("<%/?[^>]+>", " ")

	-- Split on whitespace and extract only valid words (alphabetic characters)
	for word in text:gmatch("%S+") do
		-- Convert to lowercase and remove non-alphabetic characters
		local clean_word = word:lower():gsub("[^a-z]", "")
		if #clean_word > 0 then -- Only add non-empty words
			table.insert(words, clean_word)
		end
	end

	return words
end

-- Alternative directory listing function using shell commands
function list_files_shell(directory)
	local files = {}
	local count = 0

	local is_unix = package.config:sub(1, 1) == '/'
	local cmd = is_unix and ("ls -1 " .. directory) or ("dir /b " .. directory)

	local p = io.popen(cmd)
	if not p then return files, 0 end

	for file in p:lines() do
		if file ~= "." and file ~= ".." then
			local filepath = directory .. "/" .. file
			local f = io.open(filepath, "r")
			if f then
				f:close()
				if filepath:match("%.txt$") then
					table.insert(files, file)
					count = count + 1
				end
			end
		end
	end
	p:close()

	return files, count
end

function Process_corpus_directory(directory)
	local file_count = 0
	local processed_count = 0
	local files = {}
	local words_processed = 0

	print("Processing corpus files in " .. directory)

	-- Get list of files using the appropriate method
	if lfs_available and lfs then
		-- Using LuaFileSystem
		for file in lfs.dir(directory) do
			if file ~= "." and file ~= ".." then
				local f_path = directory .. "/" .. file
				local attr = lfs.attributes(f_path)
				if attr and attr.mode == "file" and f_path:match("%.txt$") then
					table.insert(files, file)
					file_count = file_count + 1
				end
			end
		end
	else
		-- Using shell command fallback
		files, file_count = list_files_shell(directory)
	end

	print("Found " .. file_count .. " text files to process")

	-- Process each text file
	for _, file in ipairs(files) do
		local f_path = directory .. "/" .. file
		processed_count = processed_count + 1

		if processed_count % 10 == 0 or processed_count == 1 then
			print(string.format("Processing file %d of %d: %s",
				processed_count, file_count, file))
		end

		local words = Process_file(f_path)
		for _, word in ipairs(words) do
			-- Accumulate word frequencies
			unigrams[word] = (unigrams[word] or 0) + 1
			words_processed = words_processed + 1
		end

		-- Occasionally run garbage collection on large datasets
		if processed_count % 20 == 0 then
			collectgarbage("step")
		end
	end

	print("Finished processing " .. processed_count .. " files")
	print("Total words processed: " .. words_processed)
	print("Total unique words: " .. count_table_entries(unigrams))

	return unigrams
end

function Process_file(filename)
	local file = io.open(filename, "r")
	if not file then
		print("Error: Could not open file " .. filename)
		return {}
	end

	local words = {}
	local content = file:read("*all")
	file:close()

	-- Extract clean words from the content
	return extract_words(content)
end

function count_table_entries(t)
	local count = 0
	for _ in pairs(t) do
		count = count + 1
	end
	return count
end

function Save_binary(ngrams, filename)
	local file = io.open(filename, "wb")
	if not file then
		print("Error: Could not create file " .. filename)
		return
	end

	-- Convert to sorted array for deterministic output
	local sorted_items = {}
	local total_entries = 0

	for ngram, freq in pairs(ngrams) do
		table.insert(sorted_items, { word = ngram, freq = freq })
		total_entries = total_entries + 1

		-- Report progress for large datasets
		if total_entries % 100000 == 0 then
			print(string.format("Prepared %d entries for sorting", total_entries))
		end
	end

	-- Sort by frequency (highest first)
	print("Sorting " .. #sorted_items .. " entries by frequency...")
	table.sort(sorted_items, function(a, b) return a.freq > b.freq end)

	-- Write header with count
	local count = #sorted_items
	local count_ptr = ffi.new("int32_t[1]", count)
	file:write(ffi.string(count_ptr, 4))

	print("Writing " .. count .. " entries to " .. filename)

	-- Write each n-gram and its frequency in frequency order
	local entries_written = 0
	for _, item in ipairs(sorted_items) do
		local ngram = item.word
		local freq = item.freq

		local len = #ngram
		local len_ptr = ffi.new("uint16_t[1]", len)
		file:write(ffi.string(len_ptr, 2)) -- Write the word length

		-- Write the actual word string
		file:write(ngram)

		-- Write the frequency value
		local freq_ptr = ffi.new("uint32_t[1]", freq)
		file:write(ffi.string(freq_ptr, 4))

		-- Debug only high frequency words
		if freq > 10000 then
			print(string.format("Writing high-freq word: '%s' with frequency: %d", ngram, freq))
		end

		entries_written = entries_written + 1
		if entries_written % 100000 == 0 then
			print(string.format("Wrote %d/%d entries", entries_written, count))
		end
	end

	print("Top 10 highest frequency items:")
	for i = 1, math.min(10, #sorted_items) do
		print(string.format("%s: %d", sorted_items[i].word, sorted_items[i].freq))
	end

	file:close()
	print("Finished writing " .. filename)
end

-- Serialize the trie structure
function Save_trie(trie, filename)
	local file = io.open(filename, "wb")
	if not file then
		print("Error: Could not create file " .. filename)
		return
	end

	-- Write the number of unigrams
	local count = 0
	for _ in pairs(unigrams) do
		count = count + 1
	end
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

-- Main code execution starts here
print("Starting corpus processing...")

-- Process all text files in the corpus directory
Process_corpus_directory("corpus_stopwords")

-- sort and output unigrams
print("Saving unigrams...")
Save_binary(unigrams, "unigrams.bin")

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

Save_trie(word_trie, "word_trie.bin")
print("All processing complete!")
