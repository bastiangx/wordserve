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

	-- Read all words into an array preserving original order
	local all_words = {}
	local line_count = 0

	for line in file:lines() do
		line_count = line_count + 1
		local word = line:lower():match("%S+")
		if word then
			table.insert(all_words, word)
		end
	end

	-- DO NOT sort words - preserve the original frequency order from the file
	-- The 20k.txt file already has words ordered by frequency (most common first)

	-- Assign frequencies using position in the list (higher rank = lower frequency)
	for rank, word in ipairs(all_words) do
		-- Zipf's law: frequency is roughly proportional to 1/rank
		local freq = math.floor(1000000 / rank)     -- Simple inverse relationship
		if freq < 1 then freq = 1 end

		unigrams[word] = freq

		-- For debugging
		if rank % 1000 == 0 or rank < 20 then
			print(string.format("Assigning word '%s' (rank %d) frequency: %d", word, rank, freq))
		end
	end

	-- Generate bigrams with derived frequencies
	for i = 1, #all_words - 1 do
		local w1 = all_words[i]
		local w2 = all_words[i + 1]
		local freq = math.floor(math.sqrt(unigrams[w1] * unigrams[w2] * 0.1))
		if freq < 1 then freq = 1 end

		local bigram = w1 .. " " .. w2
		bigrams[bigram] = freq
	end

	file:close()
	print("Processed " .. #all_words .. " words with calculated frequencies")
end

function Save_binary(ngrams, filename)
	local file = io.open(filename, "wb")
	if not file then
		print("Error: Could not create file " .. filename)
		return
	end

	-- Convert to sorted array for deterministic output
	local sorted_items = {}
	for ngram, freq in pairs(ngrams) do
		table.insert(sorted_items, { word = ngram, freq = freq })
	end

	-- Sort by frequency (highest first)
	table.sort(sorted_items, function(a, b) return a.freq > b.freq end)

	-- Write header with count
	local count = #sorted_items
	local count_ptr = ffi.new("int32_t[1]", count)
	file:write(ffi.string(count_ptr, 4))

	print("Writing " .. count .. " entries to " .. filename)

	-- Write each n-gram and its frequency in frequency order
	for _, item in ipairs(sorted_items) do
		local ngram = item.word
		local freq = item.freq

		local len = #ngram
		local len_ptr = ffi.new("uint16_t[1]", len)
		file:write(ffi.string(len_ptr, 2))     -- Write the word length

		-- Write the actual word string
		file:write(ngram)

		-- Write the frequency value
		local freq_ptr = ffi.new("uint32_t[1]", freq)
		file:write(ffi.string(freq_ptr, 4))

		-- Debug only high frequency words
		if freq > 10000 then
			print(string.format("Writing high-freq word: '%s' with frequency: %d", ngram, freq))
		end
	end

	print("Top 10 highest frequency items:")
	for i = 1, math.min(10, #sorted_items) do
		print(string.format("%s: %d", sorted_items[i].word, sorted_items[i].freq))
	end

	file:close()
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
