#!/usr/bin/env python3

## filter.py simply filters corpus of text files
## by removing stop words and gibberish words and some specfic patterns.

import os
import re
import argparse
from tqdm import tqdm
import multiprocessing
import sys


def get_stop_words():
    return set()


# Gibberish are explicitely malformed, short, or nonsensical words.
# Its found by checking absence of vowels, length, and character patterns
def is_gibberish(word):
    # very long or short
    if len(word) < 2:
        return True
    if len(word) > 32:
        return True

    # repeated 4 characters: e.g. "aaaa", "bbbb"
    # or repeated intervals: e.g. "ababab", "cdcdcd"
    if re.search(r"(.)\1{3,}", word):
        return True

    # no vowels in long words
    if len(word) > 5 and not re.search(r"[aeiou]", word):
        return True

    # keyboard patterns
    kbd_patt = [
        "qwert",
        "asdfg",
        "zxcvb",
        "yuiop",
        "hjkl",
        "bnm",
        "poiuy",
        "lkjhg",
        "fdsa",
        "mnbvc",
        "qazwsx",
        "edcrfv",
        "tgbnhy",
        "ujmki",
        "plm",
        "qaz",
        "wsx",
        "edc",
        "rfv",
        "tgb",
        "yhn",
        "ujm",
        "ikm",
        " plm",
    ]
    for pattern in kbd_patt:
        if pattern in word:
            return True
    return False


# clean by removing special markers, HTML-tags, stop words + gibberish
def clean_text(text, stop_words):
    if not text or not text.strip():
        return ""

    words = []
    text = re.sub(r"@@\d+", " ", text)
    text = re.sub(r"<[^>]+>", " ", text)

    for word in text.split():
        clean_word = word.lower()
        clean_word = re.sub(r"[^a-z]", "", clean_word)
        if clean_word and clean_word not in stop_words and not is_gibberish(clean_word):
            words.append(clean_word)
    return " ".join(words)


def process_file(args):
    input_file, output_file, stop_words = args
    try:
        if not os.path.exists(input_file):
            print(f"ERROR: Input file does not exist: {input_file}")
            return False

        if not os.access(input_file, os.R_OK):
            print(f"ERROR: Cannot read input file: {input_file}")
            return False

        file_size = os.path.getsize(input_file)
        if file_size == 0:
            print(f"WARNING: Empty file skipped: {input_file}")
            return False

        if file_size > 100 * 1024 * 1024:  # 100MB
            print(
                f"WARNING: File too large, skipping: {input_file} ({file_size} bytes)"
            )
            return False

        with open(input_file, "r", encoding="utf-8", errors="ignore") as f:
            text = f.read()

        if not text or not text.strip():
            print(f"WARNING: File contains no readable text: {input_file}")
            return False

        filtered_text = clean_text(text, stop_words)

        if not filtered_text:
            print(f"WARNING: No content remaining after filtering: {input_file}")

        output_dir = os.path.dirname(output_file)
        if output_dir and not os.path.exists(output_dir):
            os.makedirs(output_dir, exist_ok=True)

        if not os.access(os.path.dirname(output_file) or ".", os.W_OK):
            print(
                f"ERROR: Cannot write to output directory: {os.path.dirname(output_file)}"
            )
            return False

        with open(output_file, "w", encoding="utf-8") as f:
            f.write(filtered_text)
        return True

    except UnicodeDecodeError as e:
        print(f"ERROR: Cannot decode file {input_file}: {e}")
        return False
    except PermissionError as e:
        print(f"ERROR: Permission denied for {input_file}: {e}")
        return False
    except OSError as e:
        print(f"ERROR: OS error processing {input_file}: {e}")
        return False
    except Exception as e:
        print(f"ERROR: Unexpected error processing {input_file}: {e}")
        return False


def main():
    parser = argparse.ArgumentParser(
        description="Filter txt files and remove stop words + gibberish. put `--input DIR/` as a directory that contains text files."
    )
    parser.add_argument("--input", default="raw-texts", help="Input text directory")
    parser.add_argument(
        "--output",
        default="../data/cleaned-texts",
        help="Output directory for cleaned files",
    )
    args = parser.parse_args()

    if not os.path.exists(args.input):
        print(f"ERROR: Input directory does not exist: {args.input}")
        sys.exit(1)

    if not os.path.isdir(args.input):
        print(f"ERROR: Input path is not a directory: {args.input}")
        sys.exit(1)

    if not os.access(args.input, os.R_OK):
        print(f"ERROR: Cannot read input directory: {args.input}")
        sys.exit(1)

    try:
        os.makedirs(args.output, exist_ok=True)
    except Exception as e:
        print(f"ERROR: Cannot create output directory {args.output}: {e}")
        sys.exit(1)

    if not os.access(args.output, os.W_OK):
        print(f"ERROR: Cannot write to output directory: {args.output}")
        sys.exit(1)

    # NLTK resources and load stop words
    stop_words = get_stop_words()
    print(f"Loaded {len(stop_words)} stop words")

    # get list of input files
    input_files = []
    txt_file_count = 0

    try:
        for root, dirs, files in os.walk(args.input):
            for file in files:
                if file.endswith(".txt"):
                    txt_file_count += 1
                    input_path = os.path.join(root, file)
                    relative_path = os.path.relpath(input_path, args.input)
                    output_path = os.path.join(args.output, relative_path)

                    try:
                        os.makedirs(os.path.dirname(output_path), exist_ok=True)
                        input_files.append((input_path, output_path, stop_words))
                    except Exception as e:
                        print(
                            f"ERROR: Cannot create output directory for {output_path}: {e}"
                        )
                        continue
    except Exception as e:
        print(f"ERROR: Failed to scan input directory {args.input}: {e}")
        sys.exit(1)

    if txt_file_count == 0:
        print(f"ERROR: No .txt files found in {args.input}")
        sys.exit(1)

    if len(input_files) == 0:
        print(f"ERROR: No processable .txt files found in {args.input}")
        sys.exit(1)

    print(f"Found {len(input_files)} text files to process")

    try:
        pool = multiprocessing.Pool(processes=max(1, multiprocessing.cpu_count() - 1))
        results = list(
            tqdm(pool.imap(process_file, input_files), total=len(input_files))
        )
        pool.close()
        pool.join()
    except KeyboardInterrupt:
        print("\nERROR: Processing interrupted by user")
        pool.terminate()
        pool.join()
        sys.exit(1)
    except Exception as e:
        print(f"ERROR: Multiprocessing failed: {e}")
        sys.exit(1)

    success_count = sum(results)
    failed_count = len(input_files) - success_count

    print(f"Successfully processed {success_count} of {len(input_files)} files")
    if failed_count > 0:
        print(f"Failed to process {failed_count} files")
    print(f"Filtered text saved to {args.output}/")

    if success_count == 0:
        print("ERROR: No files were successfully processed")
        sys.exit(1)
    elif failed_count > 0:
        sys.exit(2)
    else:
        sys.exit(0)


if __name__ == "__main__":
    main()
