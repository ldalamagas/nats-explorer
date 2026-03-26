# nats-explorer

An interactive terminal UI for exploring NATS server contents — like k9s, but for NATS.

## Features

- **KV Store** — browse buckets, inspect keys, view values and metadata
- **Object Store** — browse buckets, list objects, view object metadata
- **Subjects** — live subscribe to any Core NATS subject (wildcards `*` and `>` supported)

## Installation

```bash
git clone https://github.com/ldalamagas/nats-explorer
cd nats-explorer
go build -o bin/nats-explorer .
```

## Usage

```bash
nats-explorer [flags]

Flags:
  --url       NATS server URL (default: $NATS_URL or nats://localhost:4222)
  --creds     Path to .creds file
  --nkey      Path to NKey seed file
  --user      Username for authentication
  --password  Password for authentication
```

## Keybindings

| Key | Action |
| --- | --- |
| `tab` / `shift+tab` | Switch tabs |
| `1`–`3` | Jump to tab |
| `↑` / `↓` / `j` / `k` | Navigate list |
| `enter` | Drill down |
| `esc` / `backspace` | Go back |
| `r` | Refresh |
| `/` | Filter |
| `q` / `ctrl+c` | Quit |

## Development

```bash
# Start NATS with JetStream
docker compose up -d

# Seed with test data
go run ./dev/seed

# Run
go run . --url nats://localhost:4222
```

## License

Apache 2.0
