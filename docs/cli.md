# CLI Guide

The simple WordServe CLI helps you verify your setup, debug issues, and experiment with completion settings before deploying the server.

<img src="https://files.catbox.moe/acyd8z.gif" alt="WordServe CLI debugger in action">

<br />
<br />

The CLI's main features are

- *env* - make sure your system can run WordServe
- *Dictionary Validation* - Verify data/ files are present and loaded
- *Perf* - Measure completion times and memory usage
- *Config Debugging* - Test different settings before committing to config files

## Basics

### Flags

##### Options

| Flag | Description | Default |  |
|------|-------------|---------|-------|
| `-c` | toggle CLI mode | `false` | Would just run server mode if omitted |
| `-v` | Verbose logging | `false` | Shows timing, loading, and debug logs |
| `-data` | Dictionary directory | `"data/"` | Path to your `.bin` files |
| `-config` | Custom config file | `""` | Override default config location |

##### Behaviour

| Flag | Description | Default | Use Case |
|------|-------------|---------|----------|
| `-limit` | Max suggestions returned | `24` | Test different result sizes |
| `-prmin` | Minimum prefix length | `1` | Set shortest valid input |
| `-prmax` | Maximum prefix length | `24` | Set longest valid input |
| `-no-filter` | Disable input filtering | `false` | Debug raw dictionary content |

##### Dictionary

| Flag | Description | Default | Impact |
|------|-------------|---------|--------|
| `-words` | Max words to load | `50,000` | Control memory usage |
| `-chunk` | Words per chunk | `10,000` | Affects loading patterns |

### Usage

```bash
# Start interactive CLI with default settings
./wordserve -c

# Test with verbose logging
./wordserve -c -v

# Test with specific dictionary size
./wordserve -c -v -words 30000
```

### Common Workflows

env check

```bash
./wordserve -c -v -data ./data
```

perf check

```bash
# Test with larger dictionary
./wordserve -c -words 100000 -limit 50 -v
```

debug misc issues

```bash
# no filtering (shows all entries in words.txt)
./wordserve -c -v -no-filter -prmin 1
```

## Binds

Right now the only keybinds available are literally just `enter` to submit the input, and `ctrl c` to exit.

## Troubleshooting

`Failed to init completer` err

```bash
# Check if dict files exist
./wordserve -c -v -data ./data

# If files are missing, they'll be auto-generated or downloaded
# Look for these messages in verbose output:
# "not enough dictionary files found, attempting to generate them..."

# or run the luajit script manually
# luajit scripts/generate_dict.lua
```
