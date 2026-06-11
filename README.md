# a2a-probe

A command-line client for the [A2A (Agent-to-Agent) Protocol](https://google.github.io/A2A/) v0.3.0, written in Kotlin.

## Installation

### Homebrew (macOS / Linux — no JVM required)

```bash
brew tap CodeYogiCo/tap
brew install a2a-probe
```

This installs a native binary compiled with GraalVM — no Java needed.

### Docker (recommended for containers)

```bash
docker pull ghcr.io/codeyogico/a2a-probe:latest
docker run --rm ghcr.io/codeyogico/a2a-probe send "Hello"
```

For convenience, add a shell alias:

```bash
alias a2a='docker run --rm ghcr.io/codeyogico/a2a-probe'
a2a send "Hello"
```

### Pre-built JAR

Download the latest `a2a-probe.jar` from the [Releases page](https://github.com/CodeYogiCo/a2a-probe/releases) and run it with:

```bash
java -jar a2a-probe.jar [OPTIONS] COMMAND
```

Requires JDK 21+.

### Build from source

See the [Build](#build) section below.

## Features

- Send tasks, stream responses, watch live updates, and chat interactively with any A2A-compatible agent
- Four transport protocols: HTTP, SSE, WebSocket, stdio
- Named server config stored at `~/.a2a/config.json`
- Rich terminal output via [Mordant](https://github.com/ajalt/mordant)
- Single fat JAR via Gradle Shadow, Docker image included

## Requirements

- JDK 21+ (only needed for JAR/source builds; Homebrew install requires no JVM)
- Gradle (or use the included `./gradlew` wrapper)

## Build

```bash
./gradlew shadowJar
```

Produces `build/libs/a2a-probe.jar`.

### Native binary (GraalVM)

```bash
./gradlew nativeCompile
```

Produces `build/native/nativeCompile/a2a-probe` — a standalone binary with no JVM dependency.
Requires GraalVM 21 on `$PATH` (e.g. `sdk install java 21.0.7-graalce` via SDKMAN).

## Usage

```
java -jar a2a-probe.jar [OPTIONS] COMMAND

Options:
  -s, --server TEXT     Server URL or config alias (default: http://localhost:8000)
  -t, --transport TEXT  Transport: http | sse | ws | stdio (default: http)
  --debug               Enable debug logging
  -q, --quiet           Suppress non-essential output
```

### Commands

| Command | Description |
|---------|-------------|
| `send <message>` | Send a task and print the result |
| `send --stream <message>` | Send a task and stream updates |
| `get <task-id>` | Retrieve a task by ID |
| `cancel <task-id>` | Cancel a running task |
| `watch <task-id>` | Stream live updates for an existing task |
| `chat` | Interactive chat mode |
| `stdio` | JSON-RPC 2.0 over stdin/stdout for CI/CD pipelines |
| `config add <name> <url>` | Save a named server |
| `config remove <name>` | Remove a named server |
| `config list` | List all configured servers |

### Examples

```bash
# Send a one-shot task
java -jar a2a-probe.jar send "Summarise this repo"

# Stream the response
java -jar a2a-probe.jar send --stream "Write a haiku"

# Use SSE transport against a named server
java -jar a2a-probe.jar -s myagent -t sse send "Hello"

# Save a server alias
java -jar a2a-probe.jar config add myagent http://localhost:9000

# Interactive chat
java -jar a2a-probe.jar chat
```

## Docker

```bash
docker build -t a2a-probe .
docker run --rm a2a-probe send "Hello"
```

## License

MIT
