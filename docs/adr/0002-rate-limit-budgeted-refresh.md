# Rate-limit-budgeted, cursor-windowed refresh

DigitalOcean's API allows ~5,000 requests/hour (~83/min) and a fully-populated list row costs 3 metric calls per poll (CPU + `memory_available` + `filesystem_free`; memory/disk totals come free from the Droplet list, ADR-0001). We therefore do **not** refresh every Droplet on every tick. Instead a fixed per-minute call budget is spent on the Droplets nearest the cursor.

## Two mutually-exclusive modes

The app is either showing the **list** or a **single expanded Droplet** — never both, and only one Droplet expands at a time. While expanded, the list is hidden and its background refresh is **paused** (the in-flight refresh is cancelled on expand and resumed on return). This makes budgeting a single-mode problem:

- **Detail mode:** only the one expanded Droplet polls — 3 calls every 15s = 12/min, trivially under budget. No budgeting needed.
- **List mode:** the budget applies here, and only here.

## The list-mode model

- **Initial load is full, not windowed.** The very first stats pass fetches *every* active Droplet so the opening sort (default CPU desc) is correct across the whole account — windowing only the first screen would sort against mostly-empty rows. The periodic windowed refresh does not begin until this full load completes. `r` resets to this full-load behavior ("refresh everything"). After the full load, every row has data, so the list stays correctly sorted even as off-window rows go stale.
- **Re-window on sort/filter.** Changing the sort column or applying/clearing the filter re-fetches the new cursor window immediately (stats-only, no re-list), so the now-relevant rows refresh without waiting for the next cycle.
- **Fixed budget: 70 calls/min** (not configurable — leaves ~15% headroom under the 83/min ceiling).
- **Funded in priority order:** (1) the Droplet-list refresh (~1 call/min per 200 Droplets); (2) the remaining budget funds list rows at 3 calls/min each. (There is no expanded-Droplet term — detail mode pauses the list.)
- **Cursor-windowed:** the list budget is spent walking outward from the cursor (`n/2` above, `n/2` below). **Off/non-active Droplets cost nothing** (their metric calls are skipped), so the window stretches past dormant Droplets to fund more active ones.
- **Stale rows** outside the funded window show their last-known values **dimmed**; rows never fetched yet show "loading". Moving the cursor onto such a row triggers an **on-demand fetch of just that Droplet** (3 calls) — so scrolling reveals fresh data without waiting for the next cycle.
- **Live re-sort:** after each refresh the list re-sorts by the active column using each row's latest-known value. The **cursor is pinned to a Droplet ID, not a row index**, so the selection follows its Droplet as rows reorder; the refresh window recomputes around the Droplet's new position.
- **Manual refresh (`r`):** re-lists Droplets and immediately re-fetches the current cursor window. Because the model only ever refreshes a windowful, a multi-minute "sweep everything" pass is unnecessary — `r` is just "refresh now". (Simplified from the original full-sweep design once single-expand + list-pause made budgeting single-mode.)
- **Backstop:** the client tracks `resp.Rate` headers; windowing keeps us well under the ceiling, so this is a safety net rather than the primary control.

## Why record this

A reader will reasonably expect a monitoring dashboard to keep every row live, and will wonder why distant rows are stale or why the list reorders under the cursor. Both are deliberate consequences of the hard API rate limit, not bugs.
