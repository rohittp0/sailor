package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// helpView renders the centered keybinding overlay.
func helpView(w, h int) string {
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	descStyle := lipgloss.NewStyle().Foreground(colText)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colYellow)

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colAccent).Render("SAILOR — keys") + "\n")
	for _, sec := range helpSections {
		b.WriteString("\n" + titleStyle.Render(sec.title) + "\n")
		for _, e := range sec.entries {
			b.WriteString("  " + keyStyle.Render(pad(e.keys, 18)) + descStyle.Render(e.desc) + "\n")
		}
	}
	b.WriteString("\n" + styleFooter.Render("press any key to close"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colAccent).
		Padding(1, 3).
		Render(strings.TrimRight(b.String(), "\n"))
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}
