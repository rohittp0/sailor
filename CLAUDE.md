# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Status

**Greenfield.** The repository currently contains only `go.mod` (module `sailor`, Go 1.26) — no source files exist yet. The sections below describe the intended product and the commands that will apply once code lands; update this file as real architecture is built.

## What Sailor Is

Sailor is a tool for managing a user's DigitalOcean account. It launches into a UI that lists all droplets with their CPU, memory, and disk usage. Core features:

- List every droplet with current CPU / memory / disk metrics.
- Search droplets by name.
- Sort the list by any metric.
- Expand a droplet to show a full chart of all its metrics.

Implementation implication: the app is a metrics dashboard over the DigitalOcean API (droplet listing) plus its monitoring/metrics endpoints (per-droplet CPU/memory/disk time series for the expanded charts). Expect a clear seam between (1) a DigitalOcean API client, (2) a metrics/polling layer, and (3) the UI rendering list + charts.

## Commands

Standard Go toolchain (module name: `sailor`):

```bash
go run .                    # run the app
go build ./...              # build everything
go test ./...               # run all tests
go test ./path/to/pkg       # test a single package
go test -run TestName ./... # run a single test by name
go vet ./...                # static checks
gofmt -l -w .               # format (or: go fmt ./...)
```

## Notes

- A DigitalOcean API token is required to talk to the API at runtime. Never commit it; load it from the environment or a local untracked config.
- When adding the first packages, replace this Status/intent section with the actual architecture (package layout, API client design, how metrics are fetched and refreshed, and which UI framework is used — CLI/TUI vs. web).