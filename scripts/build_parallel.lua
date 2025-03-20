-- Multi-threaded corpus processing using multiple LuaJIT processes
local ffi = require("ffi")

-- Determine number of cores to use (leave one for the OS)
local function get_cpu_count()
  local is_unix = package.config:sub(1, 1) == '/'
  local cmd

  if is_unix then
    cmd = "nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4"
  else
    cmd = "echo %NUMBER_OF_PROCESSORS% 2>nul || echo 4"
  end

  local handle = io.popen(cmd)
  if not handle then
    return 4     -- Return a default value if handle creation fails
  end
  local result = handle:read("*a")
  handle:close()

  return math.max(1, tonumber(result:match("%d+")) - 1)
end

-- Split a directory of files into chunks for parallel processing
local function split_directory(directory, chunks)
  local files = {}
  local chunk_lists = {}

  -- Initialize empty chunk lists
  for i = 1, chunks do
    chunk_lists[i] = {}
  end

  -- Get all txt files in the directory
  local is_unix = package.config:sub(1, 1) == '/'
  local cmd = is_unix and ("find " .. directory .. " -name '*.txt' | sort") or
      ("dir /b /s " .. directory .. "\\*.txt")

  local handle = io.popen(cmd)
  if not handle then
    print("Error: Could not list files in directory " .. directory)
    return chunk_lists
  end

  -- Read all file paths
  for file in handle:lines() do
    table.insert(files, file)
  end
  handle:close()

  -- Distribute files across chunks
  for i, file in ipairs(files) do
    local chunk_index = ((i - 1) % chunks) + 1
    table.insert(chunk_lists[chunk_index], file)
  end

  print("Distributed " .. #files .. " files into " .. chunks .. " chunks")
  for i, chunk in ipairs(chunk_lists) do
    print("Chunk " .. i .. " has " .. #chunk .. " files")
  end

  return chunk_lists
end

-- Write a chunk of files to a temporary file
local function write_chunk_file(chunk, filename)
  local file = io.open(filename, "w")
  if not file then
    print("Error: Could not create chunk file " .. filename)
    return false
  end

  for _, filepath in ipairs(chunk) do
    file:write(filepath .. "\n")
  end
  file:close()
  return true
end

-- Merge multiple unigram binary files
local function merge_unigram_files(files, output_file)
  print("Merging " .. #files .. " unigram files...")

  -- Load and merge all unigrams
  local merged = {}

  for _, file_path in ipairs(files) do
    print("Loading " .. file_path)
    local file = io.open(file_path, "rb")
    if not file then
      print("Error: Could not open unigram file " .. file_path)
      goto continue
    end

    -- Read count
    local count_bytes = file:read(4)
    if not count_bytes or #count_bytes < 4 then
      print("Error: Invalid unigram file header in " .. file_path)
      file:close()
      goto continue
    end

    local count_ptr = ffi.cast("int32_t*", ffi.new("char[4]", count_bytes))
    local count = count_ptr[0]

    print("Reading " .. count .. " unigrams from " .. file_path)

    -- Read all unigrams
    for i = 1, count do
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

      -- Merge with existing entry if any
      merged[word] = (merged[word] or 0) + freq
    end

    file:close()
    ::continue::
  end

  -- Write merged unigrams
  print("Writing merged unigrams to " .. output_file)
  local out_file = io.open(output_file, "wb")
  if not out_file then
    print("Error: Could not create output file " .. output_file)
    return false
  end

  -- Convert to sorted array for deterministic output
  local sorted_items = {}
  local total_entries = 0

  for word, freq in pairs(merged) do
    table.insert(sorted_items, { word = word, freq = freq })
    total_entries = total_entries + 1

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
  out_file:write(ffi.string(count_ptr, 4))

  print("Writing " .. count .. " entries to " .. output_file)

  -- Write each word and its frequency in frequency order
  local entries_written = 0
  for _, item in ipairs(sorted_items) do
    local word = item.word
    local freq = item.freq

    local len = #word
    local len_ptr = ffi.new("uint16_t[1]", len)
    out_file:write(ffi.string(len_ptr, 2))     -- Write the word length

    -- Write the actual word string
    out_file:write(word)

    -- Write the frequency value
    local freq_ptr = ffi.new("uint32_t[1]", freq)
    out_file:write(ffi.string(freq_ptr, 4))

    entries_written = entries_written + 1
    if entries_written % 100000 == 0 then
      print(string.format("Wrote %d/%d entries", entries_written, count))
    end
  end

  print("Top 10 highest frequency items:")
  for i = 1, math.min(10, #sorted_items) do
    print(string.format("%s: %d", sorted_items[i].word, sorted_items[i].freq))
  end

  out_file:close()
  print("Finished writing " .. output_file)
  return true
end

-- Create a worker script to process one chunk
local function create_worker_script()
  local script = [[
local chunk_file = ... -- Get the chunk file from command line
if not chunk_file then
    print("Error: No chunk file specified")
    os.exit(1)
end

local output_file = chunk_file .. ".unigrams"

-- Load required libraries
local ffi = require("ffi")

-- store unigrams
local unigrams = {}

-- Extract words from text
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

-- Process a single file
function process_file(filename)
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

-- Count table entries
function count_table_entries(t)
    local count = 0
    for _ in pairs(t) do
        count = count + 1
    end
    return count
end

-- Save unigrams to binary file
function save_binary(ngrams, filename)
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
    end

    file:close()
end

-- Process all files in the chunk
local chunk = {}
local file = io.open(chunk_file, "r")
if not file then
    print("Error: Could not open chunk file " .. chunk_file)
    os.exit(1)
end

for line in file:lines() do
    table.insert(chunk, line)
end
file:close()

print("Worker processing " .. #chunk .. " files from " .. chunk_file)

local processed_count = 0
local words_processed = 0

for _, filename in ipairs(chunk) do
    processed_count = processed_count + 1

    if processed_count % 10 == 0 then
        print(string.format("Processing file %d of %d", processed_count, #chunk))
    end

    local words = process_file(filename)
    for _, word in ipairs(words) do
        -- Accumulate word frequencies
        unigrams[word] = (unigrams[word] or 0) + 1
        words_processed = words_processed + 1
    end

    -- Occasionally run garbage collection
    if processed_count % 20 == 0 then
        collectgarbage("step")
    end
end

print("Finished processing " .. processed_count .. " files")
print("Total words processed: " .. words_processed)
print("Total unique words: " .. count_table_entries(unigrams))

-- Save unigrams to binary file
save_binary(unigrams, output_file)
print("Saved unigrams to " .. output_file)
]]

  local worker_file = "worker.lua"
  local file = io.open(worker_file, "w")
  if not file then
    print("Error: Could not create worker script")
    return false
  end
  file:write(script)
  file:close()
  return worker_file
end

-- Main processing function
local function process_corpus_parallel(directory, output_file)
  local cpu_count = get_cpu_count()
  print("Using " .. cpu_count .. " parallel processes")

  -- Split directory into chunks
  local chunks = split_directory(directory, cpu_count)

  -- Create worker script
  local worker_script = create_worker_script()
  if not worker_script then
    return false
  end

  -- Create chunk files and launch workers
  local chunk_files = {}
  local worker_processes = {}

  for i, chunk in ipairs(chunks) do
    local chunk_file = "chunk_" .. i .. ".txt"
    if write_chunk_file(chunk, chunk_file) then
      table.insert(chunk_files, chunk_file)

      -- Launch worker process
      local cmd = "luajit " .. worker_script .. " " .. chunk_file
      print("Launching worker: " .. cmd)

      local is_unix = package.config:sub(1, 1) == '/'
      if is_unix then
        cmd = cmd .. " &"         -- Run in background on Unix
        os.execute(cmd)
      else
        cmd = "start /B " .. cmd         -- Run in background on Windows
        os.execute(cmd)
      end

      table.insert(worker_processes, cmd)
    end
  end

  -- Wait for all workers to complete
  print("Waiting for worker processes to complete...")
  local unigram_files = {}

  for _, chunk_file in ipairs(chunk_files) do
    local unigram_file = chunk_file .. ".unigrams"
    table.insert(unigram_files, unigram_file)
  end

  local timeout = 60 * 60   -- 1 hour timeout
  local start_time = os.time()

  local all_complete = false
  while not all_complete and os.time() - start_time < timeout do
    all_complete = true
    for _, file in ipairs(unigram_files) do
      local f = io.open(file, "rb")
      if not f then
        all_complete = false
        break
      end
      f:close()
    end

    if not all_complete then
      os.execute("sleep 5")       -- Wait 5 seconds before checking again
    end
  end

  if not all_complete then
    print("Error: Not all worker processes completed within the timeout")
    return false
  end

  -- Merge results
  local success = merge_unigram_files(unigram_files, output_file)

  -- Clean up temporary files
  for _, file in ipairs(chunk_files) do
    os.remove(file)
    os.remove(file .. ".unigrams")
  end
  os.remove(worker_script)

  return success
end

-- Main code execution
print("Starting parallel corpus processing...")
local success = process_corpus_parallel("corpus_stopwords", "unigrams.bin")

if success then
  print("Successfully processed corpus in parallel")
  print("Now you can build the trie from unigrams.bin")
  print("Run: luajit build_trie.lua")
else
  print("Error: Parallel processing failed")
end
