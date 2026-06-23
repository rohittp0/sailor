package ui

import (
	"testing"
	"time"

	"github.com/rohittp0/sailor/internal/do"
)

func active(cpu float64) do.Usage {
	return do.Usage{CPU: cpu, CPUValid: true, State: do.StateActive, UpdatedAt: time.Now()}
}

func offUsage() do.Usage { return do.Usage{State: do.StateOff} }

func ids(rows []Row) []int {
	out := make([]int, len(rows))
	for i, r := range rows {
		out[i] = r.D.ID
	}
	return out
}

func eqInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSortCPUDescNAlast(t *testing.T) {
	m := newListModel() // default sortCPU desc
	droplets := []do.Droplet{
		{ID: 1, Name: "a", Status: "active"},
		{ID: 2, Name: "b", Status: "active"},
		{ID: 3, Name: "c", Status: "off"},
	}
	usage := map[int]do.Usage{1: active(10), 2: active(90), 3: offUsage()}
	m.setSnapshot(droplets, usage)

	// 90, 10, then off (invalid) last.
	if got := ids(m.visible()); !eqInts(got, []int{2, 1, 3}) {
		t.Fatalf("desc order = %v, want [2 1 3]", got)
	}

	// Ascending still sinks the off Droplet to the bottom.
	m.toggleSort(sortCPU) // now asc
	if got := ids(m.visible()); !eqInts(got, []int{1, 2, 3}) {
		t.Fatalf("asc order = %v, want [1 2 3] (off last)", got)
	}
}

func TestCursorPinnedAcrossResort(t *testing.T) {
	m := newListModel()
	droplets := []do.Droplet{
		{ID: 1, Name: "a", Status: "active"},
		{ID: 2, Name: "b", Status: "active"},
	}
	m.setSnapshot(droplets, map[int]do.Usage{1: active(10), 2: active(90)})
	// Select the low-CPU Droplet (id 1), currently at the bottom.
	m.cursorID = 1
	sel, _ := m.selected()
	if sel.D.ID != 1 {
		t.Fatalf("selected = %d, want 1", sel.D.ID)
	}

	// Refresh flips the values so id 1 now sorts to the top.
	m.setSnapshot(droplets, map[int]do.Usage{1: active(99), 2: active(5)})
	if got := ids(m.visible()); !eqInts(got, []int{1, 2}) {
		t.Fatalf("order after resort = %v, want [1 2]", got)
	}
	if sel, _ := m.selected(); sel.D.ID != 1 {
		t.Fatalf("cursor jumped to %d, want it pinned to 1", sel.D.ID)
	}
}

func TestFilterAndMove(t *testing.T) {
	m := newListModel()
	droplets := []do.Droplet{
		{ID: 1, Name: "web-1", Status: "active"},
		{ID: 2, Name: "db-1", Status: "active"},
		{ID: 3, Name: "web-2", Status: "active"},
	}
	m.setSnapshot(droplets, map[int]do.Usage{1: active(10), 2: active(20), 3: active(30)})
	m.filter = "web"
	if got := len(m.visible()); got != 2 {
		t.Fatalf("filtered count = %d, want 2", got)
	}
	// Cursor lands on first visible; move down stays within filtered set.
	m.cursorID = m.visible()[0].D.ID
	m.move(5) // clamps to last visible
	if sel, _ := m.selected(); sel.D.Name[:3] != "web" {
		t.Fatalf("cursor left filtered set: %s", sel.D.Name)
	}
}

func TestViewRendersAtVariousSizes(t *testing.T) {
	m := newListModel()
	droplets := []do.Droplet{
		{ID: 1, Name: "nyc1-web-01", Status: "active"},
		{ID: 2, Name: "db-replica", Status: "active"},
		{ID: 3, Name: "old-box", Status: "off"},
	}
	usage := map[int]do.Usage{
		1: {CPU: 87, Mem: 72, Disk: 45, CPUValid: true, MemValid: true, DiskValid: true, State: do.StateActive, UpdatedAt: time.Now()},
		2: {CPU: 12, CPUValid: true, State: do.StateNoAgent, UpdatedAt: time.Now()}, // no agent: mem/disk n/a
		3: offUsage(),
	}
	m.setSnapshot(droplets, usage)

	for _, size := range [][2]int{{120, 30}, {80, 24}, {40, 10}} {
		m.width, m.height = size[0], size[1]
		out := m.view(time.Now()) // must not panic
		if out == "" || !contains(out, "SAILOR") {
			t.Fatalf("view at %v missing header / empty", size)
		}
		// Full name shows where there's room; narrow widths truncate (reflow is M6).
		if size[0] >= 80 && !contains(out, "nyc1-web-01") {
			t.Fatalf("view at %v should show full name", size)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestProgressivePageThenUsage(t *testing.T) {
	m := newListModel()
	m.width, m.height = 120, 30
	m.beginRefresh()

	// First page arrives: rows are present but pending (loading).
	m.addPage([]do.Droplet{
		{ID: 1, Name: "web", Status: "active"},
		{ID: 2, Name: "db", Status: "active"},
	})
	if m.count() != 2 {
		t.Fatalf("count = %d, want 2", m.count())
	}
	if r := m.data[1]; !r.Pending || r.HasUsage {
		t.Fatalf("row 1 should be pending pre-usage: %+v", r)
	}
	if !contains(m.view(time.Now()), "loading") {
		t.Fatal("pending rows should render a loading placeholder")
	}

	// Stats arrive for one droplet.
	m.applyUsage(1, active(55))
	if r := m.data[1]; r.Pending || !r.HasUsage || r.U.CPU != 55 {
		t.Fatalf("row 1 should have usage applied: %+v", r)
	}
}

func TestRefreshKeepsUsageAndPrunes(t *testing.T) {
	m := newListModel()
	m.setSnapshot([]do.Droplet{
		{ID: 1, Name: "a", Status: "active"},
		{ID: 2, Name: "b", Status: "active"},
	}, map[int]do.Usage{1: active(40), 2: active(60)})

	// A new refresh cycle re-lists only droplet 1.
	m.beginRefresh()
	m.addPage([]do.Droplet{{ID: 1, Name: "a", Status: "active"}})
	// Existing usage is retained (shown stale until refreshed), not reset to loading.
	if r := m.data[1]; r.Pending || !r.HasUsage || r.U.CPU != 40 {
		t.Fatalf("row 1 lost its last-known usage on re-list: %+v", r)
	}
	m.endRefresh()
	if _, ok := m.data[2]; ok {
		t.Fatal("droplet 2 should have been pruned (no longer listed)")
	}
	if m.count() != 1 {
		t.Fatalf("count = %d, want 1 after prune", m.count())
	}
}

func TestStaleDetection(t *testing.T) {
	now := time.Now()
	fresh := Row{HasUsage: true, U: do.Usage{UpdatedAt: now.Add(-30 * time.Second)}}
	old := Row{HasUsage: true, U: do.Usage{UpdatedAt: now.Add(-5 * time.Minute)}}
	if fresh.stale(now) {
		t.Fatal("30s-old row should not be stale")
	}
	if !old.stale(now) {
		t.Fatal("5m-old row should be stale")
	}
}
