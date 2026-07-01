package ui

import (
	"testing"

	"github.com/rohittp0/sailor/internal/config"
	"github.com/rohittp0/sailor/internal/do"
)

func TestParseScpProgress(t *testing.T) {
	cases := []struct {
		line     string
		wantFile string
		wantPct  int
		wantRate string
		wantOK   bool
	}{
		{"logo.png            45%  512KB   1.2MB/s   00:02", "logo.png", 45, "1.2MB/s", true},
		{"deploy.sh          100% 4096     4.0MB/s   00:00", "deploy.sh", 100, "4.0MB/s", true},
		{"my report.pdf       10%  128KB   256.0KB/s  01:15", "my report.pdf", 10, "256.0KB/s", true},
		// Non-meter output must be rejected.
		{"Warning: Permanently added '1.2.3.4' to the list of known hosts.", "", 0, "", false},
		{"", "", 0, "", false},
		{"Permission denied (publickey).", "", 0, "", false},
	}
	for _, c := range cases {
		file, pct, rate, ok := parseScpProgress(c.line)
		if ok != c.wantOK {
			t.Fatalf("parseScpProgress(%q) ok=%v, want %v", c.line, ok, c.wantOK)
		}
		if !ok {
			continue
		}
		if file != c.wantFile || pct != c.wantPct || rate != c.wantRate {
			t.Fatalf("parseScpProgress(%q) = (%q,%d,%q), want (%q,%d,%q)",
				c.line, file, pct, rate, c.wantFile, c.wantPct, c.wantRate)
		}
	}
}

func TestScpArgs(t *testing.T) {
	d := do.Droplet{PublicIP: "203.0.113.10"}

	// With a key: -r, non-interactive options, -i key, paths, then user@ip:.
	got := scpArgs(d, config.Profile{User: "deploy", Key: "/keys/id"}, []string{"/a/file", "/a/dir"})
	want := []string{
		"-r",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=yes",
		"-i", "/keys/id",
		"/a/file", "/a/dir",
		"deploy@203.0.113.10:",
	}
	if !equalStrings(got, want) {
		t.Fatalf("scpArgs with key = %v, want %v", got, want)
	}

	// No key -> no -i; empty user falls back to root.
	got = scpArgs(d, config.Profile{}, []string{"/x"})
	want = []string{
		"-r",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=yes",
		"/x",
		"root@203.0.113.10:",
	}
	if !equalStrings(got, want) {
		t.Fatalf("scpArgs no key = %v, want %v", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
