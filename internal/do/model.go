// Package do is the data layer over the DigitalOcean API: listing Droplets and
// deriving current CPU/memory/disk usage from the Monitoring API.
package do

import "time"

// MetricState explains why a Droplet's metrics are or aren't available.
type MetricState int

const (
	// StateActive: Droplet is active and the metrics agent is installed —
	// CPU, memory and disk are all available.
	StateActive MetricState = iota
	// StateNoAgent: Droplet is active but has no metrics agent — only CPU is
	// available; memory and disk are "n/a".
	StateNoAgent
	// StateOff: Droplet is off/non-active — no metrics; calls are skipped.
	StateOff
)

// Droplet is the subset of a DigitalOcean Droplet that Sailor cares about.
// Memory is in MB and Disk is in GB (the plan totals, free from the list).
type Droplet struct {
	ID       int
	Name     string
	Status   string // "new", "active", "off", "archive"
	MemoryMB int
	DiskGB   int
	Vcpus    int
	SizeSlug string
	PublicIP string // resolved public IPv4, "" if none
}

// Active reports whether the Droplet is in a state that can report metrics.
func (d Droplet) Active() bool { return d.Status == "active" }

// Usage is the derived "current" usage for a Droplet, as shown in the list.
// A percentage is only meaningful when its *Valid flag is set.
type Usage struct {
	CPU, Mem, Disk float64 // 0..100
	CPUValid       bool
	MemValid       bool
	DiskValid      bool
	State          MetricState
	UpdatedAt      time.Time
}
