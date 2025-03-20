#!/usr/bin/env python3

import os
import re
import string
import nltk
from nltk.corpus import stopwords
import argparse
from tqdm import tqdm
import multiprocessing

# Download NLTK resources if not already downloaded
def download_nltk_resources():
    try:
        nltk.data.find('corpora/stopwords')
    except LookupError:
        print("Downloading NLTK stopwords...")
        nltk.download('stopwords')

# Initialize stop words
def get_stop_words():
    # Get standard English stop words from NLTK
    stop_words = set(stopwords.words('english'))
    
    # Add common contractions and their expansions
    contractions = {
        "im", "youre", "hes", "shes", "its", "were", "theyre",
        "ill", "youll", "hell", "shell", "itll", "well", "theyll",
        "cant", "dont", "wont", "couldnt", "shouldnt", "wouldnt",
        "ive", "youve", "weve", "theyve"
    }
    stop_words.update(contractions)
    
    return stop_words

# Detect gibberish words
def is_gibberish(word):
    # Very short words
    if len(word) < 2:
        return True
        
    # Words that are too long
    if len(word) > 25:
        return True
    
    # Words with repetitive characters (4 or more same chars in a row)
    if re.search(r'(.)\1{3,}', word):
        return True
    
    # Words with strange character distributions
    # English words typically have vowels
    if len(word) > 5 and not re.search(r'[aeiou]', word):
        return True
    
    # Words with unusual character patterns
    keyboard_patterns = [
        'qwert', 'asdfg', 'zxcvb', 'yuiop', 'hjkl', 'bnm',
        'poiuy', 'lkjhg', 'fdsa', 'mnbvc'
    ]
    for pattern in keyboard_patterns:
        if pattern in word:
            return True
    
    return False

# Clean and filter text content
def clean_text(text, stop_words):
    # Remove special markers and HTML tags
    text = re.sub(r'@@\d+', ' ', text)
    text = re.sub(r'<[^>]+>', ' ', text)
    
    # Split into words and process
    words = []
    for word in text.split():
        # Convert to lowercase and remove punctuation
        clean_word = word.lower()
        clean_word = re.sub(r'[^a-z]', '', clean_word)
        
        # Skip empty words, stop words, and gibberish
        if (clean_word and 
            clean_word not in stop_words and 
            not is_gibberish(clean_word)):
            words.append(clean_word)
    
    return ' '.join(words)

# Process a single file
def process_file(args):
    input_file, output_file, stop_words = args
    try:
        with open(input_file, 'r', encoding='utf-8', errors='ignore') as f:
            text = f.read()
        
        # Clean and filter the text
        filtered_text = clean_text(text, stop_words)
        
        # Write the filtered text to the output file
        with open(output_file, 'w', encoding='utf-8') as f:
            f.write(filtered_text)
            
        return True
    except Exception as e:
        print(f"Error processing {input_file}: {e}")
        return False

def main():
    parser = argparse.ArgumentParser(description='Filter corpus files to remove stop words and gibberish.')
    parser.add_argument('--input', default='corpus', help='Input corpus directory')
    parser.add_argument('--output', default='corpus_stopwords', help='Output directory for filtered files')
    args = parser.parse_args()
    
    # Download NLTK resources if needed
    download_nltk_resources()
    
    # Get stop words
    stop_words = get_stop_words()
    print(f"Loaded {len(stop_words)} stop words")
    
    # Create output directory if it doesn't exist
    os.makedirs(args.output, exist_ok=True)
    
    # Get list of text files in input directory
    input_files = []
    for root, _, files in os.walk(args.input):
        for file in files:
            if file.endswith('.txt'):
                input_path = os.path.join(root, file)
                relative_path = os.path.relpath(input_path, args.input)
                output_path = os.path.join(args.output, relative_path)
                
                # Create subdirectories in output if needed
                os.makedirs(os.path.dirname(output_path), exist_ok=True)
                
                input_files.append((input_path, output_path, stop_words))
    
    print(f"Found {len(input_files)} text files to process")
    
    # Process files in parallel
    pool = multiprocessing.Pool(processes=max(1, multiprocessing.cpu_count() - 1))
    results = list(tqdm(pool.imap(process_file, input_files), total=len(input_files)))
    pool.close()
    pool.join()
    
    success_count = sum(results)
    print(f"Successfully processed {success_count} of {len(input_files)} files")
    print(f"Filtered corpus saved to {args.output}/")

if __name__ == "__main__":
    main()