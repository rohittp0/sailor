package ui

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/digitalocean/godo/metrics"
	"github.com/rohittp0/sailor/internal/config"
	"github.com/rohittp0/sailor/internal/do"
	"github.com/rohittp0/sailor/internal/sched"
)

// refreshInterval is the list poll cadence (M2: naive, all active Droplets).
const refreshInterval = time.Minute

// metricLookback is the window used to derive "current" list values. It is wide
// enough to tolerate DigitalOcean's metric ingestion lag.
const metricLookback = 30 * time.Minute

// listFetchConcurrency caps simultaneous metric requests during a refresh.
const listFetchConcurrency = 8

type eventKind int

const (
	evPage eventKind = iota
	evListDone
	evUsage
	evStatsDone
	evError
)

// refreshEvent streams incremental progress from a refresh goroutine.
type refreshEvent struct {
	gen   int
	kind  eventKind
	page  []do.Droplet
	id    int
	usage do.Usage
	err   error
}

// focusUsageMsg carries on-demand stats for the row the cursor just landed on
// (refresh-as-you-scroll); it applies regardless of the refresh generation.
type focusUsageMsg struct {
	id    int
	usage do.Usage
	err   error
}

type beginRefreshMsg struct{}
type tickMsg time.Time
type detailTickMsg struct{ gen int }
type sshFinishedMsg struct{ err error }
type detailDataMsg struct {
	gen    int
	series do.Series
	err    error
}

// viewState is the top-level screen the app is showing.
type viewState int

const (
	viewList viewState = iota
	viewDetail
)

// detailRefreshInterval is the poll cadence for the expanded Droplet. It is the
// delay *between* fetches (scheduled after each one completes), so fetches never
// overlap. The list is paused while expanded, leaving ample budget for 5s.
const detailRefreshInterval = 5 * time.Second

// defaultWindow is the initial (global session) chart time window.
const defaultWindow = time.Hour

// Model is the root Bubble Tea model.
type Model struct {
	api       do.API
	list      listModel
	width     int
	height    int
	loading   bool   // initial load with no rows yet
	err       error  // fatal list error (no data to show)
	metricErr string // sample metric-fetch error, surfaced in the footer

	gen         int
	cancel      context.CancelFunc
	refreshCtx  context.Context
	events      chan refreshEvent
	initialized bool // true once the first full stats load has completed

	state     viewState
	detail    detailModel
	window    time.Duration // global session chart window
	detailGen int

	hosts    *config.Hosts
	sshOpen  bool
	ssh      sshModal
	sshErr   string // transient (e.g. droplet has no public IP)
	helpOpen bool
}

// New builds the root model around a data-layer API and the SSH profile store.
// It seeds the list from the on-disk cache (if fresh) so launch is instant; the
// background refresh then updates names/status and fills in live stats.
func New(api do.API, hosts *config.Hosts) Model {
	m := Model{api: api, list: newListModel(), loading: true, window: defaultWindow, hosts: hosts}
	if cached, ok := config.LoadDropletCache(); ok {
		m.list.addPage(cached)
		m.loading = false
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(func() tea.Msg { return beginRefreshMsg{} }, tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// startRefresh supersedes any in-flight refresh and starts a new one. Phase 1
// streams Droplet-list pages (so they render immediately); when listing is done
// the model picks a cursor-centered window and Phase 2 fetches just those rows'
// stats (ADR-0002).
func (m Model) startRefresh() (Model, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
	}
	m.gen++
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel, m.refreshCtx = cancel, ctx
	ch := make(chan refreshEvent, 128)
	m.events = ch
	m.metricErr = ""
	if m.list.count() == 0 {
		m.loading = true
	}
	m.list.beginRefresh()
	go listProduce(ctx, m.gen, m.api, ch)
	return m, waitForEvent(ch)
}

func waitForEvent(ch chan refreshEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return refreshEvent{gen: -1, kind: evStatsDone} // superseded channel closed
		}
		return ev
	}
}

