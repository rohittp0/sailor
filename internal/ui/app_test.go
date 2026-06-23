package ui

import (
	"testing"

	"github.com/rohittp0/sailor/internal/do"
	"github.com/rohittp0/sailor/internal/sched"
)

func TestWindowDropletsCapsAndExcludesOff(t *testing.T) {
	m := Model{list: newListModel()}

	// 50 active + 10 off droplets, all with usage so none are pending.
	var droplets []do.Droplet
	usage := map[int]do.Usage{}
	for i := 1; i <= 50; i++ {
		droplets = append(droplets, do.Droplet{ID: i, Name: "a", Status: "active"})
		usage[i] = active(float64(i)) // distinct CPU so order is deterministic
	}
	for i := 51; i <= 60; i++ {
		droplets = append(droplets, do.Droplet{ID: i, Name: "off", Status: "off"})
		usage[i] = offUsage()
	}
	m.list.setSnapshot(droplets, usage)
	m.list.cursorID = 25

	win := m.windowDroplets()

	// Never exceeds the per-minute budget window.
	if len(win) > sched.MaxDroplets(1) {
		t.Fatalf("window has %d droplets, exceeds budget %d", len(win), sched.MaxDroplets(1))
	}
	if len(win) != 23 {
		t.Fatalf("window = %d, want 23 (full budget with 50 active)", len(win))
	}
	// Only active droplets are ever fetched.
	for _, d := range win {
		if !d.Active() {
			t.Fatalf("window contains non-active droplet %d (%s)", d.ID, d.Status)
		}
	}
}

func TestRefreshDropletsInitialLoadIsFull(t *testing.T) {
	m := Model{list: newListModel()}
	var droplets []do.Droplet
	usage := map[int]do.Usage{}
	for i := 1; i <= 40; i++ {
		droplets = append(droplets, do.Droplet{ID: i, Name: "a", Status: "active"})
		usage[i] = active(float64(i))
	}
	m.list.setSnapshot(droplets, usage)
	m.list.cursorID = 1

	// Before init: every active droplet (so the first sort is correct).
	if got := len(m.refreshDroplets()); got != 40 {
		t.Fatalf("initial load fetched %d, want all 40", got)
	}
	// After init: only the budget window.
	m.initialized = true
	if got := len(m.refreshDroplets()); got != 23 {
		t.Fatalf("steady-state fetched %d, want windowed 23", got)
	}
}

func TestMetricColumnsReflow(t *testing.T) {
	cases := []struct {
		w, want int
	}{{120, 3}, {80, 3}, {79, 2}, {58, 2}, {57, 1}, {40, 1}}
	for _, c := range cases {
		if got := metricColumns(c.w); got != c.want {
			t.Fatalf("metricColumns(%d) = %d, want %d", c.w, got, c.want)
		}
	}
}

func TestHelpOverlayRenders(t *testing.T) {
	out := helpView(120, 40)
	if !contains(out, "keys") || !contains(out, "expand to charts") || !contains(out, "ssh") {
		t.Fatal("help overlay missing expected content")
	}
}

func TestEmptyAccountState(t *testing.T) {
	m := Model{list: newListModel(), initialized: true, width: 80, height: 24}
	if !contains(m.View(), "No droplets") {
		t.Fatal("expected empty-account state once initialized with zero droplets")
	}
}

func TestWindowDropletsSmallAccount(t *testing.T) {
	m := Model{list: newListModel()}
	droplets := []do.Droplet{
		{ID: 1, Name: "a", Status: "active"},
		{ID: 2, Name: "b", Status: "off"},
		{ID: 3, Name: "c", Status: "active"},
	}
	m.list.setSnapshot(droplets, map[int]do.Usage{1: active(10), 2: offUsage(), 3: active(30)})
	m.list.cursorID = 1

	win := m.windowDroplets()
	if len(win) != 2 { // both active droplets, the off one excluded
		t.Fatalf("small account window = %d, want 2 active", len(win))
	}
}
