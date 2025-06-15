-- Multi-threaded corpus processing by makings unigrams from a directory of "cleaned" text files
-- Requires LuaJIT with FFI support

-- Important: have cleaned .txt in a directory called "cleaned-texts"
-- `filter.py` file should have been run on the corpus first before this script.

local ffi = require("ffi")

-- Parse command line arguments
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

-- Show help and exit
if show_help then
  print("")
  print("Usage: luajit build-ngram.lua [ OPTIONS ]")
  print("")
  print("Multi-threaded corpus processing by making unigrams from a directory of cleaned text files.")
  print("Requires cleaned .txt files in a directory called 'cleaned-texts'.")
  print("Run 'filter.py' on the corpus first before this script.")
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

-- Multi-threading :: get n of cpu cores available
local function get_cpu_count()
  local is_unix = package.config:sub(1, 1) == '/'
  local cmd

  if is_unix then
    cmd = "nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4"
  else
    cmd = "echo %NUMBER_OF_PROCESSORS% 2>nul || echo 4"
  end

  local handle = io.popen(cmd)
  -- default : 4 cores
  if not handle then
    return 4
  end
  local result = handle:read("*a")
  handle:close()
  return math.max(1, tonumber(result:match("%d+")) - 1)
end

-- Mult-threading :: split directory into n chunks
-- chunks can speed up unigram extraction by processing files in parallel
local has_critical_error = false

