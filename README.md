<img src="web/logo.svg" alt="a2a-probe" height="44">

A command-line client for the [A2A (Agent-to-Agent) Protocol](https://google.github.io/A2A/) v0.3.0, written in Go.

> Ported from the original Kotlin/GraalVM implementation. The Go build produces a small, statically-linked binary with no runtime dependencies.

## Installation

### Homebrew (macOS / Linux)

```bash
brew tap CodeYogiCo/tap
brew install a2a-probe
```

This also installs a short **`a2a`** alias, so you can type `a2a send "Hello"` instead of the full `a2a-probe`.

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

## Architecture

`a2a-probe` is layered so the protocol logic is independent of how bytes move
and how output is rendered.

```
                         ┌───────────────────────────────────────────────┐
   you ──▶ CLI command   │                main.go (cobra)                 │
                         │  send · get · cancel · watch · chat · stdio    │
                         │                    · serve                     │
                         └──────┬─────────────────────────────────┬──────-┘
                                │                                 │
                        CLI commands                         serve (web UI)
                                │                                 │
                                ▼                                 ▼
                   internal/client.A2AClient          internal/server (HTTP)
                   typed A2A operations:              embeds web/index.html
                   SendMessage / StreamMessage        /api/{agent-card,send,
                   GetTask / CancelTask / …           stream,task,cancel}
                                │   ▲                             │
                                │   │ result (Message│Task│stream)│
                                ▼   │                             │
                   internal/transport.Transport ◀────────────────┘
                   http · sse · ws · stdio
                   (JSON-RPC 2.0 envelope, SSE read incrementally)
                                │   ▲
                                ▼   │
                         ╔══════════╧═══════╗
                         ║   A2A agent      ║  (remote)
                         ╚══════════════════╝

   rendering ◀── internal/ui  (PrintTask / PrintMessage / stream status &
                                artifacts, pretty-printed JSON, spinner)
```

**Request lifecycle (e.g. `send "hi"`):**

1. **`main.go`** parses flags, resolves the server (URL or `~/.a2a/config.json`
   alias via `internal/config`), and builds a client with the chosen transport.
2. **`A2AClient`** turns the request into an A2A operation and calls the
   transport. For `message/send` it discriminates the result into a **Message**
   or a **Task** (`SendResult`); for streaming it returns a channel of events.
3. **`Transport`** wraps the call in a JSON-RPC 2.0 envelope and sends it over
   the wire. SSE responses are read line-by-line and surfaced incrementally.
4. **`internal/ui`** renders the result — message text, task status + artifacts
   (with pretty-printed JSON), or a live stream of status/artifact events. A
   TTY spinner shows progress while waiting.

The `serve` command reuses the same `A2AClient`/`Transport` stack behind a small
HTTP API that drives the embedded single-page web UI.

See [AGENTS.md](AGENTS.md) for a deeper development guide.

## A2A protocol support

What the client implements today, mapped to the [A2A specification](https://a2a-protocol.org/latest/specification/).

### Transports (protocol bindings)

| Binding | Spec | Supported |
|---------|:----:|:---------:|
| JSON-RPC 2.0 over HTTP(S) | ✅ standard | ✅ `-t http` |
| SSE streaming (over the JSON-RPC binding) | ✅ | ✅ `-t sse` |
| gRPC | ✅ standard | ❌ planned |
| HTTP+JSON / REST | ✅ standard | ❌ planned |
| WebSocket | ⚠️ non-standard | ✅ `-t ws` (extra) |
| stdio | ⚠️ non-standard | ✅ `-t stdio` (transport only) |

### Methods / interaction patterns

| Operation | RPC method | Supported |
|-----------|------------|:---------:|
| Send message (sync) | `message/send` | ✅ `send` |
| Send streaming message | `message/stream` | ✅ `send --stream` |
| Get task | `tasks/get` | ✅ `get` |
| Cancel task | `tasks/cancel` | ✅ `cancel` |
| Resubscribe to a task stream | `tasks/resubscribe` | ✅ `watch` |
| List tasks | `tasks/list` | ❌ planned |
| Push notification config (set/get/list/delete) | `tasks/pushNotificationConfig/*` | ❌ planned |
| Authenticated extended agent card | — | ❌ planned |
| Legacy send / sendSubscribe | `tasks/send`, `tasks/sendSubscribe` | ✅ automatic fallback |

### Content & discovery

- **Message/artifact parts:** `text`, `file` (bytes or URI), `data` (structured JSON, pretty-printed) — all rendered.
- **Streaming events:** full `Task` snapshots, `TaskStatusUpdateEvent`, `TaskArtifactUpdateEvent`, and plain messages — including payloads wrapped in a JSON-RPC `result` envelope.
- **Agent card discovery:** fetched from `/.well-known/agent.json`, falling back to `/.well-known/agent-card.json`. The `serve` web UI has a **Discover** page to inspect any agent's card (capabilities, skills, security, raw JSON).
- **Multi-turn:** supported implicitly via the server's task/context continuation; no dedicated CLI affordance yet.

### Debugging

Pass `--debug` on any command to trace the full request/response flow on stderr —
the JSON-RPC envelope sent, response status and content type, and every SSE event
as it arrives (with timestamps). stdout stays clean, so you can still pipe results:

```bash
a2a-probe --debug send --stream "hello" 2>debug.log
```

Debug tracing also covers the `serve` web UI's `/api/*` calls. In the web UI,
the **⚙ debug** toggle turns tracing on at runtime and streams the live log into
an on-page panel — no terminal needed.

## Roadmap / planned

Not yet implemented, roughly in priority order:

- **Push notifications** — register a webhook (`tasks/pushNotificationConfig/set`) plus a built-in receiver command to observe agent callbacks end-to-end.
- **`--quiet`** — flag is accepted but does not yet suppress output. (`--debug` is implemented — see [Debugging](#debugging).)
- **`stdio` command** — currently echoes input line-by-line as a placeholder; the full JSON-RPC 2.0 bridge is not implemented.
- **`tasks/list`** and **authenticated extended card** retrieval.
- **gRPC / REST** transport bindings.

Contributions welcome — see [AGENTS.md](AGENTS.md) for architecture and development guidance.

## License

MIT
