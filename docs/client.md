# Client Integration

WordServe communicates through **binary MessagePack** over stdin/stdout:

```bash
bun install @msgpack/msgpack
```

#### Basic example

```ts
import { spawn, ChildProcess } from 'child_process';
import { encode, decode } from '@msgpack/msgpack';

class WordServeClient {
  private process: ChildProcess;
  private requestId = 0;

  constructor(binaryPath: string = 'wordserve') {
    this.process = spawn(binaryPath, [], {
      stdio: ['pipe', 'pipe', 'pipe']
    });
  }

  async getCompletions(prefix: string, limit: number = 20): Promise<Suggestion[]> {
    const request = {
      id: `req_${++this.requestId}`,
      p: prefix,
      l: limit      // (optional)
    };

    const binaryRequest = encode(request);
    this.process.stdin!.write(binaryRequest);

    return new Promise((resolve, reject) => {
      this.process.stdout!.once('data', (data: Buffer) => {
        try {
          const response = decode(data) as CompletionResponse;
          const suggestions = response.s.map((s, index) => ({
            word: s.w,
            rank: s.r,
            frequency: 65536 - s.r // Convert rank back to freq score
          }));
          resolve(suggestions);
        } catch (error) {
          reject(error);
        }
      });
    });
  }
}
```

## Interfaces

```ts
// Request Types
interface CompletionRequest {
  id: string;           // Unique request identifier
  p: string;            // Prefix to complete
  l?: number;           // Max suggestions (optional, server enforces its limits)
}

interface DictionaryRequest {
  id: string;           // Request identifier
  action: string;       // "get_info" | "set_size" | "get_options"
  chunk_count?: number; // For "set_size" action
}

interface ConfigRequest {
  id: string;           // Request identifier  
  action: string;       // "get_config_path" | "rebuild_config"
}

// Response Types
interface CompletionResponse {
  id: string;                    // Matches request ID
  s: CompletionSuggestion[];     // Array of suggestions
  c: number;                     // Count of suggestions
  t: number;                     // Time taken (microseconds)
}

interface CompletionSuggestion {
  w: string;          // Word
  r: number;          // Rank (1 = highest frequency)
}

interface DictionaryResponse {
  id: string;                    // Matches request ID
  status: string;                // "ok" | "error"
  error?: string;                // Error message if status = "error"
  current_chunks?: number;       // Currently loaded chunks
  available_chunks?: number;     // Total available chunks
  options?: DictionarySizeOption[]; // Available size options
}

interface DictionarySizeOption {
  chunk_count: number;    // Number of chunks
  word_count: number;     // Total words
  size_label: string;     // readable label (e.g., "30K words")
}

// client convenience types
interface Suggestion {
  word: string;
  rank: number;
  frequency: number;
}
```

## Ectended example

```typescript
import { spawn, ChildProcess } from 'child_process';
import { encode, decode } from '@msgpack/msgpack';

export class WordServeClient {
  private process: ChildProcess;
  private requestId = 0;
  private responseHandlers = new Map<string, (data: any) => void>();

  constructor(
    binaryPath: string = 'wordserve',
    options: { dataDir?: string; configPath?: string } = {}
  ) {
    const args: string[] = [];
    
    if (options.dataDir) {
      args.push('-data', options.dataDir);
    }
    if (options.configPath) {
      args.push('-config', options.configPath);
    }

    this.process = spawn(binaryPath, args, {
      stdio: ['pipe', 'pipe', 'pipe']
    });

    this.setupResponseHandler();
    this.setupErrorHandler();
  }

  private setupResponseHandler(): void {
    this.process.stdout!.on('data', (data: Buffer) => {
      try {
        const response = decode(data) as any;
        const handler = this.responseHandlers.get(response.id);
        if (handler) {
          handler(response);
          this.responseHandlers.delete(response.id);
        }
      } catch (error) {
        console.error('Failed to decode response:', error);
      }
    });
  }

  private setupErrorHandler(): void {
    this.process.stderr!.on('data', (data: Buffer) => {
      console.error('WordServe error:', data.toString());
    });

    this.process.on('exit', (code) => {
      console.log(`WordServe process exited with code ${code}`);
    });
  }

  private sendRequest<T>(request: any): Promise<T> {
    const requestId = `req_${++this.requestId}`;
    request.id = requestId;

    return new Promise((resolve, reject) => {
      this.responseHandlers.set(requestId, (response) => {
        if (response.status === 'error') {
          reject(new Error(response.error || 'Unknown error'));
        } else {
          resolve(response);
        }
      });

      // Send request
      try {
        const binaryRequest = encode(request);
        this.process.stdin!.write(binaryRequest);
      } catch (error) {
        this.responseHandlers.delete(requestId);
        reject(error);
      }

      // 5 seconds
      setTimeout(() => {
        if (this.responseHandlers.has(requestId)) {
          this.responseHandlers.delete(requestId);
          reject(new Error('Request timeout'));
        }
      }, 5000);
    });
  }

  // Main completion
  async getCompletions(prefix: string, limit: number = 20): Promise<Suggestion[]> {
    if (!prefix || prefix.length === 0) {
      return [];
    }

    const request: CompletionRequest = {
      id: '', /// auto-generated
      p: prefix,
      l: Math.min(limit, 64) // limit
    };

    const response = await this.sendRequest<CompletionResponse>(request);
    
    return response.s.map((suggestion, index) => ({
      word: suggestion.w,
      rank: suggestion.r,
      frequency: 65536 - suggestion.r + 1
    }));
  }

  // dict management
  async getDictionaryInfo(): Promise<{ currentChunks: number; availableChunks: number }> {
    const request: DictionaryRequest = {
      id: '',
      action: 'get_info'
    };

    const response = await this.sendRequest<DictionaryResponse>(request);
    return {
      currentChunks: response.current_chunks || 0,
      availableChunks: response.available_chunks || 0
    };
  }

  async setDictionarySize(chunkCount: number): Promise<void> {
    const request: DictionaryRequest = {
      id: '',
      action: 'set_size',
      chunk_count: chunkCount
    };

    await this.sendRequest<DictionaryResponse>(request);
  }

  async getDictionarySizeOptions(): Promise<DictionarySizeOption[]> {
    const request: DictionaryRequest = {
      id: '',
      action: 'get_options'
    };

    const response = await this.sendRequest<DictionaryResponse>(request);
    return response.options || [];
  }

  // config management
  async getConfigPath(): Promise<string> {
    const request: ConfigRequest = {
      id: '',
      action: 'get_config_path'
    };

    const response = await this.sendRequest<any>(request);
    return response.config_path || '';
  }

  async rebuildConfig(): Promise<void> {
    const request: ConfigRequest = {
      id: '',
      action: 'rebuild_config'
    };

    await this.sendRequest<any>(request);
  }

  // Cleanup
  destroy(): void {
    if (this.process && !this.process.killed) {
      this.process.kill('SIGTERM');
    }
  }
}
```

