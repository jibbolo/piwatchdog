# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

PiWatchDog is a Go daemon that runs on a Raspberry Pi, monitors internet connectivity, and power-cycles a router via a GPIO relay when an outage is confirmed. Full specification: `docs/piwatchdog-fsd.md`.

## Stack constraints

- **Go 1.21+** — one permitted external dependency: `gopkg.in/yaml.v3` for config parsing. No other external modules.
- Statically compiled: `CGO_ENABLED=0`.
- `log/slog` for structured JSON logging — log on state transitions and errors only, not on every check.
- ntfy.sh notification fires **only** when internet is restored after an outage, never while internet is down.

## Build & run

```bash
# Local (host arch)
go build -o piwatchdog ./cmd/piwatchdog

# Cross-compile for Pi (32-bit)
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -o piwatchdog ./cmd/piwatchdog

# Cross-compile for Pi (64-bit)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o piwatchdog ./cmd/piwatchdog

# Run with config
./piwatchdog --config config.yaml.example
```

## Test

```bash
go test ./...                        # all tests
go test ./internal/watchdog/...      # single package
go test -run TestBackoffSchedule ./...  # single test
```

No physical hardware required — `RelayController` and `PingFunc` are interfaces/function types injected at construction; tests use mocks.

## Architecture

The binary is a single process with six subsystems wired together in `cmd/piwatchdog/main.go`:

| Subsystem | Role |
|-----------|------|
| `Scheduler` | Drives the polling loop via a `time.Ticker` |
| `ConnectivityChecker` | Calls injectable `PingFunc` against all configured targets |
| `StateMachine` | Owns all state transitions, retry counter, and backoff timing |
| `RelayController` | Interface over sysfs GPIO (`/sys/class/gpio/gpioN/value`); mock for tests |
| `Notifier` | HTTP POST to ntfy.sh; non-blocking; only called on recovery |
| Logger | `log/slog` JSON handler writing to stdout |

**State machine states:** `MONITORING → OUTAGE_DETECTED → RESETTING → RECOVERING → BACKOFF → DEEP_SLEEP → MONITORING`

State is in-memory only — a process restart always begins a fresh `MONITORING` cycle.

## Key interfaces

```go
type RelayController interface {
    Open() error       // cuts router power
    Close() error      // restores router power
    State() RelayState
}

type PingFunc func(target string, timeout time.Duration) bool
```

## Git workflow

`main` is protected — direct pushes are disabled. All changes go through a branch + pull request:

```bash
git checkout -b <type>/<short-description>   # e.g. feat/ntfy-retry, fix/relay-polarity
# ... make changes, go test ./... ...
git push -u origin HEAD
gh pr create --title "..." --body "..."
```

Branch naming: `feat/`, `fix/`, `refactor/`, `docs/` prefixes. PRs must pass `go build ./...` and `go test ./...` before merging. Squash merge into `main`.

## Config file shape

```yaml
check_interval: 60s
evidence_window: 5m
targets: [8.8.8.8, 1.1.1.1, 9.9.9.9]
relay:
  gpio_pin: 17
  off_duration: 5s
  active_low: true
recovery_window: 3m
retry:
  max_count: 5
  base_backoff: 5m
  multiplier: 2.0
deep_sleep_interval: 8h
notifications:
  ntfy_url: https://ntfy.sh
  topic: piwatchdog
  token: ""
log:
  level: info
  format: json
```
