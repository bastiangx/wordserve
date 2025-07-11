# API Usage

> [!IMPORTANT]
>
> 📖 Detailed API Reference are available in the [Go Docs](https://pkg.go.dev/github.com/bastiangx/wordserve/pkg/suggest)

##### Installation

```bash
go get github.com/bastiangx/wordserve
```

### Lazy Completer

for medium/large dictionaries (>10K words)

```go
completer := suggest.NewLazyCompleter("./data", 10000, 50000)

if err := completer.Initialize(); err != nil {
    log.Fatalf("Failed to initialize: %v", err)
}

suggestions := completer.Complete("amer", 10)
```

### Static Completer

for small dictionaries, dynamic word addition, embedded applications

```go
completer := suggest.NewCompleter()

completer.AddWord("example", 500)
completer.AddWord("excellent", 400)

suggestions := completer.Complete("ex", 5)
```

Not our main focus, but useful for small datasets or dynamic word lists.

##### example usage

```go
import "github.com/bastiangx/wordserve/pkg/suggest"

completer := suggest.NewCompleter()
completer.AddWord("hello", 1000)
completer.AddWord("help", 800)

suggestions := completer.Complete("hel", 10)
for _, s := range suggestions {
    fmt.Printf("%s (freq: %d)\n", s.Word, s.Frequency)
}
```

## env setup

```
go-project/
├── main.go
└── data/                    # Dictionary
    ├── words.txt           # Source word list
    ├── dict_0001.bin       # Binary chunks (auto-generated)
    ├── dict_0002.bin
    └── dict_0003.bin
```

Learn how worserve uses its [dictionaries](./dictionary.md)

### data files

WordServe automatically handles missing files:

```go
completer := suggest.NewLazyCompleter("./data", 10000, 30000)
err := completer.Initialize()

// 1. Files exist → loads immediately
// 2. Files missing → attempts local generation (requires LuaJIT)
// 3. Generation fails → downloads from GitHub releases
// 4. Download fails → returns error
```

##### LuaJIT

For local, install [LuaJIT 2.1+](https://luajit.org/install.html):

```bash
# macOS
brew install luajit

# Ubuntu/Debian  
sudo apt-get install luajit

# Windows
# Download from https://luajit.org/install.html
```

#### common issues

Missing Data Directory

```go
// Bad: relative path
completer := suggest.NewLazyCompleter("data", 10000, 50000)

// Good: explicit path
dataDir, err := filepath.Abs("./data")
if err != nil {
    log.Fatalf("Path error: %v", err)
}
completer := suggest.NewLazyCompleter(dataDir, 10000, 50000)
```

Permission

```go
if _, err := os.Stat("./data"); os.IsNotExist(err) {
    if err := os.MkdirAll("./data", 0755); err != nil {
        log.Fatalf("Cannot create data directory: %v", err)
    }
}
```

## Core usage

```go
suggestions := completer.Complete("pref", 10)

suggestions = completer.Complete("PREF", 10)  // Returns "PREFIX", "PREFACE", etc.
suggestions = completer.Complete("Pref", 10)  // Returns "Prefix", "Preface", etc.

err := completer.CompleteWithCallback("pref", 10, func(s suggest.Suggestion) bool {
    fmt.Printf("%s (%d)\n", s.Word, s.Frequency)
    return true
})
```

#### Memory

```go
// cleanups (every 50-100 requests)
requestCount := 0
for {
    suggestions := completer.Complete(prefix, 10)
    requestCount++

    if requestCount%50 == 0 {
        completer.ForceCleanup()  // GC
    }
}
defer completer.Stop()
```

#### Stats

```go
stats := completer.Stats()
fmt.Printf("Total words: %d\n", stats["totalWords"])
fmt.Printf("Max frequency: %d\n", stats["maxFrequency"])

if stats["chunkLoader"] == 1 {
    fmt.Printf("Loaded chunks: %d/%d\n", 
        stats["loadedChunks"], stats["availableChunks"])
}
```

#### Dynamic loading

```go
// Request more words
err := completer.RequestMoreWords(20000)
if err != nil {
    log.Printf("Failed to load more words: %v", err)
}

completer.InvalidateFallbackCache()
```

### Web server example

```go
type CompletionService struct {
    completer suggest.ICompleter
    mutex     sync.RWMutex
}

func NewCompletionService(dataDir string) (*CompletionService, error) {
    completer := suggest.NewLazyCompleter(dataDir, 10000, 100000)
    if err := completer.Initialize(); err != nil {
        return nil, fmt.Errorf("failed to initialize completer: %w", err)
    }
    
    return &CompletionService{completer: completer}, nil
}

func (cs *CompletionService) Complete(prefix string, limit int) []suggest.Suggestion {
    cs.mutex.RLock()
    defer cs.mutex.RUnlock()
    
    if len(prefix) < 1 || len(prefix) > 50 {
        return []suggest.Suggestion{}
    }
    
    return cs.completer.Complete(prefix, min(limit, 20))
}

func (cs *CompletionService) Shutdown() {
    cs.mutex.Lock()
    defer cs.mutex.Unlock()
    cs.completer.Stop()
}

// init
func initializeCompleterWithRetry(dataDir string, maxRetries int) (suggest.ICompleter, error) {
    var lastErr error
    
    for attempt := 1; attempt <= maxRetries; attempt++ {
        completer := suggest.NewLazyCompleter(dataDir, 10000, 50000)
        err := completer.Initialize()
        
        if err == nil {
            log.Printf("Completer initialized successfully on attempt %d", attempt)
            return completer, nil
        }
        
        lastErr = err
        log.Printf("Initialization attempt %d failed: %v", attempt, err)
        
        if attempt < maxRetries {
            time.Sleep(time.Duration(attempt) * time.Second)
        }
    }
    
    return nil, fmt.Errorf("failed to initialize after %d attempts: %w", maxRetries, lastErr)
}
```

##### Health Checks

```go
func (cs *CompletionService) HealthCheck() error {
    suggestions := cs.Complete("test", 1)
    if len(suggestions) == 0 {
        return errors.New("completer returned no results for test prefix")
    }
    
    stats := cs.completer.Stats()
    if stats["totalWords"] == 0 {
        return errors.New("completer has no loaded words")
    }
    
    return nil
}
```

#### Choosing parameters

```go
// mem-optimized (faster startup, limited coverage)
completer := suggest.NewLazyCompleter("./data", 5000, 25000)

// Balanced (good allrounder)
completer := suggest.NewLazyCompleter("./data", 10000, 50000)

// coverage-optimized (slower startup, comprehensive results)
completer := suggest.NewLazyCompleter("./data", 15000, 100000)
```
