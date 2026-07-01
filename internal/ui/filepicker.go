package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rohittp0/sailor/internal/do"
)

// fpAction is the outcome of a file-picker key press bubbled up to the app.
type fpAction int

const (
	fpNone fpAction = iota
	fpUpload
	fpCancel
)

// fpEntry is one row in the picker: a file or directory in the current dir,
// plus the synthetic ".." parent entry.
type fpEntry struct {
	name  string
	isDir bool
	path  string // absolute
}

// filePickerModel is the terminal file/folder chooser for an SCP upload. It
// browses the local filesystem with vim-style keys and multi-selects entries;
// selections persist across directories (keyed by absolute path) so files can
// be gathered from several folders in one upload.
type filePickerModel struct {
	d          do.Droplet      // upload target (for the header)
	dir        string          // current absolute directory
	entries    []fpEntry       // ".." first, then dirs, then files
	cursor     int             // index into the filtered view
	top        int             // viewport scroll offset
	selected   map[string]bool // absolute path -> checked
	showHidden bool
	filter     string
	filtering  bool
	input      textinput.Model
	width      int
	height     int
	errMsg     string
}

func newFilePickerModel(startDir string, d do.Droplet) filePickerModel {
	in := textinput.New()
	in.Placeholder = "filter"
	in.Prompt = "/"
	m := filePickerModel{d: d, dir: startDir, selected: map[string]bool{}, input: in}
	m.readDir()
	return m
}

// readDir loads the current directory's entries (dirs first, then files, each
// case-insensitively sorted), prepending "..". Dotfiles are hidden unless
// showHidden. On error the previous listing is kept and errMsg is set.
func (m *filePickerModel) readDir() {
	ents, err := os.ReadDir(m.dir)
	if err != nil {
		m.errMsg = err.Error()
		return
	}
	var dirs, files []fpEntry
	for _, e := range ents {
		name := e.Name()
		if !m.showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		fe := fpEntry{name: name, isDir: e.IsDir(), path: filepath.Join(m.dir, name)}
		if fe.isDir {
			dirs = append(dirs, fe)
		} else {
			files = append(files, fe)
		}
	}
	byName := func(s []fpEntry) func(i, j int) bool {
		return func(i, j int) bool { return strings.ToLower(s[i].name) < strings.ToLower(s[j].name) }
	}
	sort.Slice(dirs, byName(dirs))
	sort.Slice(files, byName(files))

	m.entries = make([]fpEntry, 0, len(dirs)+len(files)+1)
	m.entries = append(m.entries, fpEntry{name: "..", isDir: true, path: filepath.Dir(m.dir)})
	m.entries = append(m.entries, dirs...)
	m.entries = append(m.entries, files...)
	m.cursor, m.top, m.errMsg = 0, 0, ""
}

// visible applies the name filter (".." is always shown).
func (m *filePickerModel) visible() []fpEntry {
	if m.filter == "" {
		return m.entries
	}
	q := strings.ToLower(m.filter)
	out := make([]fpEntry, 0, len(m.entries))
	for _, e := range m.entries {
		if e.name == ".." || strings.Contains(strings.ToLower(e.name), q) {
			out = append(out, e)
		}
	}
	return out
}

func (m *filePickerModel) current() (fpEntry, bool) {
	vis := m.visible()
	if m.cursor < 0 || m.cursor >= len(vis) {
		return fpEntry{}, false
	}
	return vis[m.cursor], true
}

func (m *filePickerModel) move(delta int) { m.setCursor(m.cursor + delta) }

