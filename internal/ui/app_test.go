package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rohittp0/sailor/internal/config"
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

func TestStartSCPRouting(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	hosts, err := config.LoadHosts()
	if err != nil {
		t.Fatal(err)
	}
	base := Model{hosts: hosts, state: viewDetail}

	// No public IP -> transient error, no picker, no modal.
	m := base
	m.detail = newDetailModel(do.Droplet{ID: 1, Name: "x"}, 80, 24)
	res, _ := m.startSCP()
	if rm := res.(Model); rm.scpOpen || rm.sshOpen || rm.scpErr == "" {
		t.Fatalf("no-IP should error without opening anything: picker=%v modal=%v err=%q",
			rm.scpOpen, rm.sshOpen, rm.scpErr)
	}

	// Has IP, no stored profile -> SSH modal opens first, upload pending.
	m = base
	m.detail = newDetailModel(do.Droplet{ID: 2, Name: "y", PublicIP: "5.6.7.8"}, 80, 24)
	res, _ = m.startSCP()
	rm := res.(Model)
	if !rm.sshOpen || !rm.scpPending || rm.scpOpen {
		t.Fatalf("no profile should open modal with scpPending: modal=%v pending=%v picker=%v",
			rm.sshOpen, rm.scpPending, rm.scpOpen)
	}

	// Has IP + stored profile -> picker opens directly.
	if err := hosts.Set(3, config.Profile{User: "deploy"}); err != nil {
		t.Fatal(err)
	}
	m = base
	m.detail = newDetailModel(do.Droplet{ID: 3, Name: "z", PublicIP: "9.9.9.9"}, 80, 24)
	res, _ = m.startSCP()
	if rm := res.(Model); !rm.scpOpen || rm.sshOpen {
		t.Fatalf("stored profile should open the picker directly: picker=%v modal=%v", rm.scpOpen, rm.sshOpen)
	}
}

func TestSCPPendingModalOpensPicker(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	hosts, err := config.LoadHosts()
	if err != nil {
		t.Fatal(err)
	}
	base := Model{hosts: hosts, state: viewDetail}
	base.detail = newDetailModel(do.Droplet{ID: 7, Name: "w", PublicIP: "5.6.7.8"}, 80, 24)

	// Start SCP with no profile -> modal opens pending.
	res, _ := base.startSCP()
	m := res.(Model)

	// Confirm the modal (default user root, empty key is allowed) -> picker opens.
	res, _ = m.handleSSHKey(key(tea.KeyEnter))
	rm := res.(Model)
	if rm.sshOpen || rm.scpPending || !rm.scpOpen {
		t.Fatalf("after modal confirm: modal=%v pending=%v picker=%v, want picker only",
			rm.sshOpen, rm.scpPending, rm.scpOpen)
	}
	if _, ok := hosts.Get(7); !ok {
		t.Fatal("modal confirm should have saved the connection profile")
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
