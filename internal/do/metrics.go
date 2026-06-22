package do

import (
	"sort"
	"time"

	"github.com/digitalocean/godo/metrics"
)

// Point is one (time, percentage) sample of a derived usage series.
type Point struct {
	T time.Time
	V float64
}

// Series holds the derived usage time series for one Droplet's expanded charts.
type Series struct {
	CPU  []Point
	Mem  []Point
	Disk []Point
}

// DeriveSeries builds the full CPU/mem/disk percentage series for the detail
// charts from a Droplet's plan totals and its fetched metric streams.
func DeriveSeries(d Droplet, cpu, memAvail, fsFree []metrics.SampleStream) Series {
	return Series{
		CPU:  cpuPercentSeries(cpu),
		Mem:  memPercentSeries(memAvail, d.MemoryMB),
		Disk: diskPercentSeries(fsFree, d.DiskGB),
	}
}

// DeriveUsage combines a Droplet's plan totals with its freshly-fetched metric
// series into the current Usage shown in the list. Off Droplets yield no
// metrics; an active Droplet with no memory/disk data is flagged StateNoAgent.
func DeriveUsage(d Droplet, cpu, memAvail, fsFree []metrics.SampleStream, now time.Time) Usage {
	u := Usage{State: StateOff, UpdatedAt: now}
	if !d.Active() {
		return u
	}
	u.CPU, u.CPUValid = cpuPercent(cpu)
	u.Mem, u.MemValid = memPercent(memAvail, d.MemoryMB)
	u.Disk, u.DiskValid = diskPercent(fsFree, d.DiskGB)
	if u.MemValid || u.DiskValid {
		u.State = StateActive
	} else {
		u.State = StateNoAgent
	}
	return u
}

// clampPct constrains a percentage to [0, 100].
func clampPct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// cpuPercentSeries derives a CPU-utilisation series from the per-mode
// cumulative counters returned by GetDropletCPU. For each adjacent pair of
// timestamps: util = 1 - Δidle/Δtotal, where Δtotal sums every mode. The point
// is stamped at the later timestamp. n samples yield n-1 points.
func cpuPercentSeries(streams []metrics.SampleStream) []Point {
	type acc struct{ idle, total float64 }
	byTS := map[int64]*acc{}
	for _, s := range streams {
		isIdle := s.Metric["mode"] == "idle"
		for _, p := range s.Values {
			ts := int64(p.Timestamp)
			a := byTS[ts]
			if a == nil {
				a = &acc{}
				byTS[ts] = a
			}
			a.total += float64(p.Value)
			if isIdle {
				a.idle += float64(p.Value)
			}
		}
	}
	if len(byTS) < 2 {
		return nil
	}
	tss := make([]int64, 0, len(byTS))
	for ts := range byTS {
		tss = append(tss, ts)
	}
	sort.Slice(tss, func(i, j int) bool { return tss[i] < tss[j] })

	out := make([]Point, 0, len(tss)-1)
	for i := 1; i < len(tss); i++ {
		a0, a1 := byTS[tss[i-1]], byTS[tss[i]]
		dTotal := a1.total - a0.total
		if dTotal <= 0 {
			continue
		}
		util := clampPct((1 - (a1.idle-a0.idle)/dTotal) * 100)
		out = append(out, Point{T: metrics.Time(tss[i]).Time(), V: util})
	}
	return out
}

// memPercentSeries derives a memory-usage series from the available-memory
// stream and the Droplet's total RAM (MB): used = (total - available)/total.
func memPercentSeries(available []metrics.SampleStream, totalMB int) []Point {
	if totalMB <= 0 || len(available) == 0 {
		return nil
	}
	total := float64(totalMB) * 1024 * 1024
	return ratioSeries(available[0], total)
}

// diskPercentSeries derives a root-filesystem usage series from the
// filesystem-free stream and the Droplet's plan disk (GB).
//
// Unit note: the agent reports bytes; we treat the plan disk as GiB. This is
// calibrated against the DO console during testing (see plan, milestone 1).
func diskPercentSeries(free []metrics.SampleStream, totalGB int) []Point {
	if totalGB <= 0 || len(free) == 0 {
		return nil
	}
	total := float64(totalGB) * 1024 * 1024 * 1024
	return ratioSeries(rootMount(free), total)
}

// ratioSeries turns a stream of "free/available" byte counts into a used-percent
// series given a fixed total.
func ratioSeries(s metrics.SampleStream, total float64) []Point {
	out := make([]Point, 0, len(s.Values))
	for _, p := range s.Values {
		out = append(out, Point{T: p.Timestamp.Time(), V: clampPct((total - float64(p.Value)) / total * 100)})
	}
	return out
}

func lastV(pts []Point) (float64, bool) {
	if len(pts) == 0 {
		return 0, false
	}
	return pts[len(pts)-1].V, true
}

// cpuPercent / memPercent / diskPercent return the latest point of each series,
// i.e. the current value shown in the list.
func cpuPercent(streams []metrics.SampleStream) (float64, bool) {
	return lastV(cpuPercentSeries(streams))
}
func memPercent(available []metrics.SampleStream, totalMB int) (float64, bool) {
	return lastV(memPercentSeries(available, totalMB))
}
func diskPercent(free []metrics.SampleStream, totalGB int) (float64, bool) {
	return lastV(diskPercentSeries(free, totalGB))
}

// rootMount picks the SampleStream for the root filesystem, falling back to the
// first stream when no mount label identifies "/".
func rootMount(streams []metrics.SampleStream) metrics.SampleStream {
	for _, s := range streams {
		for _, k := range []string{"mountpoint", "device", "fstype", "filesystem"} {
			if s.Metric[metrics.LabelName(k)] == "/" {
				return s
			}
		}
	}
	return streams[0]
}