// listProduce streams Droplet-list pages, then signals listing is complete.
func listProduce(ctx context.Context, gen int, api do.API, ch chan refreshEvent) {
	defer close(ch)
	send := func(ev refreshEvent) {
		select {
		case ch <- ev:
		case <-ctx.Done():
		}
	}
	err := api.ListDropletsPaged(ctx, func(page []do.Droplet) {
		send(refreshEvent{gen: gen, kind: evPage, page: page})
	})
	if err != nil {
		send(refreshEvent{gen: gen, kind: evError, err: err})
	}
	send(refreshEvent{gen: gen, kind: evListDone})
}

// statsProduce fetches stats for the windowed Droplets concurrently, streaming
// each result back as it completes.
func statsProduce(ctx context.Context, gen int, api do.API, ch chan refreshEvent, droplets []do.Droplet) {
	defer close(ch)
	end := time.Now()
	start := end.Add(-metricLookback)
	send := func(ev refreshEvent) {
		select {
		case ch <- ev:
		case <-ctx.Done():
		}
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, listFetchConcurrency)
	for _, d := range droplets {
		wg.Add(1)
		go func(d do.Droplet) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			u, err := fetchUsage(ctx, api, d, start, end)
			send(refreshEvent{gen: gen, kind: evUsage, id: d.ID, usage: u, err: err})
		}(d)
	}
	wg.Wait()
	send(refreshEvent{gen: gen, kind: evStatsDone})
}

// fetchUsage retrieves and derives one Droplet's current usage.
func fetchUsage(ctx context.Context, api do.API, d do.Droplet, start, end time.Time) (do.Usage, error) {
	id := do.IDString(d.ID)
	cpu, e1 := api.CPUSeries(ctx, id, start, end)
	mem, e2 := api.MemAvailableSeries(ctx, id, start, end)
	fs, e3 := api.FSFreeSeries(ctx, id, start, end)
	return do.DeriveUsage(d, cpu, mem, fs, time.Now()), firstErr(e1, e2, e3)
}

// refreshDroplets chooses which active Droplets to fetch stats for: every one
// on the first load (so the initial sort is correct), then the cursor-centered
// budget window for steady-state refreshes.
func (m Model) refreshDroplets() []do.Droplet {
	if m.initialized {
		return m.windowDroplets()
	}
	var out []do.Droplet
	for _, r := range m.list.visible() {
		if r.D.Active() {
			out = append(out, r.D)
		}
	}
	return out
}

// windowDroplets selects the active Droplets to refresh this cycle: a
// cursor-centered window sized to the per-minute budget.
func (m Model) windowDroplets() []do.Droplet {
	vis := m.list.visible()
	reserved := 1 + len(vis)/200 // ~one list call per page
	sel := sched.SelectWindow(len(vis), func(i int) bool { return vis[i].D.Active() },
		m.list.cursorIndex(vis), sched.MaxDroplets(reserved))
	out := make([]do.Droplet, 0, len(sel))
	for i := range vis {
		if sel[i] {
			out = append(out, vis[i].D)
		}
	}
	return out
}

// startWindowStats re-fetches stats for the current cursor window without
// re-listing. Used after a sort or filter change so the newly-relevant rows
// refresh promptly. No-op until the initial full load has completed.
func (m Model) startWindowStats() (Model, tea.Cmd) {
	if !m.initialized || m.state == viewDetail {
		return m, nil
	}
	droplets := m.windowDroplets()
	if len(droplets) == 0 {
		return m, nil
	}
	if m.cancel != nil {
		m.cancel()
	}
	m.gen++
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel, m.refreshCtx = cancel, ctx
	ch := make(chan refreshEvent, 128)
	m.events = ch
	go statsProduce(ctx, m.gen, m.api, ch, droplets)
	return m, waitForEvent(ch)
}

// focusFetchCmd fetches stats for the row the cursor just moved to, if it is
// active and has no fresh data yet (refresh-as-you-scroll).
func (m Model) focusFetchCmd() tea.Cmd {
	sel, ok := m.list.selected()
	if !ok || !sel.D.Active() {
		return nil
	}
	if !sel.Pending && !sel.stale(time.Now()) {
		return nil // already fresh
	}
	api, d := m.api, sel.D
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		u, err := fetchUsage(ctx, api, d, time.Now().Add(-metricLookback), time.Now())
		return focusUsageMsg{id: d.ID, usage: u, err: err}
	}
}

