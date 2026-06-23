# AGENTS.md

Guidance for AI coding agents (and humans) working in this repository.

## What this is

`a2a-probe` is a command-line client for the [A2A (Agent-to-Agent) Protocol](https://a2a-protocol.org/latest/specification/), written in Go. It sends messages/tasks to A2A agents, streams responses, and ships a small embedded web UI. It compiles to a single statically-linked binary with no runtime dependencies.

- **Module:** `github.com/codeyogico/a2a-probe`
- **Go version:** 1.23 (see `go.mod`)
- **CLI framework:** [cobra](https://github.com/spf13/cobra)
- The project was ported from an earlier Kotlin/GraalVM implementation; there is no JVM/Gradle anymore.

## Layout

```
main.go                  Entry point — all cobra commands & flag wiring
integration_test.go      End-to-end tests: builds the binary, runs it vs a fake A2A server
internal/
  client/                High-level, transport-agnostic A2AClient + part/text helpers
  transport/             Transport interface + http, sse, websocket, stdio implementations
  model/                 A2A protocol types (Message, Task, Artifact, configs, …)
  ui/                    Terminal rendering + ANSI colour helpers
  chat/                  Interactive REPL (`chat` command)
  server/                Web UI backend for the `serve` command (embeds web/)
web/index.html           Single-file web UI, embedded via go:embed at build time
.github/workflows/       publish.yml — build/test, release, Docker, Homebrew
Dockerfile               Multi-stage build → scratch image
```

## Build, test, run

```bash
go build -o a2a-probe .          # build
go test ./...                    # all tests (incl. the e2e integration tests)
go vet ./...                     # vet
./a2a-probe send "hello"         # run

# cross-compile
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o a2a-probe-linux-amd64 .
```

Always run `go build ./...`, `go vet ./...`, and `go test ./...` before proposing changes.

## Architecture notes

- **`transport.Transport`** is the seam: `Call(method, params) -> result`, `Stream() -> <-chan json.RawMessage`, `Close()`. `client.BuildClient(server, name)` picks the implementation from `-t` (`http` | `sse` | `ws` | `stdio`).
- **`client.A2AClient`** wraps a transport and exposes typed A2A operations (`SendMessage`, `StreamMessage`, `GetTask`, `CancelTask`, `Resubscribe`, plus legacy `SendTask`/`SendSubscribe`).
- **JSON-RPC plumbing** (envelope, error extraction, `parseResponse`) lives in `internal/transport/transport.go`.
- **Rendering** is centralised in `internal/ui` (`PrintTask`, `PrintMessage`, `PrintStreamStatus`, `PrintStreamArtifact`, colour helpers `Bold`/`Dim`/`Green`/…). Prefer reusing these over ad-hoc `fmt.Print`.
- **Message parts** are kept as `[]json.RawMessage` and decoded lazily; use `ui.ExtractText` / `client.PartText`. Parts are discriminated by a `kind` field (`text`/`file`/`data`), with a legacy `type` fallback.
- **Web UI:** `internal/server` serves the embedded `web/` FS and exposes `/api/agent-card`, `/api/send`, `/api/stream`, `/api/task`, `/api/cancel`. The front-end is plain vanilla JS in `web/index.html`.
- **Config** lives at `~/.a2a/config.json` (`model.CliConfig`), managed by `internal/config`.

## Conventions

- Match the surrounding Go style; keep comments at the existing density (short, purposeful).
- Keep the binary dependency-light. Don't add heavy deps (e.g. gRPC/protobuf) without discussing the binary-size trade-off first.
- New A2A operations go on `A2AClient`; new rendering goes in `internal/ui`; don't render directly from `main.go` beyond wiring.
- Add tests with new behaviour. Unit tests live beside the code; cross-command behaviour goes in `integration_test.go`.

## Gotchas (learned the hard way)

- **`message/send` returns a Message OR a Task.** Agents that answer with a Task carry the real content in `artifacts`. `SendMessage` discriminates on `kind` (with a structural fallback) and returns a `SendResult` with exactly one of `Message`/`Task` set. Render Tasks via `ui.PrintTask`, not `PrintMessage`, or output comes out blank.
- **The web UI is embedded at build time** (`go:embed web`). Editing `web/index.html` has no effect until you rebuild. When mutating DOM, don't overwrite a parent's `innerHTML` and then reference a child you just destroyed.
- **`stdio` command vs `stdio` transport** are different: the `-t stdio` transport works; the top-level `stdio` *command* is currently a placeholder that echoes input.
- **`--debug`** is wired: it calls `debug.Enable()` in the root `PersistentPreRun`, and the transport/client/server layers call `debug.Logf` to trace the wire. Logs go to **stderr** so stdout stays pipeable; the spinner becomes a no-op under debug. **`--quiet`** is still unwired.
- **Stream events may be wrapped** in a JSON-RPC `result` envelope, and a "stream" may be a single final `Task`. `coerceStreamEvent` unwraps `result` and detects `Task` snapshots; `printEvent`/`handleEvent` must handle `ev.Task`. Don't consume/discard the first SSE event in the transport — emit them all.

## Release process

Releases are cut by pushing a `vX.Y.Z` tag, which triggers `.github/workflows/publish.yml`:

1. Bump `const version` in `main.go` to match the tag.
2. Merge to `main`, then `git tag -a vX.Y.Z && git push origin vX.Y.Z`.
3. The workflow builds binaries (linux/amd64, darwin/arm64, windows/amd64), pushes a Docker image to GHCR, creates a GitHub Release, and updates the Homebrew formula.
4. **Homebrew caveat:** the `update-homebrew-tap` job is gated on a `HOMEBREW_TAP_TOKEN` secret. While that secret is unset it silently skips, so the formula in `CodeYogiCo/homebrew-tap` (`Formula/a2a-probe.rb`, class `A2aProbe`) must be updated manually — bump `version` and both `sha256` values to the new release binaries.

## What's supported / planned

See the **A2A protocol support** and **Roadmap / planned** sections of [README.md](README.md) for the current capability matrix and gaps.