-- handle empty directories or no files
local function split_directory(directory, chunks)
  local files = {}
  local chunk_lists = {}

  for i = 1, chunks do
    chunk_lists[i] = {}
  end

  -- Gets all txt files
  local is_unix = package.config:sub(1, 1) == '/'
  local cmd = is_unix and ("find " .. directory .. " -name '*.txt' | sort") or
      ("dir /b /s " .. directory .. "\\*.txt")

  local handle = io.popen(cmd)
  if not handle then
    err_print("Critical Error: Could not access directory " .. directory)
    has_critical_error = true
    return chunk_lists
  end

  -- Reads all files
  local file_count = 0
  for file in handle:lines() do
    table.insert(files, file)
    file_count = file_count + 1
  end
  handle:close()

  if file_count == 0 then
    err_print("Critical Error: No .txt files found in directory " .. directory)
    has_critical_error = true
    return chunk_lists
  end

  -- Distribute files across chunks
  for i, file in ipairs(files) do
    local chunk_index = ((i - 1) % chunks) + 1
    table.insert(chunk_lists[chunk_index], file)
  end

  dbg_print("Mashed " .. #files .. " files into " .. chunks .. " chunks")
  for i, chunk in ipairs(chunk_lists) do
    dbg_print("Chunk " .. i .. " has " .. #chunk .. " files")
  end

  return chunk_lists
end

-- Write a chunk of files to a temp file
local function write_chunk_file(chunk, filename)
  local file = io.open(filename, "w")
  if not file then
    err_print("Error: Could not create chunk file " .. filename)
    return false
  end

  for _, filepath in ipairs(chunk) do
    file:write(filepath .. "\n")
  end
  file:close()
  return true
end

-- Merges by parsing multiple unigram files
local function merge_unigram_files(files, output_file)
  dbg_print("Merging " .. #files .. " unigram files...")

  local merged = {}

  for _, file_path in ipairs(files) do
    dbg_print("Loading " .. file_path)
    local file = io.open(file_path, "rb")
    if not file then
      err_print("Error: Could not open unigram file " .. file_path)
      goto continue
    end

    -- Read count
    local count_bytes = file:read(4)
    if not count_bytes or #count_bytes < 4 then
      err_print("Error: Invalid unigram file header in " .. file_path)
      file:close()
      goto continue
    end

    local count_ptr = ffi.cast("int32_t*", ffi.new("char[4]", count_bytes))
    local count = count_ptr[0]

    dbg_print("Reading " .. count .. " unigrams from " .. file_path)

    -- Read all unigrams
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

      merged[word] = (merged[word] or 0) + freq
    end

    file:close()
    ::continue::
  end

  -- Write merged unigrams
  dbg_print("Writing unigrams to " .. output_file)
  local out_file = io.open(output_file, "wb")
  if not out_file then
    err_print("Error: Could not create output file " .. output_file)
    return false
  end

  -- Convert to sorted array for deterministic output
  local sorted_items = {}
  local total_entries = 0

  for word, freq in pairs(merged) do
    table.insert(sorted_items, { word = word, freq = freq })
    total_entries = total_entries + 1

    if total_entries % 100000 == 0 then
      dbg_print(string.format("Prepared %d entries for sorting", total_entries))
    end
  end

  -- Sort by frequency (highest first)
  dbg_print("Sorting " .. #sorted_items .. " entries by frequency...")
  table.sort(sorted_items, function(a, b) return a.freq > b.freq end)

  -- Write header with count
  local count = #sorted_items
  local count_ptr = ffi.new("int32_t[1]", count)
  out_file:write(ffi.string(count_ptr, 4))

  dbg_print("Writing " .. count .. " entries to " .. output_file)

  -- Write each word and its frequency in frequency order
  local entries_written = 0
  for _, item in ipairs(sorted_items) do
    local word = item.word
    local freq = item.freq

    local len = #word
    local len_ptr = ffi.new("uint16_t[1]", len)
    out_file:write(ffi.string(len_ptr, 2)) -- word length

    -- Write the actual word string
    out_file:write(word)

    -- Write the frequency value
    local freq_ptr = ffi.new("uint32_t[1]", freq)
    out_file:write(ffi.string(freq_ptr, 4))

    entries_written = entries_written + 1
    if entries_written % 100000 == 0 then
      dbg_print(string.format("Wrote %d/%d entries", entries_written, count))
    end
  end

  -- DBG: confirming top 10 highest frequency items
  if verbose then
    dbg_print("Top 10 highest frequency items:")
    for i = 1, math.min(10, #sorted_items) do
      dbg_print(string.format("%s: %d", sorted_items[i].word, sorted_items[i].freq))
    end
  end

  out_file:close()
  dbg_print("Finished writing " .. output_file)
  return true
end

-- worker script processes chunk of files
local function create_worker_script()
  local verbose_flag = verbose and " --verbose" or ""
  local script = [[
local chunk_file = ... -- Get the chunk file from cl arguments
if not chunk_file then
    print("Error: No chunk file specified")
    os.exit(1)
end

local output_file = chunk_file .. ".unigrams"

-- Check for verbose flag passed from parent
local verbose = false
for i = 1, #arg do
    if arg[i] == "--verbose" then
        verbose = true
        break
    end
end

local function dbg_print(...)
    if verbose then
        print(...)
    end
end

local ffi = require("ffi")

local unigrams = {}

function extract_words(text)
    local words = {}
    text = text:gsub("@@%d+", " "):gsub("<%/?[^>]+>", " ")

    for word in text:gmatch("%S+") do
        -- Convert to lowercase and remove non-alphabetic characters
        local clean_word = word:lower():gsub("[^a-z]", "")
        if #clean_word > 0 then -- Only add non-empty words
            table.insert(words, clean_word)
        end
    end
    return words
end

function process_file(filename)
    local file = io.open(filename, "r")
    if not file then
        print("Error: Could not open file " .. filename)
        return {}
    end

    local words = {}
    local content = file:read("*all")
    file:close()

    return extract_words(content)
end

function count_table_entries(t)
    local count = 0
    for _ in pairs(t) do
        count = count + 1
    end
    return count
end

function save_binary(ngrams, filename)
    local file = io.open(filename, "wb")
    if not file then
        print("Error: Could not create file " .. filename)
        return
    end

    local sorted_items = {}
    for ngram, freq in pairs(ngrams) do
        table.insert(sorted_items, { word = ngram, freq = freq })
    end

    table.sort(sorted_items, function(a, b) return a.freq > b.freq end)

    local count = #sorted_items
    local count_ptr = ffi.new("int32_t[1]", count)
    file:write(ffi.string(count_ptr, 4))

    for _, item in ipairs(sorted_items) do
        local ngram = item.word
        local freq = item.freq

        local len = #ngram
        local len_ptr = ffi.new("uint16_t[1]", len)
        file:write(ffi.string(len_ptr, 2))     -- Write the word length

        file:write(ngram)

        local freq_ptr = ffi.new("uint32_t[1]", freq)
        file:write(ffi.string(freq_ptr, 4))
    end

    file:close()
end

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

dbg_print("Worker processing " .. #chunk .. " files from " .. chunk_file)

local processed_count = 0
local words_processed = 0

for _, filename in ipairs(chunk) do
    processed_count = processed_count + 1

    if processed_count % 10 == 0 then
        dbg_print(string.format("Processing file %d of %d", processed_count, #chunk))
    end

    local words = process_file(filename)
    for _, word in ipairs(words) do
        -- Accumulate word frequencies
        unigrams[word] = (unigrams[word] or 0) + 1
        words_processed = words_processed + 1
    end

    if processed_count % 20 == 0 then
        collectgarbage("step")
    end
end

dbg_print("Finished processing " .. processed_count .. " files")
dbg_print("Total words processed: " .. words_processed)
dbg_print("Total unique words: " .. count_table_entries(unigrams))

save_binary(unigrams, output_file)
dbg_print("Saved unigrams to " .. output_file)
]]

  local worker_file = "worker.lua"
  local file = io.open(worker_file, "w")
  if not file then
    err_print("Error: Could not create worker script")
    return false
  end
  file:write(script)
  file:close()
  return worker_file
end

-- Main processing func
local function process_corpus_parallel(directory, output_file)
  local dir_exists = io.open(directory, "r")
  if not dir_exists then
    err_print("Critical Error: Directory '" .. directory .. "' does not exist")
    return false
  end
  dir_exists:close()

  local chunk_files = {}
  local worker_processes = {}
  local cpu_count = get_cpu_count()
  dbg_print("Using " .. cpu_count .. " cores")

  local chunks = split_directory(directory, cpu_count)
  if has_critical_error then
    return false
  end

  local worker_script = create_worker_script()
  if not worker_script then
    err_print("Critical Error: Failed to create worker script")
    return false
  end

  -- if there are chunks to process
  local total_chunks = 0
  for _ in ipairs(chunks) do
    total_chunks = total_chunks + 1
  end

  if total_chunks == 0 then
    err_print("Critical Error: No files to process")
    os.remove(worker_script)
    return false
  end

  for i, chunk in ipairs(chunks) do
    local chunk_file = "chunk_" .. i .. ".txt"
    if write_chunk_file(chunk, chunk_file) then
      table.insert(chunk_files, chunk_file)
      local verbose_flag = verbose and " --verbose" or ""
      local cmd = "luajit " .. worker_script .. " " .. chunk_file .. verbose_flag
      dbg_print("Launching worker: " .. cmd)
      local is_unix = package.config:sub(1, 1) == '/'
      if is_unix then
        cmd = cmd .. " &"
        os.execute(cmd)
      else
        cmd = "start /B " .. cmd
        os.execute(cmd)
      end
      table.insert(worker_processes, cmd)
    end
  end

  dbg_print("Waiting for worker processes to complete...")
  local unigram_files = {}

  for _, chunk_file in ipairs(chunk_files) do
    local unigram_file = chunk_file .. ".unigrams"
    table.insert(unigram_files, unigram_file)
  end

  local timeout = 60 * 5 -- 5mins timeout
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
      -- waits 5seconds before checking again
      os.execute("sleep 5")
    end
  end

  if not all_complete then
    err_print("Error: Not all worker processes completed within the timeout")
    return false
  end

  local success = merge_unigram_files(unigram_files, output_file)

  -- cleanup
  for _, file in ipairs(chunk_files) do
    os.remove(file)
    os.remove(file .. ".unigrams")
  end
  os.remove(worker_script)
  return success
end

dbg_print("Starting ngram processing...")

local data_dir = "../data"
local data_dir_exists = io.open(data_dir, "r")
if not data_dir_exists then
  err_print("Critical Error: Data directory '../data' does not exist")
  os.exit(1)
end
data_dir_exists:close()

local cleaned_texts = "../data/cleaned-texts"
local cleaned_texts_exists = io.open(cleaned_texts, "r")
if not cleaned_texts_exists then
  err_print("Critical Error: Directory '../data/cleaned-texts' does not exist")
  err_print("Have you run `filter.py` to clean the corpus?")
  os.exit(1)
end
cleaned_texts_exists:close()

local success = process_corpus_parallel("../data/cleaned-texts", "../data/unigrams.bin")

if success then
  print("Successfully processed unigrams!")
  print("Now you can build the trie from ../data/unigrams.bin")
  print("Run: $ luajit build-trie.lua")
else
  err_print("Error: Failed to process unigrams")
  if not has_critical_error then
    err_print("Have you run `filter.py` to clean the corpus?")
    err_print("Ensure you have a dir named 'cleaned-texts' processed via `py filter.py' first")
  end
  os.exit(1)
end
