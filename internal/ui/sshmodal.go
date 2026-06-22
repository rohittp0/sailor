package ui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"sailor/internal/config"
	"sailor/internal/do"
)

type sshAction int

const (
	sshNone sshAction = iota
	sshConnect
	sshCancel
)

// sshModal is the centered prompt for a Droplet's SSH user + identity-file path.
type sshModal struct {
	d      do.Droplet
	user   textinput.Model
	key    textinput.Model
	focus  int // 0 = user, 1 = key
	errMsg string
}

func newSSHModal(d do.Droplet, p config.Profile) sshModal {
	u := textinput.New()
	u.Prompt = ""
	u.SetValue(firstNonEmpty(p.User, "root"))
	u.Focus()
	k := textinput.New()
	k.Prompt = ""
	k.SetValue(firstNonEmpty(p.Key, config.DefaultKeyPath()))
	return sshModal{d: d, user: u, key: k, focus: 0}
}

func (s sshModal) update(msg tea.KeyMsg) (sshModal, sshAction, tea.Cmd) {
	switch msg.String() {
	case keyEsc:
		return s, sshCancel, nil
	case keyExpand: // enter confirms
		if strings.TrimSpace(s.user.Value()) == "" {
			s.errMsg = "user is required"
			return s, sshNone, nil
		}
		if key := strings.TrimSpace(s.key.Value()); key != "" {
			if _, err := os.Stat(expandHome(key)); err != nil {
				s.errMsg = "key not found: " + key
				return s, sshNone, nil
			}
		}
		return s, sshConnect, nil
	case "tab", "down", "shift+tab", "up":
		s.focus = 1 - s.focus
		if s.focus == 0 {
			s.user.Focus()
			s.key.Blur()
		} else {
			s.key.Focus()
			s.user.Blur()
		}
		return s, sshNone, nil
	}
	var cmd tea.Cmd
	if s.focus == 0 {
		s.user, cmd = s.user.Update(msg)
	} else {
		s.key, cmd = s.key.Update(msg)
	}
	s.errMsg = ""
	return s, sshNone, cmd
}

func (s sshModal) view(w, h int) string {
	label := lipgloss.NewStyle().Foreground(colDim)
	field := func(name string, in textinput.Model, focused bool) string {
		caret := "  "
		if focused {
			caret = lipgloss.NewStyle().Foreground(colAccent).Render("▸ ")
		}
		return caret + label.Render(pad(name, 6)) + in.View()
	}
	body := lipgloss.NewStyle().Bold(true).Foreground(colAccent).Render("SSH → "+s.d.Name) + "\n" +
		styleFooter.Render(s.d.PublicIP) + "\n\n" +
		field("user", s.user, s.focus == 0) + "\n" +
		field("key", s.key, s.focus == 1) + "\n"
	if s.errMsg != "" {
		body += "\n" + lipgloss.NewStyle().Foreground(colRed).Render(s.errMsg)
	}
	body += "\n\n" + styleFooter.Render("tab·switch  enter·connect  esc·cancel")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colAccent).
		Padding(1, 2).
		Render(body)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// expandHome resolves a leading ~ to the user's home directory.
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
