# Config

WordServe uses a TOML file to manage server, dictionary, and CLI settings.

Your client parsers can load or change this config file to adjust the server behavior OR send [config commands](#server-config-commands) directly to the server.

config files are loaded with the following priority:

1. _Custom path_ -  via `--config` flag  
2. _Default path_
   - `~/.config/wordserve/config.toml` (Linux/macOS) or `~/Library/Application Support/wordserve/config.toml` (fallback)
   - Windows: (falls back to executable directory in current version)
3. _defaults_ - if no config file is found, creates one

### Config Params

| Section | Parameter | Description | Default Value |
|:--------|:----------|:------------|:-------------:|
| **[server]** | `max_limit` | Maximum number of suggestions to return | 64 |
| | `min_prefix` | Minimum prefix length for suggestions | 1 |
| | `max_prefix` | Maximum prefix length for suggestions | 60 |
| | `enable_filter` | Enable input filtering (excludes numbers, symbols) | true |
| **[dict]** | `max_words` | Maximum number of words to load from dictionary | 50,000 |
| | `chunk_size` | Number of words per chunk for lazy loading | 10,000 |
| | `min_frequency_threshold` | Minimum frequency for word inclusion | 20 |
| | `min_frequency_short_prefix` | Min frequency for short prefix matches | 24 |
| | `max_word_count_validation` | Max words for validation during build | 1,000,000 |
| **[cli]** | `default_limit` | Default number of suggestions in CLI mode | 24 |
| | `default_min_len` | Default minimum prefix length for CLI | 1 |
| | `default_max_len` | Default maximum prefix length for CLI | 24 |
| | `default_no_filter` | Default filter setting for CLI mode | false |

#### Example config.toml

```toml
[server]
max_limit = 64
min_prefix = 1
max_prefix = 60
enable_filter = true

[dict]
max_words = 50000
chunk_size = 10000
min_frequency_threshold = 20
min_frequency_short_prefix = 24
max_word_count_validation = 1000000

[cli]
default_limit = 24
default_min_len = 1
default_max_len = 24
default_no_filter = false
```

### Server config commands

WordServe provides runtime config management through MessagePack IPC.

All commands are sent as **binary MessagePack** over stdin and receive binary MessagePack responses from stdout.

##### Dictionary Size

```ts
import { encode, decode } from '@msgpack/msgpack';

const request = { id: "dict_001", action: "set_size", chunk_count: 3 };
const binaryData = encode(request);
// Send binaryData over stdin
```

> Sets dict size to about 30,000 words (3 chunks × 10,000 words each).

**Get current dictionary info:**

```ts
const request = { id: "dict_002", action: "get_info" };
const binaryData = encode(request);
// Send binaryData, receive binary response, then:
const response = decode(binaryResponse);

// response = { id: "dict_002", status: "ok", current_chunks: 3, available_chunks: 5 }
```

**Get available dictionary size options:**

```ts
const request = { id: "dict_003", action: "get_options" };
const binaryData = encode(request);


// Response after decode():
// {
//   id: "dict_003", 
//   status: "ok",
//   options: [
//     { chunk_count: 1, word_count: 10000, size_label: "10K words" },
//     { chunk_count: 2, word_count: 20000, size_label: "20K words" },
//     { chunk_count: 3, word_count: 30000, size_label: "30K words" }
//   ]
// }
```

#### Config Path

**Get active path:**

```ts
const request = { id: "config_001", action: "get_config_path" };
const binaryData = encode(request);
```

**Rebuild config default values:**

```ts
const request = { id: "config_002", action: "rebuild_config" };
const binaryData = encode(request);
```

> **Note**: TOML parameters (server limits, filtering, etc.) require a server restart to take effect.
> Only the dictionary can be adjusted at runtime via MessagePack.