// metricErrMessage turns a metric-fetch error into an actionable footer line.
func metricErrMessage(err error) string {
	if do.IsForbidden(err) {
		return "metrics forbidden (403): your API token lacks the Monitoring read scope. " +
			"Create a new token with Monitoring read (or Full Access) — scopes can't be edited after creation."
	}
	return "metric error: " + err.Error()
}

func detailTickCmd(gen int) tea.Cmd {
	return tea.Tick(detailRefreshInterval, func(time.Time) tea.Msg { return detailTickMsg{gen: gen} })
}

// fetchDetailCmd fetches the metric series for the expanded Droplet over the
// current window and derives the chart series. The three endpoints are fetched
// concurrently (so latency is one call, not three) and partial results are
// rendered — one slow or empty metric won't blank or fail the whole view.
func (m Model) fetchDetailCmd() tea.Cmd {
	api, d, window, gen := m.api, m.detail.d, m.window, m.detailGen
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		end := time.Now()
		start := end.Add(-window)
		id := do.IDString(d.ID)

		var cpu, mem, fs []metrics.SampleStream
		var e1, e2, e3 error
		var wg sync.WaitGroup
		wg.Add(3)
		go func() { defer wg.Done(); cpu, e1 = api.CPUSeries(ctx, id, start, end) }()
		go func() { defer wg.Done(); mem, e2 = api.MemAvailableSeries(ctx, id, start, end) }()
		go func() { defer wg.Done(); fs, e3 = api.FSFreeSeries(ctx, id, start, end) }()
		wg.Wait()

		series := do.DeriveSeries(d, cpu, mem, fs)
		if err := firstErr(e1, e2, e3); err != nil &&
			len(series.CPU) == 0 && len(series.Mem) == 0 && len(series.Disk) == 0 {
			return detailDataMsg{gen: gen, err: err}
		}
		return detailDataMsg{gen: gen, series: series}
	}
}

