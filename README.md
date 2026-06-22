# a2a-probe

A command-line client for the [A2A (Agent-to-Agent) Protocol](https://google.github.io/A2A/) v0.3.0, written in Go.

> Ported from the original Kotlin/GraalVM implementation. The Go build produces a small, statically-linked binary with no runtime dependencies.

## Installation

### Homebrew (macOS / Linux)

```bash
brew tap CodeYogiCo/tap
brew install a2a-probe
```

### Docker

```bash
docker pull ghcr.io/codeyogico/a2a-probe:latest
docker run --rm ghcr.io/codeyogico/a2a-probe send "Hello"
```

For convenience, add a shell alias:

```bash
alias a2a='docker run --rm ghcr.io/codeyogico/a2a-probe'
a2a send "Hello"
```

### Pre-built binaries

Download the binary for your platform from the [Releases page](https://github.com/CodeYogiCo/a2a-probe/releases):

- `a2a-probe-linux-amd64`
- `a2a-probe-darwin-arm64`
- `a2a-probe-windows-amd64.exe`

Make it executable and put it on your `$PATH`:

```bash
chmod +x a2a-probe-darwin-arm64
mv a2a-probe-darwin-arm64 /usr/local/bin/a2a-probe
```

### go install

```bash
go install github.com/codeyogico/a2a-probe@latest
```

### Build from source

See the [Build](#build) section below.

## Features

- Send tasks, stream responses, watch live updates, and chat interactively with any A2A-compatible agent
- Four transport protocols: HTTP, SSE, WebSocket, stdio
- Built-in web UI (`serve`) for browser-based interaction
- Named server config stored at `~/.a2a/config.json`
- Single statically-linked binary; scratch-based Docker image included

## Requirements

- Go 1.23+ (only needed for source/`go install` builds; Homebrew and binary installs need nothing)

## Build

```bash
go build -ldflags="-s -w" -o a2a-probe .
```

Cross-compile for another platform:

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o a2a-probe-linux-amd64 .
```

Run the tests:

```bash
go test ./...
```

## Usage

```
a2a-probe [OPTIONS] COMMAND

Options:
  -s, --server TEXT     Server URL or config alias (default: http://localhost:8000)
  -t, --transport TEXT  Transport: http | sse | ws | stdio (default: http)
      --debug           Enable debug logging
  -q, --quiet           Suppress non-essential output
      --version         Print version and exit
```

### Commands

| Command | Description |
|---------|-------------|
| `send <message>` | Send a task and print the result |
| `send --stream <message>` | Send a task and stream updates |
| `get <task-id>` | Retrieve a task by ID (`--with-history`, `--history N`) |
| `cancel <task-id>` | Cancel a running task |
| `watch <task-id>` | Stream live updates for an existing task |
| `chat` | Interactive chat mode |
| `serve` | Start a local web UI (`--port`, default 7070) |
| `stdio` | JSON-RPC 2.0 over stdin/stdout for CI/CD pipelines |
| `config add <name> <url>` | Save a named server (`--transport`) |
| `config remove <name>` | Remove a named server |
| `config list` | List all configured servers |

### Examples

```bash
# Send a one-shot task
a2a-probe send "Summarise this repo"

# Stream the response
a2a-probe send --stream "Write a haiku"

# Use SSE transport against a named server
a2a-probe -s myagent -t sse send "Hello"

# Save a server alias
a2a-probe config add myagent http://localhost:9000

# Interactive chat
a2a-probe chat

# Browser-based web UI on port 7070
a2a-probe serve
```

## Docker

```bash
docker build -t a2a-probe .
docker run --rm a2a-probe send "Hello"
```

## Status / Known gaps

The Go port is functional but a few things from the original are not yet wired up:

- **`--debug` / `--quiet`** — the flags are accepted but do not yet change logging behaviour.
- **`stdio` command** — currently echoes input line-by-line as a placeholder; the full JSON-RPC 2.0 bridge is not implemented yet.

Contributions welcome.

## License

MIT
