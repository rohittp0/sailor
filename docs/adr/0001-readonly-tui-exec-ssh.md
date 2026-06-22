# Read-only terminal UI, exec system SSH — chosen over a web app

Sailor is a **read-only TUI** (Charm Bubble Tea + ntcharts braille charts) that ships as a single Go binary, and connects to a Droplet by **suspending the TUI and exec'ing the system `ssh` binary** (`ssh -i <key> <user>@<public-ipv4>`, inheriting the user's keys, agent, and `~/.ssh/config`). It does not mutate account or Droplet state (no power/reboot/resize/destroy).

## Why, and what we rejected

A browser UI (Chart.js/ECharts + xterm.js) would give nicer charts, but **SSH-on-select is the deciding requirement**: in a terminal it's a native ~20-line exec hand-off; in a browser it requires standing up a PTY-over-WebSocket proxy that brokers SSH credentials — a real security surface for a personal admin tool. Terminal charts via ntcharts are good enough (btop-class), and Kitty/Sixel image charts remain an optional later upgrade. The single-binary distribution also fits a personal tool well. See the research in the planning session.

## Consequences

- **Read-only is a deliberate boundary**, not an oversight. Lifecycle actions are out of scope for v1 because destructive operations need their own confirmation-UX and blast-radius design.
- The metrics-fetch design is **independent of this decision** — the DigitalOcean Monitoring API calls would be identical under a web app. One constraint it imposes regardless: against DO's ~5,000 req/hour (~83/min) limit, a list row costs **3 metric calls per poll** (CPU + `memory_available` + `filesystem_free`; the memory/disk *totals* come free from the Droplet list). That limit forces a budgeted, cursor-windowed refresh rather than refreshing every Droplet — see [ADR-0002](./0002-rate-limit-budgeted-refresh.md).
