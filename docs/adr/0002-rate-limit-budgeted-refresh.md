# Rate-limit-budgeted, cursor-windowed refresh

DigitalOcean's API allows ~5,000 requests/hour (~83/min) and a fully-populated list row costs 3 metric calls per poll (CPU + `memory_available` + `filesystem_free`; memory/disk totals come free from the Droplet list, ADR-0001). We therefore do **not** refresh every Droplet on every tick. Instead a fixed per-minute call budget is spent on the Droplets nearest the cursor.

## The model

- **Fixed budget: 70 calls/min** (not configurable — leaves ~15% headroom under the 83/min ceiling).
- **Funded in priority order:** (1) the Droplet-list refresh (~1 call/min per 200 Droplets); (2) each expanded Droplet, which polls every 15s = 12 calls/min; (3) the remaining budget funds list rows at 3 calls/min each.
- **Cursor-windowed:** the list budget is spent walking outward from the cursor (`n/2` above, `n/2` below). **Off/non-active Droplets cost nothing** (their metric calls are skipped), so the window stretches past dormant Droplets to fund more active ones.
- **Stale rows** outside the funded window show their last-known values **dimmed**, and refresh as the cursor moves toward them — they never blank.
- **Live re-sort:** after each refresh the list re-sorts by the active column using each row's latest-known value. The **cursor is pinned to a Droplet ID, not a row index**, so the selection follows its Droplet as rows reorder; the refresh window recomputes around the Droplet's new position.
- **Manual refresh (`r`):** pauses automatic refresh and runs a full sweep of all active Droplets, throttled to stay under budget — so on large accounts it drains over several minutes with a progress indicator, then automatic refresh resumes.

## Why record this

A reader will reasonably expect a monitoring dashboard to keep every row live, and will wonder why distant rows are stale or why the list reorders under the cursor. Both are deliberate consequences of the hard API rate limit, not bugs.
