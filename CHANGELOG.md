# WordServe changelog

---

## [BETA - 0.9.0] - 2025-07-05

### Release (initial)

Starting with this release, WordServe is in beta.
This means that while the core functionality is stable,
some features may still be under development or subject to change.

#### Meat and Potatoes

- Prefix completion Server with msgpack
- Radix Trie prefix matching with frequency ranking
- Lazy-loaded chunked dictionaries
- Runtime config via TOML

#### Server (initial)

Client-agnostic interface for WordServe server with support for fast, flexible and minimal integration.

#### CLI [DBG] (initial)

Running WordServe in CLI mode for testing and debugging mainly for prefix, Trie traversal ops.