// openDetail expands the currently-selected Droplet.
func (m Model) openDetail() (tea.Model, tea.Cmd) {
	sel, ok := m.list.selected()
	if !ok {
		return m, nil
	}
	// The list is hidden while expanded, so stop refreshing it — this also
	// frees all API bandwidth for the expanded Droplet's faster polling.
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.state = viewDetail
	m.detailGen++
	m.detail = newDetailModel(sel.D, m.width, m.height)
	if !sel.D.Active() {
		m.detail.setErr("droplet is " + sel.D.Status + " — no metrics available")
		return m, nil
	}
	return m, m.fetchDetailCmd() // refresh chain self-schedules from each result
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.list.width, m.list.height = msg.Width, msg.Height
		m.detail.setSize(msg.Width, msg.Height)
		return m, nil

	case beginRefreshMsg:
		return m.startRefresh()

	case detailDataMsg:
		if msg.gen != m.detailGen {
			return m, nil // superseded by a window switch / re-open
		}
		if msg.err != nil {
			m.detail.setErr(metricErrMessage(msg.err))
		} else {
			m.detail.setSeries(msg.series)
		}
		if m.state == viewDetail {
			return m, detailTickCmd(m.detailGen) // schedule the next fetch 5s after this one
		}
		return m, nil

	case detailTickMsg:
		if m.state == viewDetail && msg.gen == m.detailGen {
			return m, m.fetchDetailCmd()
		}
		return m, nil

	case sshFinishedMsg:
		m.sshOpen = false
		if msg.err != nil {
			m.sshErr = "ssh: " + msg.err.Error()
		}
		return m, nil

	case tickMsg:
		if m.state == viewDetail || !m.initialized {
			// List hidden, or the initial full load is still running — keep the
			// timer alive but don't start a (superseding) windowed refresh yet.
			return m, tickCmd()
		}
		m2, cmd := m.startRefresh()
		return m2, tea.Batch(cmd, tickCmd())

	case refreshEvent:
		return m.handleEvent(msg)

	case focusUsageMsg:
		m.list.applyUsage(msg.id, msg.usage)
		if msg.err != nil && m.metricErr == "" {
			m.metricErr = metricErrMessage(msg.err)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.list.filtering {
		var cmd tea.Cmd
		m.list.input, cmd = m.list.input.Update(msg)
		m.list.filter = m.list.input.Value()
		return m, cmd
	}
	return m, nil
}

func (m Model) handleEvent(ev refreshEvent) (tea.Model, tea.Cmd) {
	if ev.gen != m.gen {
		return m, nil // stale or superseded refresh; stop reading that channel
	}
	switch ev.kind {
	case evPage:
		m.loading = false
		m.list.addPage(ev.page)
	case evError:
		if m.list.count() == 0 {
			m.err = ev.err
		} else if m.metricErr == "" {
			m.metricErr = metricErrMessage(ev.err)
		}
	case evListDone:
		// Listing complete: prune vanished Droplets, refresh the cache, then
		// fetch stats — all active Droplets on the first load, the cursor
		// window thereafter.
		m.list.endRefresh()
		_ = config.SaveDropletCache(m.list.droplets())
		droplets := m.refreshDroplets()
		if len(droplets) == 0 {
			m.initialized = true
			m.cancel = nil
			return m, nil
		}
		ch := make(chan refreshEvent, 128)
		m.events = ch
		go statsProduce(m.refreshCtx, m.gen, m.api, ch, droplets)
		return m, waitForEvent(ch)
	case evUsage:
		m.list.applyUsage(ev.id, ev.usage)
		if ev.err != nil && m.metricErr == "" {
			m.metricErr = metricErrMessage(ev.err)
		}
	case evStatsDone:
		m.loading = false
		m.initialized = true // the first full load (or any cycle) is done
		m.cancel = nil
		return m, nil // stop listening; channel will close
	}
	return m, waitForEvent(m.events)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.sshOpen {
		return m.handleSSHKey(msg)
	}
	if m.helpOpen { // any key dismisses the help overlay
		m.helpOpen = false
		return m, nil
	}
	filtering := m.state == viewList && m.list.filtering
	if msg.String() == keyHelp && !filtering {
		m.helpOpen = true
		return m, nil
	}
	if m.state == viewDetail {
		return m.handleDetailKey(msg)
	}
	if m.list.filtering {
		switch msg.String() {
		case keyEsc:
			m.list.filtering = false
			m.list.filter = ""
			m.list.input.SetValue("")
			m.list.input.Blur()
			return m.startWindowStats() // re-window over the unfiltered list
		case keyExpand:
			m.list.filtering = false
			m.list.input.Blur()
			return m.startWindowStats() // re-window over the filtered set
		default:
			var cmd tea.Cmd
			m.list.input, cmd = m.list.input.Update(msg)
			m.list.filter = m.list.input.Value()
			return m, cmd
		}
	}

	switch msg.String() {
	case keyQuit, keyQuitCtrl:
		return m, tea.Quit
	case keyDown, keyDownArrow:
		m.list.move(1)
		return m, m.focusFetchCmd()
	case keyUp, keyUpArrow:
		m.list.move(-1)
		return m, m.focusFetchCmd()
	case keyPageDown:
		m.list.move(m.list.rowsHeight() - 1)
		return m, m.focusFetchCmd()
	case keyPageUp:
		m.list.move(-(m.list.rowsHeight() - 1))
		return m, m.focusFetchCmd()
	case keyTop:
		m.list.gotoTop()
		return m, m.focusFetchCmd()
	case keyBottom:
		m.list.gotoBottom()
		return m, m.focusFetchCmd()
	case keyFilter:
		m.list.filtering = true
		m.list.input.Focus()
		return m, textinput.Blink
	case keySortName:
		m.list.toggleSort(sortName)
		return m.startWindowStats()
	case keySortCPU:
		m.list.toggleSort(sortCPU)
		return m.startWindowStats()
	case keySortMem:
		m.list.toggleSort(sortMem)
		return m.startWindowStats()
	case keySortDisk:
		m.list.toggleSort(sortDisk)
		return m.startWindowStats()
	case keyExpand, keyExpandAlt:
		return m.openDetail()
	case keySSH:
		return m.startSSH(false)
	case keySSHEdit:
		return m.startSSH(true)
	case keyRefresh:
		m.initialized = false // re-fetch all stats, not just the window
		m2, cmd := m.startRefresh()
		return m2, cmd
	}
	return m, nil
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyQuit, keyQuitCtrl:
		return m, tea.Quit
	case keyEsc, "left", keyExpandAlt:
		m.state = viewList // cursor is preserved (pinned by ID)
		// Resume list refreshing (paused while expanded); data may be stale.
		m2, cmd := m.startRefresh()
		return m2, cmd
	case keySSH:
		return m.startSSH(false)
	case keySSHEdit:
		return m.startSSH(true)
	case "1", "2", "3":
		windows := map[string]time.Duration{"1": time.Hour, "2": 6 * time.Hour, "3": 24 * time.Hour}
		m.window = windows[msg.String()]
		m.detailGen++
		m.detail.loading = true
		return m, m.fetchDetailCmd()
	}
	return m, nil
}

