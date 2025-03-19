-- This script processes a text file and builds n-gram models from it.
-- It then saves the n-grams to binary files for later use.

-- Optimized for LUAJIT processing

local ffi = require("ffi")

-- store different n-gram levels
local unigrams = {}
local bigrams = {}
local trigrams = {}

local Trie = {}
Trie.__index = Trie

function Trie.new()
	return setmetatable({
		children = {},
		is_word = false,
		frequency = 0,
		-- {word = frequency}
		next_words = {},
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
	node.frequency = freq
end

function Trie:add_bigram(word1, word2, freq)
	local node = self
	-- Insert first
	for i = 1, #word1 do
		local char = word1:sub(i, i)
		node.children[char] = node.children[char] or Trie.new()
		node = node.children[char]
	end
	-- add association with second word
	node.next_words[word2] = (node.next_words[word2] or 0) + freq
end

function Process_corpus(filename)
	local file = io.open(filename, "r")
	if not file then
		print("Error: Could not open file " .. filename)
		return
	end
	local line_count = 0

	for line in file:lines() do
		line_count = line_count + 1
		-- Tokenize line into words
		local words = {}
		for word in line:gmatch("%S+") do
			table.insert(words, word:lower())
		end

		-- unigrams
		for i = 1, #words do
			local word = words[i]
			unigrams[word] = (unigrams[word] or 0) + 1
		end

		for i = 1, #words - 1 do
			local bigram = words[i] .. " " .. words[i + 1]
			bigrams[bigram] = (bigrams[bigram] or 0) + 1
		end

		-- trigrams
		for i = 1, #words - 2 do
			local trigram = words[i] .. " " .. words[i + 1] .. " " .. words[i + 2]
			trigrams[trigram] = (trigrams[trigram] or 0) + 1
		end

		if line_count % 10000 == 0 then
			print("Processed " .. line_count .. " lines")
		end
	end

	if file then
		file:close()
	end
end

function Save_binary(ngrams, filename)
	local file = io.open(filename, "wb")
	if not file then
		print("Error: Could not create file " .. filename)
		return
	end

	-- Write header with count
	local count = 0
	for _ in pairs(ngrams) do
		count = count + 1
	end
	local count_ptr = ffi.new("int32_t[1]", count)
	file:write(ffi.string(count_ptr, 4))

	print("Writing " .. count .. " entries to " .. filename)

	-- Write each n-gram and its frequency
	for ngram, freq in pairs(ngrams) do
		local len = #ngram
		local len_ptr = ffi.new("uint16_t[1]", len)
		file:write(ffi.string(len_ptr, 2)) -- Write the word length

		-- Write the actual word string
		file:write(ngram)

		-- Write the frequency value
		local freq_ptr = ffi.new("uint32_t[1]", freq)
		file:write(ffi.string(freq_ptr, 4))
	end

	if file then
		file:close()
	end

	print("Finished writing " .. filename)
end

-- Fix the Save_trie function to properly serialize the trie
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

	-- Serialize the trie structure using DFS
	local function serialize_node(node, prefix)
		if node.is_word then
			-- Write this word and its frequency
			local word_len = #prefix
			file:write(ffi.string(ffi.new("uint16_t[1]", word_len), 2))
			file:write(prefix)
			file:write(ffi.string(ffi.new("uint32_t[1]", node.frequency), 4))
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
Process_corpus("20k.txt")

-- sort and output top n-grams
print("Saving n-grams...")
Save_binary(unigrams, "unigrams.bin")
Save_binary(bigrams, "bigrams.bin")
Save_binary(trigrams, "trigrams.bin")

local word_trie = Trie.new()
for word, freq in pairs(unigrams) do
	word_trie:insert(word, freq)
end

Save_trie(word_trie, "word_trie.bin")
print("All processing complete!")

