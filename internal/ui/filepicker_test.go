package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rohittp0/sailor/internal/do"
)

func runeKey(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.Mkdir(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p string) {
	t.Helper()
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// buildTree lays out: adir/child.txt, bdir/, afile.txt, .hidden
func buildTree(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	mustMkdir(t, filepath.Join(tmp, "adir"))
	mustMkdir(t, filepath.Join(tmp, "bdir"))
	mustWrite(t, filepath.Join(tmp, "adir", "child.txt"))
	mustWrite(t, filepath.Join(tmp, "afile.txt"))
	mustWrite(t, filepath.Join(tmp, ".hidden"))
	return tmp
}

func TestFilePickerListingAndHidden(t *testing.T) {
	m := newFilePickerModel(buildTree(t), do.Droplet{Name: "web"})

	// "..", adir, bdir, afile.txt — dotfile excluded, dirs before files.
	if got := len(m.entries); got != 4 {
		t.Fatalf("entries=%d, want 4; got %+v", got, m.entries)
	}
	if m.entries[0].name != ".." || m.entries[1].name != "adir" ||
		m.entries[2].name != "bdir" || m.entries[3].name != "afile.txt" {
		t.Fatalf("unexpected order: %+v", m.entries)
	}

	// '.' reveals the hidden file, again hides it.
	m, _, _ = m.update(runeKey('.'))
	if len(m.entries) != 5 {
		t.Fatalf("showHidden entries=%d, want 5", len(m.entries))
	}
	m, _, _ = m.update(runeKey('.'))
	if len(m.entries) != 4 {
		t.Fatalf("re-hidden entries=%d, want 4", len(m.entries))
	}
}

func TestFilePickerNavigation(t *testing.T) {
	tmp := buildTree(t)
	m := newFilePickerModel(tmp, do.Droplet{})

	m, _, _ = m.update(key(tea.KeyDown)) // cursor -> adir
	if e, _ := m.current(); e.name != "adir" {
		t.Fatalf("cursor on %q, want adir", e.name)
	}
	m, _, _ = m.update(runeKey('l')) // descend into adir
	if m.dir != filepath.Join(tmp, "adir") {
		t.Fatalf("dir=%q, want adir", m.dir)
	}
	if len(m.entries) != 2 { // "..", child.txt
		t.Fatalf("adir entries=%d, want 2", len(m.entries))
	}
	m, _, _ = m.update(runeKey('h')) // back up
	if m.dir != tmp {
		t.Fatalf("dir=%q, want %q", m.dir, tmp)
	}
}

func TestFilePickerConfirm(t *testing.T) {
	tmp := buildTree(t)
	m := newFilePickerModel(tmp, do.Droplet{})

	// enter with nothing selected -> no-op + hint.
	m, act, _ := m.update(key(tea.KeyEnter))
	if act != fpNone || m.errMsg == "" {
		t.Fatalf("empty confirm: act=%v err=%q, want fpNone + hint", act, m.errMsg)
	}

	// select afile.txt, then enter -> upload.
	m, _, _ = m.update(key(tea.KeyDown)) // adir
	m, _, _ = m.update(key(tea.KeyDown)) // bdir
	m, _, _ = m.update(key(tea.KeyDown)) // afile.txt
	m, _, _ = m.update(runeKey(' '))     // select
	if !m.selected[filepath.Join(tmp, "afile.txt")] {
		t.Fatal("afile.txt should be selected")
	}
	if _, act, _ = m.update(key(tea.KeyEnter)); act != fpUpload {
		t.Fatalf("confirm with selection: act=%v, want fpUpload", act)
	}
}

func TestFilePickerNestedDedup(t *testing.T) {
	tmp := buildTree(t)
	m := newFilePickerModel(tmp, do.Droplet{})

	// Select adir (the parent folder).
	m, _, _ = m.update(key(tea.KeyDown)) // adir
	m, _, _ = m.update(runeKey(' '))
	// Descend and also select the child file inside it.
	m, _, _ = m.update(runeKey('l'))     // into adir
	m, _, _ = m.update(key(tea.KeyDown)) // child.txt
	m, _, _ = m.update(runeKey(' '))

	got := m.selectedPaths()
	if len(got) != 1 || got[0] != filepath.Join(tmp, "adir") {
		t.Fatalf("selectedPaths=%v, want only the parent dir", got)
	}
}

func TestFilePickerFilter(t *testing.T) {
	m := newFilePickerModel(buildTree(t), do.Droplet{})

	m, _, _ = m.update(runeKey('/')) // enter filter mode
	m, _, _ = m.update(runeKey('a'))
	m, _, _ = m.update(runeKey('f')) // "af" matches afile.txt only (plus "..")
	vis := m.visible()
	if len(vis) != 2 || vis[0].name != ".." || vis[1].name != "afile.txt" {
		t.Fatalf("filtered visible=%+v, want [.., afile.txt]", vis)
	}
}
