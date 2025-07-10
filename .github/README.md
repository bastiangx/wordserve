
<h1 align="center">
  <a href="https://github.com/bastiangx/wordserve">
    <img src="https://files.catbox.moe/hy2y82.png" alt="WordServe Logo and banner">
  </a>
</h1>

<div align="center">
Lightweight prefix completion library | server, designed for any MessagePack clients!
  <br />
  <br />

<div align="center">
<img src="https://files.catbox.moe/vj4tms.gif" alt="MessagePack">
</div>

<br />
  <a href="https://github.com/dec0dOS/amazing-github-template/issues/new?assignees=&labels=bug&template=01_BUG_REPORT.md&title=bug%3A+">Report a Bug</a>
  ·
  <a href="https://github.com/dec0dOS/amazing-github-template/issues/new?assignees=&labels=enhancement&template=02_FEATURE_REQUEST.md&title=feat%3A+">Request a Feature</a>
</div>

<br>

#### What is this?

<table>
<tr>
<td>

WordServe is a minimalistic and  high performance **prefix completion library** with a server executable written in Go.

Its designed to provide auto-completion for various clients, especially those using [MessagePack](https://msgpack.org/index.html) as a serialization format.

#### Why?

So many tools and apps I use on daily basis do not offer any form of word completion, AI/NLP driven or otherwise, there are times when I need to quickly find a word or phrase that I know exists in my vocabulary, but I have no idea how to spell it or don't feel like typing for _that_ long.

Why not make my own tool that can power any TS/JS/etc clients with a completion server?

#### Similar to?

Think of this as a elementary nvim-cmp or vscode Intellisense daemon, but for any plugin/app that can use a MessagePack client. (which is super [easy to implement](https://www.npmjs.com/package/@msgpack/msgpack) and use compared to JSON parsing btw, in fact, about **411%** [improvement in speed](https://halilibrahimkocaoz.medium.com/message-queues-messagepack-vs-json-for-serialization-749914e3d0bb) and **40%** reduction in payload sizes)

> This is my first attempt on creating a small scaled but usable Go server/library. Expect unstable or incomplete features, as well as some bugs.
> I primarily made this for myself so I can make a completion plugin for [Obsidian](https://obsidian.md) but hey, you might find it useful too!

</td>
</tr>
</table>

### Prerequisites

- [Go 1.22](https://go.dev/doc/install) or later
- [Luajit 2.1](https://luajit.org/install.html) _(only for dictionary build scripts)_
  - A simple `words.txt` file for building the dictionary with most used words and their corresponding frequencies <span style="color: #908caa;"> -- see [dictionary](#dictionary) for more info</span>

## Installation

### Releases

Download the latest precompiled binaries from the [releases page](https://github.com/bastiangx/wordserve/releases/latest).

> pay attention to the OS and architecture of the binary you are downloading.

> If you'not not sure, use 'go install' from instructions below.

### Building from source

- using `go install` _(Recommended)_:

```sh
go install github.com/bastiangx/wordserve/cmd/wordserve@latest
```

You can also clone via git and build the old fashioned way:

```sh
git clone https://github.com/bastiangx/wordserve.git
cd wordserve
# -w -s strips debug info & symbols | alias wserve
go build -ldflags="-w -s" -o wserve ./cmd/wordserve/main.go
```

### Go

- use `go get` to add `wordserve` as a dependency in your project:

```sh
go get github.com/bastiangx/wordserve
```

and then import it in your code:

```go
import "github.com/bastiangx/wordserve/pkg/suggest"
```

## What can it do?

### Batched Word Suggestions

<img src="https://files.catbox.moe/h26n6q.png" alt="WordServe's average processing time is shown to be average around 150 milliseconds"  width="500">

### Responsive Server

<img src="https://files.catbox.moe/jbnp25.png" alt="IPC Server" width="500">

### MessagePack Integration

<img src="https://files.catbox.moe/7kwkwk.png" alt="MessagePack" height="300">

###

<img src="https://files.catbox.moe/14hvay.png" alt="Radix Trie Traversal" height="300">

### Limitless Suggestions

## What can it _not_ do?

As this is the early version and Beta, there are _many_ features that are yet not implemented, such as:

- fuzzy matching
- string searching algo (haystack-needle)
- spelling correction (aspell)
- use conventional dict formats like `.dict`

Will monitor the issues and usage to see if enough people are interested in adding them.

## Usage

### Standalone server

you can run `wordserve` as a dependency in your Go project, a standalone IPC server, and as a CLI in terminal to test its dictionary.

### Library API

Please follow these steps for manual setup:

1. [Download the precompiled template](https://github.com/dec0dOS/amazing-github-template/releases/download/latest/template.zip)
2. Replace all the [variables](#variables-reference) to your desired values
3. Initialize the repo in the precompiled template folder

    `or`

    Move the necessary files from precompiled template folder to your existing project directory. Don't forget the `.github` directory that may be hidden by default in your operating system

### Client Integration

You can inspect the _informal_ flow diagram on the internals of WordServe and how it returns suggestions to the client:

<a href="https://files.catbox.moe/6wy79k.png">
<img src="https://files.catbox.moe/6wy79k.png" alt="Flow Diagram" width="500">
</a>

### CLI

##### Flags

in terminal, run:

```sh
wordserve [flags]
```

| Flag       | Description                                                                                   | Default Value |
|:---------- |:--------------------------------------------------------------------------------------------- |:-------------:|
| -version   | Show current version                                                                          |     false     |
| -config    | Path to custom config.toml file                                                               |      ""       |
| -data      | Directory containing the binary files                                                         |    "data/"    |
| -v         | Toggle verbose mode                                                                           |     false     |
| -c         | Run CLI -- useful for testing and debugging                                                   |     false     |
| -limit     | Number of suggestions to return                                                               |      10       |
| -prmin     | Minimum Prefix length for suggestions (1 < n <= prmax)                                        |       3       |
| -prmax     | Maximum Prefix length for suggestions                                                         |      24       |
| -no-filter | Disable input filtering (DBG only) - shows all raw dictionary entries (numbers, symbols, etc) |     false     |
| -words     | Maximum number of words to load (use 0 for all words)                                         |    100,000     |
| -chunk     | Number of words per chunk for lazy loading                                                    |     10,000     |

## Dictionary

todo

## Configuration

todo

#### Server Config

todo

## Development

See the [open issues](https://github.com/bastiangx/wordserve/issues) for a list of proposed features (and known issues).

Contributions are welcome! Refer to the [contributing guidelines](./CONTRIBUTING.md)

## License

WordServe is licensed under the **MIT license**.
Feel free to edit and distribute this library as you like.

See [LICENSE](LICENSE)

## Acknowledgements

- Inspired _heavily_ by [fluent-typer extension](https://github.com/bartekplus/FluentTyper) made by Bartosz Tomczyk.
  - <span style="color: #908caa;">  Its a great extension to use on browsers, but I wanted something that can be used basically in any electron/local webapps with plugin clients, but also make it wayyy faster and more efficient since the depeendencies used there are way too bloated (C++ ...) and had too many bindings for my liking, and also more imporatantly, make this a good practice for me to learn how radix tries work for prefixes.</span>

- The _Beautiful_ [Rosepine theme](https://rosepinetheme.com/) used for graphics and screenshots throughout the readme.
