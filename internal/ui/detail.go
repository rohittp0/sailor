package ui

import (
	"fmt"
	"time"

	"github.com/NimbleMarkets/ntcharts/linechart/timeserieslinechart"
	"github.com/charmbracelet/lipgloss"
	"sailor/internal/do"
)

// detailModel is the full-screen expanded view: three stacked time-series
// charts (CPU / memory / disk) for a single Droplet.
type detailModel struct {
	d       do.Droplet
	series  do.Series
	hasData bool
	loading bool
	errMsg  string
	width   int
	height  int
}

func newDetailModel(d do.Droplet, w, h int) detailModel {
	return detailModel{d: d, loading: true, width: w, height: h}
}

func (m *detailModel) setSize(w, h int) { m.width, m.height = w, h }
func (m *detailModel) setSeries(s do.Series) {
	m.series, m.hasData, m.loading, m.errMsg = s, true, false, ""
}
func (m *detailModel) setErr(msg string) { m.loading, m.errMsg = false, msg }

func windowLabel(d time.Duration) string {
	switch d {
	case time.Hour:
		return "1h"
	case 6 * time.Hour:
		return "6h"
	case 24 * time.Hour:
		return "24h"
	default:
		return d.String()
	}
}

func (m *detailModel) view(window time.Duration, now time.Time) string {
	header := styleHeader.Render("SAILOR ▸ "+m.d.Name) + "   " +
		styleFooter.Render(fmt.Sprintf("%s · %d vCPU · %d MB · %d GB    window: %s",
			m.d.Status, m.d.Vcpus, m.d.MemoryMB, m.d.DiskGB, windowLabel(window)))
	footer := styleFooter.Render(detailFooterHint)

	var body string
	switch {
	case m.errMsg != "":
		body = lipgloss.NewStyle().Foreground(colOrange).Render(m.errMsg)
	case m.loading && !m.hasData:
		body = styleFooter.Render("Loading metrics…")
	default:
		body = m.charts(window, now)
	}
	return header + "\n" + body + "\n" + footer
}

// charts renders the three stacked panels sized to the available height.
func (m *detailModel) charts(window time.Duration, now time.Time) string {
	avail := m.height - 2 // header + footer
	per := avail / 3
	chartH := per - 1 // title line per panel
	if chartH < 2 {
		chartH = 2
	}
	w := m.width - 1
	if w < 10 {
		w = 10
	}
	panels := []struct {
		label string
		pts   []do.Point
	}{
		{"CPU", m.series.CPU},
		{"MEM", m.series.Mem},
		{"DISK", m.series.Disk},
	}
	out := ""
	for i, p := range panels {
		if i > 0 {
			out += "\n"
		}
		out += renderChart(p.label, p.pts, window, now, w, chartH)
	}
	return out
}

func renderChart(label string, pts []do.Point, window time.Duration, now time.Time, w, h int) string {
	cur, peak, avg := stats(pts)
	curLabel := "n/a"
	statLabel := ""
	color := colDim
	if len(pts) > 0 {
		curLabel = fmt.Sprintf("%.0f%%", cur)
		statLabel = fmt.Sprintf("  (peak %.0f%%  avg %.0f%%)", peak, avg)
		color = usageColor(cur)
	}
	title := lipgloss.NewStyle().Bold(true).Render(pad(label, 5)) +
		lipgloss.NewStyle().Foreground(color).Render(curLabel) +
		styleFooter.Render(statLabel)

	if len(pts) == 0 {
		return title + "\n" + styleDim.Render("  no data in window")
	}

	ch := timeserieslinechart.New(w, h)
	ch.SetViewTimeAndYRange(now.Add(-window), now, 0, 100)
	ch.SetStyle(lipgloss.NewStyle().Foreground(color))
	for _, p := range pts {
		ch.Push(timeserieslinechart.TimePoint{Time: p.T, Value: p.V})
	}
	ch.DrawBraille()
	return title + "\n" + ch.View()
}

func stats(pts []do.Point) (cur, peak, avg float64) {
	if len(pts) == 0 {
		return 0, 0, 0
	}
	var sum float64
	for _, p := range pts {
		sum += p.V
		if p.V > peak {
			peak = p.V
		}
	}
	return pts[len(pts)-1].V, peak, sum / float64(len(pts))
}
