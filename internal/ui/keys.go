package ui

// Keybindings (vim/k9s conventions). Kept as plain strings matched against
// tea.KeyMsg.String() in Update; the help overlay (M6) renders from this table.
const (
	keyUp        = "k"
	keyUpArrow   = "up"
	keyDown      = "j"
	keyDownArrow = "down"
	keyTop       = "g"
	keyBottom    = "G"
	keyPageDown  = "ctrl+d"
	keyPageUp    = "ctrl+u"
	keyFilter    = "/"
	keySortName  = "N"
	keySortCPU   = "C"
	keySortMem   = "M"
	keySortDisk  = "D"
	keyExpand    = "enter"
	keyExpandAlt = "e"
	keySSH       = "s"
	keySSHEdit   = "S"
	keySCP       = "u"
	keyRefresh   = "r"
	keyHelp      = "?"
	keyQuit      = "q"
	keyQuitCtrl  = "ctrl+c"
	keyEsc       = "esc"
)

// footerHint is the persistent key bar shown under the list.
const footerHint = "?·help  /·filter  N/C/M/D·sort  enter·expand  s·ssh  u·upload  r·refresh  q·quit"

// detailFooterHint is the key bar shown under the expanded view.
const detailFooterHint = "1/2/3·window  s·ssh  u·upload  esc·back  ?·help  q·quit"

// helpEntry is one keybinding row in the help overlay.
type helpEntry struct{ keys, desc string }

var helpSections = []struct {
	title   string
	entries []helpEntry
}{
	{"Navigate", []helpEntry{
		{"j / k  ↑ / ↓", "move cursor"},
		{"g / G", "jump to top / bottom"},
		{"ctrl+d / ctrl+u", "page down / up"},
	}},
	{"List", []helpEntry{
		{"/", "filter by name (esc clears)"},
		{"N / C / M / D", "sort by name / cpu / mem / disk"},
		{"enter", "expand to charts"},
		{"s / S", "ssh / edit ssh profile"},
		{"u", "upload files (scp)"},
		{"r", "refresh all"},
	}},
	{"Detail", []helpEntry{
		{"1 / 2 / 3", "window 1h / 6h / 24h"},
		{"s", "ssh"},
		{"u", "upload files (scp)"},
		{"esc", "back to list"},
	}},
	{"General", []helpEntry{
		{"?", "toggle this help"},
		{"q", "quit"},
	}},
}
