# nats-explorer — Agent Instructions

## Project overview
A Go TUI application for exploring NATS server contents interactively.
It is modeled after k9s (for Kubernetes) but targets NATS.

## Stack
- **Language**: Go (module: `github.com/ldalamagas/nats-explorer`)
- **TUI**: [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Bubbles](https://github.com/charmbracelet/bubbles) + [Lipgloss](https://github.com/charmbracelet/lipgloss)
- **NATS client**: `github.com/nats-io/nats.go` (JetStream new API via `nats.go/jetstream`)

## Directory layout
```
main.go                         — entry point, CLI flags
internal/
  nats/client.go                — NATS connection wrapper + all data-access helpers
  tui/
    app.go                      — root Bubble Tea model, tab routing, layout
    styles.go                   — all Lipgloss style vars
    views/
      kv.go                     — KV Store tab (buckets → keys → value)
      objects.go                — Object Store tab (buckets → objects → detail)
      subjects.go               — Core NATS tab (live subscribe, real-time message feed)
```

## Supported features

- **KV Store**: list buckets, browse keys, view value + metadata
- **Object Store**: list buckets, list objects, view object metadata
- **Subjects** (Core NATS): live subscribe to any subject (wildcards `*` and `>` supported)

## Key UX conventions

- `tab` / `shift+tab` — switch between tabs
- `1`–`3` — jump directly to a tab
- `↑↓` / `j`/`k` — navigate lists
- `enter` — drill down (bucket → keys, etc.)
- `esc` / `backspace` — go back one level or unsubscribes if listening to a subject
- `r` — refresh current tab data
- `/` — filter (built-in bubbles list filter)
- `q` / `ctrl+c` — quit

## CLI flags
```
--url       NATS server URL (default: $NATS_URL or nats://localhost:4222)
--creds     path to .creds file
--nkey      path to NKey seed file
--user      username for authentication
--password  password for authentication
```

## NATS API notes (nats.go v1.50+)
- Use `nats.go/jetstream` new API (not legacy `nats.JetStream()`)
- `KeyValueNamesLister` → `.Name() <-chan string`, `.Error() error`  (NOT `.Err()`)
- `ObjectStoreNamesLister` → `.Name() <-chan string`, `.Error() error`  (NOT `.Err()`)
- `KeyLister` → `.Keys() <-chan string`, `.Stop() error`  (no `.Err()`)
- `ObjectStoreStatus` has no `.Created()` method
- `ObjectInfo.ModTime` (not `.Modified`)
- `nats.NkeyOptionFromSeed(path)` returns `(nats.Option, error)` — handle both

## Architecture patterns

- Each view (kv, objects, subjects) is a self-contained struct with `Init()`, `Update()`, `View()`, and `SetSize()` methods — compatible with Bubble Tea's model pattern.
- Data loading is done via `tea.Cmd` (async), returning typed msg structs.
- The root `App` in `tui/app.go` routes keyboard events and size changes to the active view, and data-loading messages to all views.
- All NATS data access is in `internal/nats/client.go` — views never import `nats.go` directly.

## Build
```
go build -o bin/nats-explorer .   # produces ./bin/nats-explorer
go build ./...                    # build all packages (no output)
```

## Dev / testing setup

```bash
# 1. Start NATS with JetStream
docker compose up -d

# 2. Seed with test data (KV, object store)
go run ./dev/seed

# 3. Run the explorer
go run . --url nats://localhost:4222
```

Seed data created:

- **KV buckets**: `config` (10 keys), `sessions` (8 keys, 2h TTL), `feature-flags` (7 keys)
- **Object buckets**: `assets` (openapi.json, config yaml, email template, SVG logo), `backups` (3 fake snapshots)
