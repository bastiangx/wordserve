
<h1 align="center">
  <a href="https://github.com/bastiangx/wordserve">
    <img src="https://files.catbox.moe/hy2y82.png" alt="WordServe Logo and banner">
  </a>
</h1>

<div align="center">
Lightweight prefix completion libray|server, designed for any msgpack clients!
  <br />
  <br />
  <a href="https://github.com/dec0dOS/amazing-github-template/issues/new?assignees=&labels=bug&template=01_BUG_REPORT.md&title=bug%3A+">Report a Bug</a>
  ·
  <a href="https://github.com/dec0dOS/amazing-github-template/issues/new?assignees=&labels=enhancement&template=02_FEATURE_REQUEST.md&title=feat%3A+">Request a Feature</a>
</div>

<div align="center">
<br />

</div>

<br>

#### What is this?

<table>
<tr>
<td>

WordServe is a minimalistic, _actually_ high perf prefix completion library with a daemon (+dbg cli) written in Go.
Its designed to provide truly fast completion for various clients, especially those using [MessagePack](https://msgpack.org/index.html) as a serialization format.

#### Why?

So many tools and apps I use on daily basis do not offer any form of word completion, AI/NLP driven or otherwise, there are times when I need to quickly find a word or phrase that I know exists in my vocabulary, but I have no idea how to spell it or don't feel like typing for _that_ long.

Why not make my own tool that can power any TS/JS/etc clients with a completion server? 

#### Similar to?

Think of this as a elementary nvim-cmp or vscode intellisense daemon, but for any plugn/app that can use a msgpack client. (which is super [easy to implement](https://www.npmjs.com/package/@msgpack/msgpack) and and use compared to json parsing btw, in fact, about **411%** [improvement in speed](https://halilibrahimkocaoz.medium.com/message-queues-messagepack-vs-json-for-serialization-749914e3d0bb) and **40%** reduction in payload sizes)

> This is my first attempt on creating a small scaled but usable Go server/library. Expect unstable or incomplete features, as well as some bugs.
> I primarily made this for myself so I can make a completion plugin for [Obsidian](https://obsidian.md) but hey, you might find it useful too!

</td>
</tr>
</table>

### Prerequisites

- [Go 1.22](https://go.dev/doc/install) or later 
- [Luajit 2.1](https://luajit.org/install.html) _(only for dictionary build scripts)_
  + A simple `words.txt` file for building the dictionary with most used words and their corresponding frequencies <span style="color: #908caa;"> -- see [dictionary](#dictionary) for more info</span>

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

You can also clone via git and build it the old fashioned way:

```sh
git clone https://github.com/bastiangx/wordserve.git
cd wordserve
# -w -s strips debug info & symbols | alias wserve
go build -ldflags="-w -s" -o wserve ./cmd/wordserve/main.go
```

### Library

- use `go get` to add wordserve as a dependency in your project:

```sh
go get github.com/bastiangx/wordserve
```

and then import it in your code:

```go
import "github.com/bastiangx/wordserve/pkg/suggest"
```

## What can it do?


### Radix Trie Traversal
<img src="https://files.catbox.moe/h26n6q.png" alt="Radix Trie Traversal"  width="500">

### IPC
<img src="https://files.catbox.moe/jbnp25.png" alt="IPC Server" width="500">

### MessagePack

<img src="https://files.catbox.moe/7kwkwk.png" alt="MessagePack" height="300">



### Dictionary

<img src="https://files.catbox.moe/14hvay.png" alt="Radix Trie Traversal" height="300">

### Limitless Suggestions



## Usage

### Standalone server

you can run `wordserve` as a standalone IPC server, as a CLI in terminal to test the dictionary or as a library in your Go project.


### Library API

Please follow these steps for manual setup:

1. [Download the precompiled template](https://github.com/dec0dOS/amazing-github-template/releases/download/latest/template.zip)
2. Replace all the [variables](#variables-reference) to your desired values
3. Initialize the repo in the precompiled template folder

    `or`

    Move the necessary files from precompiled template folder to your existing project directory. Don't forget the `.github` directory that may be hidden by default in your operating system

### Client Integration


#### Flow 

You can inspect the _informal_ flow diagram on the internals of WordServe and how it returns suggestions to the client: _[high quality image](https://files.catbox.moe/6wy79k.png)_

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
| :--------- | :-------------------------------------------------------------------------------------------- | :------------ |
| -version   | Show current version                                                                          | false         |
| -config    | Path to custom config.toml file                                                               | ""            |
| -data      | Directory containing the binary files                                                         | "data/"       |
| -d         | Toggle verbose mode                                                                           | false         |
| -c         | Run CLI -- useful for testing and debugging                                                   | false         |
| -limit     | Number of suggestions to return                                                               | 10            |
| -prmin     | Minimum prefix length for suggestions (1 < n <= prmax)                                        | 3             |
| -prmax     | Maximum prefix length for suggestions                                                         | 24            |
| -no-filter | Disable input filtering (DBG only) - shows all raw dictionary entries (numbers, symbols, etc) | false         |
| -words     | Maximum number of words to load (use 0 for all words)                                         | 100000        |
| -chunk     | Number of words per chunk for lazy loading                                                    | 10000         |

## Dictionary

## Configuration

#### Server Config



## Development

See the [open issues](https://github.com/bastiangx/wordserve/issues) for a list of proposed features (and known issues).

Contributions are welcome! Refer to the [contributing guidelines](./CONTRIBUTING.md)



## License

This project is licensed under the **MIT license**. 
Feel free to edit and distribute this library as you like.

See [LICENSE](LICENSE)


## Acknowledgements

- Inspired _heavily_ by [fluent-typer extension](https://github.com/bartekplus/FluentTyper) made by Bartosz Tomczyk.
  - <span style="color: #908caa;">  Its a great extension to use on browsers, but I wanted something that can be used basically in any electron/local webapps with plugin clients, but also make it wayyy faster and more efficient since the depeendencies used there are way too bloated (C++ ...) and had too many bindings for my liking, and also more imporatantly, make this a good practice for me to learn how radix tries work for prefixes.</span>

- The _Beautiful_ [Rosepine theme](https://rosepinetheme.com/) used for graphics and screenshots throughout the readme.