// currentDroplet returns the Droplet the SSH action targets: the expanded one
// in detail mode, otherwise the selected list row.
func (m Model) currentDroplet() (do.Droplet, bool) {
	if m.state == viewDetail {
		return m.detail.d, true
	}
	if sel, ok := m.list.selected(); ok {
		return sel.D, true
	}
	return do.Droplet{}, false
}

// startSSH connects to the current Droplet, prompting for user+key on the first
// connection (or when forceModal edits an existing profile).
func (m Model) startSSH(forceModal bool) (tea.Model, tea.Cmd) {
	d, ok := m.currentDroplet()
	if !ok {
		return m, nil
	}
	if d.PublicIP == "" {
		m.sshErr = "no public IPv4 for " + d.Name
		return m, nil
	}
	m.sshErr = ""
	p, have := m.hosts.Get(d.ID)
	if have && !forceModal {
		return m, sshConnectCmd(d, p)
	}
	m.sshOpen = true
	m.ssh = newSSHModal(d, p)
	return m, textinput.Blink
}

func (m Model) handleSSHKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var act sshAction
	var cmd tea.Cmd
	m.ssh, act, cmd = m.ssh.update(msg)
	switch act {
	case sshCancel:
		m.sshOpen = false
		return m, nil
	case sshConnect:
		p := config.Profile{
			User: strings.TrimSpace(m.ssh.user.Value()),
			Key:  strings.TrimSpace(m.ssh.key.Value()),
		}
		d := m.ssh.d
		if err := m.hosts.Set(d.ID, p); err != nil {
			m.ssh.errMsg = "could not save profile: " + err.Error()
			return m, nil
		}
		m.sshOpen = false
		return m, sshConnectCmd(d, p)
	}
	return m, cmd
}

// sshConnectCmd suspends the TUI and execs the system ssh binary, restoring the
// TUI when the session ends.
func sshConnectCmd(d do.Droplet, p config.Profile) tea.Cmd {
	var args []string
	if key := strings.TrimSpace(p.Key); key != "" {
		args = append(args, "-i", expandHome(key))
	}
	user := firstNonEmpty(p.User, "root")
	args = append(args, user+"@"+d.PublicIP)
	c := exec.Command("ssh", args...)
	return tea.ExecProcess(c, func(err error) tea.Msg { return sshFinishedMsg{err: err} })
}

func (m Model) View() string {
	if m.helpOpen {
		return helpView(m.width, m.height)
	}
	if m.sshOpen {
		return m.ssh.view(m.width, m.height)
	}
	if m.state == viewDetail {
		return m.detail.view(m.window, time.Now())
	}
	if m.err != nil && m.list.count() == 0 {
		return lipgloss.NewStyle().Foreground(colRed).Render("Error: "+m.err.Error()) + "\n"
	}
	if m.loading && m.list.count() == 0 {
		return styleFooter.Render("Loading droplets…")
	}
	if m.initialized && m.list.count() == 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			styleDim.Render("No droplets in this account.\nPress r to refresh · q to quit."))
	}
	out := m.list.view(time.Now())
	if m.sshErr != "" {
		out += "\n" + lipgloss.NewStyle().Foreground(colRed).Render(m.sshErr)
	}
	if m.metricErr != "" {
		out += "\n" + lipgloss.NewStyle().Foreground(colOrange).Render(m.metricErr)
	}
	return out
}
