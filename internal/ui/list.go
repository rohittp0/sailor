package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/rohittp0/sailor/internal/do"
)

type sortCol int

const (
	sortCPU sortCol = iota
	sortMem
	sortDisk
	sortName
)

func (c sortCol) label() string {
	switch c {
	case sortName:
		return "name"
	case sortMem:
		return "mem"
	case sortDisk:
		return "disk"
	default:
		return "cpu"
	}
}

// Row pairs a Droplet with its most recent derived Usage. Pending marks a row
// whose stats are still being fetched (shown as "loading").
type Row struct {
	D        do.Droplet
	U        do.Usage
	HasUsage bool
	Pending  bool
}

// staleAfter is how long until a row's last-known values are dimmed.
const staleAfter = 2 * time.Minute

func (r Row) stale(now time.Time) bool {
	return r.HasUsage && now.Sub(r.U.UpdatedAt) > staleAfter
}

type listModel struct {
	data      map[int]Row // source of truth, keyed by Droplet ID
	order     []int       // sorted Droplet IDs (rebuilt on mutation)
	seen      map[int]bool
	sortCol   sortCol
	desc      bool
	filter    string
	filtering bool
	input     textinput.Model
	cursorID  int
	top       int
	width     int
	height    int
}

func newListModel() listModel {
	in := textinput.New()
	in.Placeholder = "filter by name"
	in.Prompt = "/"
	return listModel{data: map[int]Row{}, sortCol: sortCPU, desc: true, input: in}
}

func (m *listModel) count() int { return len(m.data) }

// droplets returns all Droplets in sorted order (used to persist the cache).
func (m *listModel) droplets() []do.Droplet {
	out := make([]do.Droplet, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.data[id].D)
	}
	return out
}

// setSnapshot replaces all rows at once (used in tests and for a clean reset).
func (m *listModel) setSnapshot(droplets []do.Droplet, usage map[int]do.Usage) {
	m.data = make(map[int]Row, len(droplets))
	for _, d := range droplets {
		r := Row{D: d}
		if u, ok := usage[d.ID]; ok {
			r.U, r.HasUsage = u, true
		} else {
			r.Pending = d.Active()
		}
		m.data[d.ID] = r
	}
	m.rebuild()
	m.ensureCursor()
}

// beginRefresh starts tracking which Droplets are seen this cycle (for pruning).
func (m *listModel) beginRefresh() { m.seen = map[int]bool{} }

// addPage merges a freshly-listed page: existing rows keep their last-known
// usage (shown stale until updated); new rows arrive Pending.
func (m *listModel) addPage(droplets []do.Droplet) {
	for _, d := range droplets {
		if m.seen != nil {
			m.seen[d.ID] = true
		}
		if existing, ok := m.data[d.ID]; ok {
			existing.D = d
			if !d.Active() { // became/stayed off: show off state, never fetched
				existing.U = do.Usage{State: do.StateOff}
				existing.HasUsage, existing.Pending = true, false
			}
			m.data[d.ID] = existing
		} else if d.Active() {
			m.data[d.ID] = Row{D: d, Pending: true}
		} else {
			m.data[d.ID] = Row{D: d, U: do.Usage{State: do.StateOff}, HasUsage: true}
		}
	}
	m.rebuild()
	m.ensureCursor()
}

// applyUsage updates a single row's stats as they arrive.
func (m *listModel) applyUsage(id int, u do.Usage) {
	r, ok := m.data[id]
	if !ok {
		return
	}
	r.U, r.HasUsage, r.Pending = u, true, false
	m.data[id] = r
	m.rebuild()
}

// endRefresh prunes rows for Droplets no longer present in the account.
func (m *listModel) endRefresh() {
	if m.seen == nil {
		return
	}
	for id := range m.data {
		if !m.seen[id] {
			delete(m.data, id)
		}
	}
	m.seen = nil
	m.rebuild()
	m.ensureCursor()
}

// rebuild recomputes the sorted ID order.
func (m *listModel) rebuild() {
	m.order = m.order[:0]
	for id := range m.data {
		m.order = append(m.order, id)
	}
	sort.SliceStable(m.order, func(i, j int) bool {
		a, b := m.data[m.order[i]], m.data[m.order[j]]
		if eq, less := m.primaryLess(a, b); !eq {
			return less
		}
		if a.D.Name != b.D.Name {
			return a.D.Name < b.D.Name
		}
		return a.D.ID < b.D.ID
	})
}

func (m *listModel) ensureCursor() {
	if _, ok := m.data[m.cursorID]; ok {
		return
	}
	vis := m.visible()
	if len(vis) > 0 {
		m.cursorID = vis[0].D.ID
	} else {
		m.cursorID = 0
	}
}

func (m *listModel) primaryLess(a, b Row) (eq, less bool) {
	switch m.sortCol {
	case sortName:
		if a.D.Name == b.D.Name {
			return true, false
		}
		if m.desc {
			return false, a.D.Name > b.D.Name
		}
		return false, a.D.Name < b.D.Name
	case sortMem:
		return metricLess(a.U.Mem, a.U.MemValid, b.U.Mem, b.U.MemValid, m.desc)
	case sortDisk:
		return metricLess(a.U.Disk, a.U.DiskValid, b.U.Disk, b.U.DiskValid, m.desc)
	default:
		return metricLess(a.U.CPU, a.U.CPUValid, b.U.CPU, b.U.CPUValid, m.desc)
	}
}