func (m *filePickerModel) setCursor(i int) {
	n := len(m.visible())
	if n == 0 {
		m.cursor = 0
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= n {
		i = n - 1
	}
	m.cursor = i
}

func (m *filePickerModel) descend() {
	e, ok := m.current()
	if !ok || !e.isDir {
		return // files aren't navigable; select them with space
	}
	m.dir = e.path // ".." carries the parent path
	m.readDir()
}

func (m *filePickerModel) ascend() {
	m.dir = filepath.Dir(m.dir)
	m.readDir()
}

func (m *filePickerModel) toggleSelect() {
	e, ok := m.current()
	if !ok || e.name == ".." {
		return
	}
	if m.selected[e.path] {
		delete(m.selected, e.path)
	} else {
		m.selected[e.path] = true
	}
}

// selectedPaths returns the checked paths with nested selections collapsed: if a
// directory and something inside it are both checked, only the directory is
// returned (its recursive copy already includes the child).
func (m *filePickerModel) selectedPaths() []string {
	var sel []string
	for p, ok := range m.selected {
		if ok {
			sel = append(sel, p)
		}
	}
	sort.Strings(sel) // ancestors sort before their descendants
	var out []string
	for _, p := range sel {
		covered := false
		for _, k := range out {
			if strings.HasPrefix(p, k+string(os.PathSeparator)) {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, p)
		}
	}
	return out
}

func (m *filePickerModel) selectedCount() int {
	n := 0
	for _, ok := range m.selected {
		if ok {
			n++
		}
	}
	return n
}

func (m filePickerModel) update(msg tea.KeyMsg) (filePickerModel, fpAction, tea.Cmd) {
	if m.filtering {
		switch msg.String() {
		case keyEsc:
			m.filtering = false
			m.filter = ""
			m.input.SetValue("")
			m.input.Blur()
			m.setCursor(m.cursor)
			return m, fpNone, nil
		case keyExpand: // apply filter, leave filter mode
			m.filtering = false
			m.input.Blur()
			return m, fpNone, nil
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			m.filter = m.input.Value()
			m.setCursor(m.cursor)
			return m, fpNone, cmd
		}
	}

	switch msg.String() {
	case keyEsc:
		return m, fpCancel, nil
	case keyDown, keyDownArrow:
		m.move(1)
	case keyUp, keyUpArrow:
		m.move(-1)
	case keyPageDown:
		m.move(m.rowsHeight() - 1)
	case keyPageUp:
		m.move(-(m.rowsHeight() - 1))
	case keyTop:
		m.setCursor(0)
	case keyBottom:
		m.setCursor(1 << 30)
	case "right", "l":
		m.descend()
	case "left", "h", "backspace":
		m.ascend()
	case " ":
		m.toggleSelect()
	case ".":
		m.showHidden = !m.showHidden
		m.readDir()
	case keyFilter:
		m.filtering = true
		m.input.Focus()
		return m, fpNone, textinput.Blink
	case keyExpand: // confirm & upload
		if len(m.selectedPaths()) == 0 {
			m.errMsg = "select with space"
			return m, fpNone, nil
		}
		return m, fpUpload, nil
	}
	return m, fpNone, nil
}

func (m *filePickerModel) rowsHeight() int {
	h := m.height - 8
	if h < 3 {
		h = 3
	}
	if h > 20 {
		h = 20
	}
	return h
}

func (m filePickerModel) view(w, h int) string {
	m.width, m.height = w, h
	innerW := w - 8
	if innerW < 30 {
		innerW = 30
	}
	if innerW > 72 {
		innerW = 72
	}

	title := lipgloss.NewStyle().Bold(true).Foreground(colAccent).Render("scp → " + m.d.Name)
	head := title + "   " + styleFooter.Render(fmt.Sprintf("%d selected", m.selectedCount()))
	dirLine := styleDim.Render(truncate(m.dir, innerW))

	var b strings.Builder
	b.WriteString(head + "\n" + dirLine + "\n")
	if m.filtering {
		b.WriteString(m.input.View() + "\n")
	} else {
		b.WriteString("\n")
	}

	vis := m.visible()
	rows := m.rowsHeight()
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+rows {
		m.top = m.cursor - rows + 1
	}
	if m.top < 0 {
		m.top = 0
	}
	end := m.top + rows
	if end > len(vis) {
		end = len(vis)
	}
	if len(vis) == 0 {
		b.WriteString(styleDim.Render("  (empty)") + "\n")
	}
	for i := m.top; i < end; i++ {
		b.WriteString(m.renderRow(vis[i], i == m.cursor, innerW) + "\n")
	}

	if m.errMsg != "" {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(colRed).Render(m.errMsg))
	}
	b.WriteString("\n" + styleFooter.Render("space·select  l·open  h·up  enter·upload  /·filter  .·hidden  esc·cancel"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colAccent).
		Padding(1, 2).
		Render(b.String())
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}

func (m *filePickerModel) renderRow(e fpEntry, cursor bool, innerW int) string {
	marker := "  "
	if cursor {
		marker = lipgloss.NewStyle().Foreground(colAccent).Render("▸ ")
	}
	box := "[ ]"
	switch {
	case e.name == "..":
		box = "   "
	case m.selected[e.path]:
		box = lipgloss.NewStyle().Foreground(colAccent).Render("[x]")
	}
	name := e.name
	if e.isDir {
		name += "/"
	}
	name = truncate(name, innerW-6)
	line := marker + box + " " + name
	if cursor {
		return lipgloss.NewStyle().Bold(true).Render(line)
	}
	return line
}
