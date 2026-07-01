package ui

import (
	"bufio"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/creack/pty"
	"github.com/rohittp0/sailor/internal/config"
	"github.com/rohittp0/sailor/internal/do"
)

// scpProgress is the latest parsed state of an in-flight upload. scp reports one
// meter per file, so pct/rate track the file currently being sent.
type scpProgress struct {
	file string
	pct  int
	rate string
}

// scpProgressMsg carries a meter update from the transfer goroutine to Update.
type scpProgressMsg scpProgress

// scpFinishedMsg signals the transfer ended (err nil on success).
type scpFinishedMsg struct{ err error }

// scpArgs builds the argument list for the system scp binary: recursive, using
// the Connection Profile's identity file (if any), uploading every path to the
// Droplet's remote home directory (user@ip:). It runs non-interactively because
// the PTY would otherwise let scp block on a host-key or passphrase prompt:
// unknown host keys are auto-added, and anything else fails fast (surfaced as a
// footer error) rather than hanging.
func scpArgs(d do.Droplet, p config.Profile, paths []string) []string {
	args := []string{"-r",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=yes",
	}
	if key := strings.TrimSpace(p.Key); key != "" {
		args = append(args, "-i", expandHome(key))
	}
	args = append(args, paths...)
	user := firstNonEmpty(p.User, "root")
	args = append(args, user+"@"+d.PublicIP+":")
	return args
}

// startScpCmd launches scp on a PTY and returns a tea.Cmd that reads the first
// event off ch. A goroutine scans the PTY master, parsing meter updates into
// scpProgressMsg and finishing with scpFinishedMsg. Progress is streamed into
// Update via the same channel + waitFor pattern the refresh pipeline uses.
func startScpCmd(ch chan tea.Msg, d do.Droplet, p config.Profile, paths []string) tea.Cmd {
	go runScp(ch, scpArgs(d, p, paths))
	return waitForScpEvent(ch)
}

func waitForScpEvent(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

// runScp execs scp attached to a PTY, streams parsed progress to ch, and sends
// a final scpFinishedMsg with the command's exit status.
func runScp(ch chan tea.Msg, args []string) {
	cmd := exec.Command("scp", args...)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		ch <- scpFinishedMsg{err: err}
		return
	}
	defer func() { _ = ptmx.Close() }()

	parseScpStream(ptmx, ch)
	ch <- scpFinishedMsg{err: cmd.Wait()}
}

// parseScpStream reads scp's PTY output, which updates the meter in place with
// carriage returns, and emits an scpProgressMsg for every parseable meter line.
func parseScpStream(r io.Reader, ch chan tea.Msg) {
	br := bufio.NewReader(r)
	var buf []byte
	flush := func() {
		if len(buf) == 0 {
			return
		}
		line := string(buf)
		buf = buf[:0]
		if file, pct, rate, ok := parseScpProgress(line); ok {
			ch <- scpProgressMsg{file: file, pct: pct, rate: rate}
		}
	}
	for {
		b, err := br.ReadByte()
		if err != nil {
			flush()
			return
		}
		if b == '\r' || b == '\n' {
			flush()
			continue
		}
		buf = append(buf, b)
	}
}

// parseScpProgress extracts the filename, percentage, and rate from one scp
// meter segment, e.g. "logo.png   45%  512KB  1.2MB/s   00:02". A valid meter
// line has a "NN%" token preceded by a filename and followed by a ".../s" rate
// token; anything else (warnings, banners) returns ok=false.
func parseScpProgress(line string) (file string, pct int, rate string, ok bool) {
	fields := strings.Fields(line)
	pctIdx := -1
	for i, f := range fields {
		if n, err := strconv.Atoi(strings.TrimSuffix(f, "%")); err == nil && strings.HasSuffix(f, "%") {
			pct, pctIdx = n, i
			break
		}
	}
	if pctIdx < 1 { // need at least one filename field before the percentage
		return "", 0, "", false
	}
	for _, f := range fields[pctIdx+1:] {
		if strings.Contains(f, "/s") {
			rate = f
			break
		}
	}
	if rate == "" {
		return "", 0, "", false
	}
	return strings.Join(fields[:pctIdx], " "), pct, rate, true
}

// scpProgressView renders the centered, Sailor-styled upload progress overlay:
// a gradient bar with the live percentage, then the current file and rate.
func scpProgressView(bar progress.Model, d do.Droplet, prog scpProgress, w, h int) string {
	barW := w - 12
	if barW < 20 {
		barW = 20
	}
	if barW > 60 {
		barW = 60
	}
	bar.Width = barW

	frac := float64(prog.pct) / 100
	title := lipgloss.NewStyle().Bold(true).Foreground(colAccent).Render("scp → " + d.Name)
	dest := styleFooter.Render(d.PublicIP + ":~")

	detail := styleDim.Render("starting…")
	if prog.file != "" {
		detail = styleName.Render(truncate(prog.file, barW)) + "  " + styleFooter.Render(prog.rate)
	}

	body := title + "\n" + dest + "\n\n" + bar.ViewAs(frac) + "\n" + detail

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colAccent).
		Padding(1, 2).
		Render(body)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}
