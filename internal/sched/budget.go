// Package sched decides which Droplets to refresh each cycle so Sailor stays
// under DigitalOcean's API rate limit. See docs/adr/0002.
package sched

const (
	// CallsPerMinute is the fixed per-minute metric-call budget (~15% headroom
	// under DO's ~83/min ceiling). Not configurable.
	CallsPerMinute = 70
	// CallsPerDroplet is the metric calls a fully-populated row costs per cycle
	// (CPU + memory_available + filesystem_free).
	CallsPerDroplet = 3
)

// MaxDroplets returns how many active Droplets can be refreshed in one cycle,
// given calls already reserved (e.g. for listing pages).
func MaxDroplets(reserved int) int {
	n := (CallsPerMinute - reserved) / CallsPerDroplet
	if n < 1 {
		return 1
	}
	return n
}

// SelectWindow chooses which row indices [0,n) to refresh this cycle. It walks
// outward from the cursor (cursor, ±1, ±2, …), selecting only indices for which
// active(i) is true, until maxActive are chosen or the list is exhausted.
// Inactive rows are visited but cost nothing, so the window stretches past them
// to fund more active Droplets near the cursor.
func SelectWindow(n int, active func(int) bool, cursor, maxActive int) map[int]bool {
	sel := make(map[int]bool)
	if n <= 0 || maxActive <= 0 {
		return sel
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= n {
		cursor = n - 1
	}
	consider := func(i int) {
		if i >= 0 && i < n && len(sel) < maxActive && active(i) {
			sel[i] = true
		}
	}
	consider(cursor)
	for d := 1; len(sel) < maxActive; d++ {
		left, right := cursor-d, cursor+d
		if left < 0 && right >= n {
			break // fully expanded in both directions
		}
		consider(left)
		consider(right)
	}
	return sel
}