// metricLess orders valid metrics by value (honoring desc) and always sinks
// invalid ("n/a"/off/loading) rows to the bottom regardless of direction.
func metricLess(av float64, aok bool, bv float64, bok bool, desc bool) (eq, less bool) {
	if aok != bok {
		return false, aok
	}
	if !aok || av == bv {
		return true, false
	}
	if desc {
		return false, av > bv
	}
	return false, av < bv
}

func (m *listModel) visible() []Row {
	q := strings.ToLower(m.filter)
	out := make([]Row, 0, len(m.order))
	for _, id := range m.order {
		r := m.data[id]
		if q == "" || strings.Contains(strings.ToLower(r.D.Name), q) {
			out = append(out, r)
		}
	}
	return out
}

func (m *listModel) cursorIndex(vis []Row) int {
	for i, r := range vis {
		if r.D.ID == m.cursorID {
			return i
		}
	}
	return 0
}

func (m *listModel) selected() (Row, bool) {
	vis := m.visible()
	if len(vis) == 0 {
		return Row{}, false
	}
	return vis[m.cursorIndex(vis)], true
}

func (m *listModel) move(delta int) { m.setCursorIdx(m.cursorIndex(m.visible()) + delta) }
func (m *listModel) gotoTop()       { m.setCursorIdx(0) }
func (m *listModel) gotoBottom()    { m.setCursorIdx(1 << 30) }

func (m *listModel) setCursorIdx(i int) {
	vis := m.visible()
	if len(vis) == 0 {
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= len(vis) {
		i = len(vis) - 1
	}
	m.cursorID = vis[i].D.ID
}

func (m *listModel) toggleSort(c sortCol) {
	if m.sortCol == c {
		m.desc = !m.desc
	} else {
		m.sortCol = c
		m.desc = c != sortName
	}
	m.rebuild()
}

func (m *listModel) rowsHeight() int {
	h := m.height - 4
	if h < 1 {
		return 1
	}
	return h
}

func (m *listModel) view(now time.Time) string {
	var b strings.Builder

	dir := "↓"
	if !m.desc {
		dir = "↑"
	}
	b.WriteString(styleHeader.Render("SAILOR") + "   " +
		styleFooter.Render(fmt.Sprintf("sort: %s %s", m.sortCol.label(), dir)) + "\n")

	vis := m.visible()
	nameW := m.nameWidth()
	if m.filtering {
		b.WriteString(m.input.View() + styleFooter.Render(fmt.Sprintf("   (%d match)", len(vis))) + "\n")
	} else {
		b.WriteString(styleColHead.Render(m.columnHeader(nameW)) + "\n")
	}

	h := m.rowsHeight()
	ci := m.cursorIndex(vis)
	if ci < m.top {
		m.top = ci
	}
	if ci >= m.top+h {
		m.top = ci - h + 1
	}
	if m.top < 0 {
		m.top = 0
	}
	end := m.top + h
	if end > len(vis) {
		end = len(vis)
	}

	if len(vis) == 0 {
		b.WriteString(styleDim.Render("  (no droplets)") + "\n")
	}
	for i := m.top; i < end; i++ {
		b.WriteString(m.renderRow(vis[i], vis[i].D.ID == m.cursorID, nameW, now) + "\n")
	}

	b.WriteString("\n" + styleFooter.Render(footerHint))
	return b.String()
}

// metricColumns is how many metric columns fit at the current width: CPU only
// when narrow, +MEM when wider, +DISK when widest (responsive reflow).
func metricColumns(width int) int {
	switch {
	case width >= 80:
		return 3
	case width >= 58:
		return 2
	default:
		return 1
	}
}

func (m *listModel) nameWidth() int {
	cols := metricColumns(m.width)
	w := m.width - 2 - 2 - cols*(barWidth+6) - 2
	if w < 8 {
		w = 8
	}
	if w > 40 {
		w = 40
	}
	return w
}

func (m *listModel) columnHeader(nameW int) string {
	heads := []string{"CPU", "MEM", "DISK"}
	s := fmt.Sprintf("  %s %s", pad("S", 1), pad("NAME", nameW))
	for i := 0; i < metricColumns(m.width); i++ {
		s += " " + pad(heads[i], barWidth+5)
	}
	return s
}

func (m *listModel) renderRow(r Row, selected bool, nameW int, now time.Time) string {
	stale := r.stale(now)
	marker := "  "
	if selected {
		marker = lipgloss.NewStyle().Foreground(colAccent).Render("▸ ")
	}
	glyph := statusGlyph(r.U.State, r.D.Status)

	nameStyle := styleName
	if stale || r.U.State == do.StateOff || (r.Pending && !r.HasUsage) {
		nameStyle = styleDim
	}
	name := nameStyle.Render(pad(r.D.Name, nameW))

	var cells [3]string
	if r.Pending && !r.HasUsage {
		cells = [3]string{loadingCell(), loadingCell(), loadingCell()}
		glyph = styleDim.Render("◌")
	} else {
		cells = [3]string{
			renderMetric(r.U.CPU, r.U.CPUValid, r.U.State, stale),
			renderMetric(r.U.Mem, r.U.MemValid, r.U.State, stale),
			renderMetric(r.U.Disk, r.U.DiskValid, r.U.State, stale),
		}
	}

	line := fmt.Sprintf("%s%s %s", marker, glyph, name)
	for i := 0; i < metricColumns(m.width); i++ {
		line += " " + cells[i]
	}
	if selected {
		return lipgloss.NewStyle().Bold(true).Render(line)
	}
	return line
}
