package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"sailor/internal/do"
)

// Color palette (btop-style thresholds).
var (
	colGreen  = lipgloss.Color("42")
	colYellow = lipgloss.Color("220")
	colOrange = lipgloss.Color("208")
	colRed    = lipgloss.Color("196")
	colDim    = lipgloss.Color("240")
	colAccent = lipgloss.Color("39")
	colText   = lipgloss.Color("252")
)

var (
	styleHeader   = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	styleColHead  = lipgloss.NewStyle().Bold(true).Foreground(colDim)
	styleFooter   = lipgloss.NewStyle().Foreground(colDim)
	styleSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("236"))
	styleDim      = lipgloss.NewStyle().Foreground(colDim)
	styleName     = lipgloss.NewStyle().Foreground(colText)
)

// usageColor maps a usage percentage to its threshold color.
func usageColor(p float64) lipgloss.Color {
	switch {
	case p >= 90:
		return colRed
	case p >= 75:
		return colOrange
	case p >= 50:
		return colYellow
	default:
		return colGreen
	}
}

// statusGlyph returns the colored status indicator for a row's metric state.
func statusGlyph(state do.MetricState, status string) string {
	switch state {
	case do.StateActive:
		return lipgloss.NewStyle().Foreground(colGreen).Render("●")
	case do.StateNoAgent:
		return lipgloss.NewStyle().Foreground(colYellow).Render("◐")
	default: // off / non-active
		if status == "new" || status == "archive" {
			return lipgloss.NewStyle().Foreground(colDim).Render("⊘")
		}
		return lipgloss.NewStyle().Foreground(colRed).Render("○")
	}
}

const barWidth = 10

// renderMetric renders a usage cell: a colored bar + percentage, or a dim
// placeholder ("n/a" for no-agent, "--" for off) when the value is invalid.
func renderMetric(pct float64, valid bool, state do.MetricState, stale bool) string {
	if !valid {
		if state == do.StateOff {
			return styleDim.Render(pad("--", barWidth+5))
		}
		return styleDim.Render(pad("n/a", barWidth+5))
	}
	filled := int(pct/100*barWidth + 0.5)
	if filled > barWidth {
		filled = barWidth
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	color := usageColor(pct)
	if stale {
		color = colDim
	}
	cell := lipgloss.NewStyle().Foreground(color).Render(bar) + fmt.Sprintf(" %3.0f%%", pct)
	return cell
}

// loadingCell renders a placeholder for a metric whose stats are still loading.
func loadingCell() string {
	return styleDim.Render(pad("loading…", barWidth+5))
}

// pad right-pads (or truncates) s to width visible columns.
func pad(s string, width int) string {
	w := lipgloss.Width(s)
	if w > width {
		return truncate(s, width)
	}
	return s + strings.Repeat(" ", width-w)
}

func truncate(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	if width <= 1 {
		return "…"
	}
	// Trim runes until it fits, leaving room for the ellipsis.
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > width {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}
