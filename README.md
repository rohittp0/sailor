# ⛵ Sailor

A fast **terminal dashboard for your DigitalOcean Droplets**. List every Droplet with live CPU / memory / disk usage, search and sort, expand any Droplet into full time-series charts, SSH straight into one, and SCP files up to it — all without leaving the terminal.

![Sailor list view](docs/assets/list.png)

Sailor collapses the round-trip between the DigitalOcean web console (for metrics) and your terminal (for SSH) into a single static binary.

## Features

- **Live metrics list** — CPU, memory and disk usage per Droplet, with colored threshold bars (green → yellow → orange → red).
- **Search & sort** — substring name filter (`/`) and sort by name / CPU / memory / disk; the cursor stays pinned to its Droplet as rows re-sort live.
- **Expandable charts** — drill into any Droplet for stacked CPU/mem/disk braille time-series over a 1h / 6h / 24h window.
- **One-key SSH** — hand off to your system `ssh` for a real shell; the login user and key are remembered per Droplet.
- **SCP upload** — pick local files/folders in a built-in multi-select file browser and send them to the Droplet's home directory, with a live progress bar. Reuses the saved SSH login.
- **Rate-limit aware** — a cursor-windowed refresh scheduler stays under DigitalOcean's API limit on accounts of any size.
- **Instant launch** — the Droplet list is cached for a day, so the UI paints immediately while fresh data loads in the background.
- **Single binary** — no agent, no server, no browser. Read-only: it never mutates your account.

> **Note on metrics:** DigitalOcean always exposes CPU, but **memory and disk usage require the [metrics agent](https://docs.digitalocean.com/products/monitoring/how-to/install-metrics-agent/) (`do-agent`)** on the Droplet. Droplets without it show CPU only (memory/disk as `n/a`); powered-off Droplets show `--`.

## Install

### Download a release

Grab a prebuilt binary from the [latest release](https://github.com/rohittp0/sailor/releases/latest):

```bash
# macOS (Apple Silicon)
curl -L https://github.com/rohittp0/sailor/releases/latest/download/sailor-darwin-arm64.tar.gz | tar xz

# Linux (x86-64)
curl -L https://github.com/rohittp0/sailor/releases/latest/download/sailor-linux-amd64.tar.gz | tar xz

./sailor
```

Binaries are published for macOS and Linux on both `amd64` and `arm64`.

### Go install

With Go 1.26+ on your `PATH`:

```bash
go install github.com/rohittp0/sailor/cmd/sailor@latest
```

This drops the `sailor` binary in `$(go env GOPATH)/bin`.

### Build from source

```bash
git clone https://github.com/rohittp0/sailor.git
cd sailor
go build -o sailor ./cmd/sailor
./sailor
```

## Usage

Sailor needs a DigitalOcean API token **with the Monitoring read scope** (a token without it lists Droplets but returns `403` on metrics). It is resolved in this order:

1. The `DIGITALOCEAN_ACCESS_TOKEN` environment variable.
2. Your existing [`doctl`](https://docs.digitalocean.com/reference/doctl/) config (`~/.config/doctl/config.yaml`, current context).

```bash
export DIGITALOCEAN_ACCESS_TOKEN=dop_v1_xxx
sailor

# …or, if doctl is already authenticated:
sailor
```

### Keybindings

| Key | Action |
| --- | --- |
| `j` / `k`, `↓` / `↑` | Move cursor |
| `g` / `G` | Jump to top / bottom |
| `ctrl+d` / `ctrl+u` | Page down / up |
| `/` | Filter by name (`esc` clears) |
| `N` / `C` / `M` / `D` | Sort by name / CPU / memory / disk |
| `enter` / `e` | Expand to charts |
| `s` / `S` | SSH / edit SSH profile |
| `u` | Upload files/folders (SCP) |
| `r` | Refresh all |
| `1` / `2` / `3` | Chart window 1h / 6h / 24h (in the expanded view) |
| `esc` | Back to the list |
| `?` | Toggle help |
| `q` | Quit |

### Expanded view

Press `enter` on a Droplet to open stacked CPU / memory / disk charts. The time window (`1`/`2`/`3` → 1h/6h/24h) is a global setting that persists across Droplets.

![Sailor expanded chart view](docs/assets/detail.png)

### SSH

Press `s` on a Droplet to connect. The first time, a small prompt asks for the login user (default `root`) and identity-file path; the choice is saved per Droplet in `~/.config/sailor/hosts.toml` (**paths only — no secrets**) and reused on every later connection. Press `S` to change a saved profile. Sailor execs your system `ssh`, so it inherits your keys, agent and `~/.ssh/config`.

### SCP upload

Press `u` on a Droplet to upload local files or folders to its home directory. A terminal file browser opens at your current directory — navigate with `l`/`→` (into a folder), `h`/`←` (up), `space` to check items (across directories), `/` to filter, `.` to toggle hidden files — then `enter` to send everything selected. Selecting a folder *and* something inside it just sends the folder. A styled progress bar shows the transfer live. The upload reuses the Droplet's saved SSH login (prompting for it first if you haven't connected yet). It runs the system `scp` binary non-interactively, so uploads never mutate your DigitalOcean account — they only write files onto a server you already reach over SSH.

## How it stays under the rate limit

DigitalOcean allows ~5,000 API requests/hour and each fully-populated row costs 3 metric calls. Sailor fetches **all** stats once at launch (so the initial sort is correct), then settles into a **cursor-centered window** of ≤ 23 active Droplets per minute; off-screen rows show their last value (dimmed) and refresh as you scroll to them. The expanded view pauses the list and polls just that one Droplet every 5s. See [ADR-0002](docs/adr/0002-rate-limit-budgeted-refresh.md) for the full design.

## Documentation

- A browsable docs site lives in [`docs/`](docs/index.html) (served via GitHub Pages).
- Design decisions are recorded as ADRs in [`docs/adr/`](docs/adr/).
- The domain language is defined in [`CONTEXT.md`](CONTEXT.md).

## Scope

Sailor is **read-only against your DigitalOcean account by design** — it observes and connects, but never powers off, reboots, resizes, or destroys Droplets through the DO API. The SSH and SCP hand-offs act *inside* a server you already control (SCP upload writes files there), not on the control plane. That keeps it safe to run against a production account.