## Usage

### Basic completion

```ts
const client = new WordServeClient();

const suggestions = await client.getCompletions('amer', 10);
console.log(suggestions);
// [
//   { word: 'america', rank: 1, frequency: 65535 },
//   { word: 'american', rank: 2, frequency: 65534 },
//   { word: 'americans', rank: 3, frequency: 65533 }
// ]
```

### Completion (debounced)

```ts
class DebounceCompleter {
  private client: WordServeClient;
  private timeoutId: NodeJS.Timeout | null = null;

  constructor() {
    this.client = new WordServeClient();
  }

  getCompletions(
    prefix: string, 
    callback: (suggestions: Suggestion[]) => void,
    delay: number = 150
  ): void {
    if (this.timeoutId) {
      clearTimeout(this.timeoutId);
    }

    // new timeout
    this.timeoutId = setTimeout(async () => {
      try {
        const suggestions = await this.client.getCompletions(prefix, 15);
        callback(suggestions);
      } catch (error) {
        console.error('Completion error:', error);
        callback([]);
      }
    }, delay);
  }
}

// Usage
const completer = new DebounceCompleter();

// Simulate typing
completer.getCompletions('hel', (suggestions) => {
  console.log('Suggestions:', suggestions.map(s => s.word));
});
```

### Dynamic dict management

```typescript
class AdaptiveDictionary {
  private client: WordServeClient;

  constructor() {
    this.client = new WordServeClient();
  }

  async optimizeForPerformance(): Promise<void> {
    const options = await this.client.getDictionarySizeOptions();
    
    const mediumOption = options[Math.floor(options.length / 2)];
    
    await this.client.setDictionarySize(mediumOption.chunk_count);
    console.log(`Dictionary set to ${mediumOption.size_label}`);
  }

  async getCurrentStatus(): Promise<void> {
    const info = await this.client.getDictionaryInfo();
    console.log(`Loaded: ${info.currentChunks}/${info.availableChunks} chunks`);
  }
}
```

### Errors

```ts
async function robustCompletion(client: WordServeClient, prefix: string): Promise<Suggestion[]> {
  try {
    if (!prefix || prefix.length < 1) {
      return [];
    }

    if (prefix.length > 50) {
      prefix = prefix.substring(0, 50);
    }

    const suggestions = await client.getCompletions(prefix, 20);
    return suggestions;

  } catch (error) {
    console.warn(`Completion failed for "${prefix}":`, error);
    return [];
  }
}
```

## Considerations

### request batching

Avoid sending too many rapid requests:

```ts
class RateLimitedClient {
  private client: WordServeClient;
  private lastRequest = 0;
  private minInterval = 20; // 20ms minimum between requests

  constructor() {
    this.client = new WordServeClient();
  }

  async getCompletions(prefix: string, limit?: number): Promise<Suggestion[]> {
    const now = Date.now();
    const timeSinceLastRequest = now - this.lastRequest;
    
    if (timeSinceLastRequest < this.minInterval) {
      await new Promise(resolve => 
        setTimeout(resolve, this.minInterval - timeSinceLastRequest)
      );
    }

    this.lastRequest = Date.now();
    return this.client.getCompletions(prefix, limit);
  }
}
```

### memory

```typescript
// Clean up properly
process.on('exit', () => {
  client.destroy();
});

process.on('SIGINT', () => {
  client.destroy();
  process.exit(0);
});
```

## Server config

#### Default Limits

1. *Prefix length*: 1-60 characters
2. *Max suggestions*: 64 per request
3. *Request rate*: No built-in limits

Learn more about [configs](./config.md)

```ts
// custom config
const client = new WordServeClient('wordserve', {
  dataDir: './custom-dictionaries',
  configPath: './config.toml'
});

await client.setDictionarySize(3); // ~30K words
```

