# PiWatchDog

A lightweight internet watchdog for Raspberry Pi. When your router stops responding, PiWatchDog power-cycles it via a GPIO relay and notifies you when connectivity is restored.

## How it works

1. Pings a set of configurable targets at a regular interval
2. If all targets are unreachable for longer than the evidence window (default 5 min) → confirms outage
3. Toggles a relay to cut and restore router power
4. Waits for recovery, then rechecks
5. Applies exponential backoff across up to N retries; falls back to a long sleep before resuming

A push notification is sent via [ntfy.sh](https://ntfy.sh) only when internet is confirmed restored.

## Requirements

- Raspberry Pi with a GPIO-connected relay module
- Go 1.21+ (for cross-compilation from any host)
- A [ntfy.sh](https://ntfy.sh) topic (or self-hosted instance)

## Install

```bash
# Cross-compile on your workstation
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o piwatchdog ./cmd/piwatchdog

# Deploy
scp piwatchdog pi@<host>:/usr/local/bin/
scp config.yaml.example pi@<host>:/etc/piwatchdog/config.yaml
```

Edit `/etc/piwatchdog/config.yaml` to set your GPIO pin, ping targets, and ntfy.sh topic, then enable the service:

```bash
sudo systemctl enable --now piwatchdog
journalctl -u piwatchdog -f
```

## Configuration

See `config.yaml.example` for all options. Key settings:

| Key | Default | Description |
|-----|---------|-------------|
| `targets` | — | IPs or hostnames to ping (required) |
| `relay.gpio_pin` | — | BCM GPIO pin number (required) |
| `evidence_window` | `5m` | How long all targets must fail before acting |
| `retry.max_count` | `5` | Max resets before deep sleep |
| `deep_sleep_interval` | `8h` | Wait time after max retries exhausted |

## License

MIT — see [LICENSE](LICENSE).
