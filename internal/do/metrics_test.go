package do

import (
	"math"
	"testing"

	"github.com/digitalocean/godo/metrics"
)

// cpuStream builds a single-mode CPU SampleStream of cumulative counter values
// at successive 1-second-apart timestamps starting at `base`.
func cpuStream(mode string, base int64, vals ...float64) metrics.SampleStream {
	pairs := make([]metrics.SamplePair, len(vals))
	for i, v := range vals {
		pairs[i] = metrics.SamplePair{
			Timestamp: metrics.TimeFromUnix(base + int64(i)*60),
			Value:     metrics.SampleValue(v),
		}
	}
	return metrics.SampleStream{Metric: metrics.Metric{"mode": metrics.LabelValue(mode)}, Values: pairs}
}

func approx(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestCPUPercent(t *testing.T) {
	tests := []struct {
		name    string
		streams []metrics.SampleStream
		want    float64
		ok      bool
	}{
		{
			name: "50 percent busy",
			// Between the two samples: idle +30, user +20, system +10 -> total +60,
			// idle delta 30 => util = 1 - 30/60 = 50%.
			streams: []metrics.SampleStream{
				cpuStream("idle", 1000, 100, 130),
				cpuStream("user", 1000, 200, 220),
				cpuStream("system", 1000, 50, 60),
			},
			want: 50, ok: true,
		},
		{
			name: "fully idle",
			streams: []metrics.SampleStream{
				cpuStream("idle", 1000, 100, 160),
				cpuStream("user", 1000, 10, 10),
			},
			want: 0, ok: true,
		},
		{
			name: "fully busy",
			streams: []metrics.SampleStream{
				cpuStream("idle", 1000, 100, 100),
				cpuStream("user", 1000, 10, 70),
			},
			want: 100, ok: true,
		},
		{
			name:    "single sample insufficient",
			streams: []metrics.SampleStream{cpuStream("idle", 1000, 100)},
			want:    0, ok: false,
		},
		{
			name:    "empty",
			streams: nil,
			want:    0, ok: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := cpuPercent(tt.streams)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && !approx(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCPUPercentSeries(t *testing.T) {
	// 4 samples -> 3 utilisation points.
	streams := []metrics.SampleStream{
		cpuStream("idle", 1000, 100, 130, 190, 220), // +30, +60, +30
		cpuStream("user", 1000, 200, 230, 230, 260), // +30, +0, +30
	}
	// totals delta: 60,60,60 ; idle delta: 30,60,30 -> util 50%,0%,50%.
	got := cpuPercentSeries(streams)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	want := []float64{50, 0, 50}
	for i, p := range got {
		if !approx(p.V, want[i]) {
			t.Fatalf("point %d = %v, want %v", i, p.V, want[i])
		}
	}
	// Timestamps are strictly increasing.
	for i := 1; i < len(got); i++ {
		if !got[i].T.After(got[i-1].T) {
			t.Fatalf("timestamps not increasing at %d", i)
		}
	}
}

func TestMemDiskSeries(t *testing.T) {
	const mb = 1024 * 1024
	mem := memPercentSeries(memStream(1000, 512*mb, 256*mb), 1024) // 50%, 75%
	if len(mem) != 2 || !approx(mem[0].V, 50) || !approx(mem[1].V, 75) {
		t.Fatalf("mem series = %+v, want [50 75]", mem)
	}
	const gib = 1024 * 1024 * 1024
	disk := diskPercentSeries([]metrics.SampleStream{fsStream("/", 1000, 50*gib, 25*gib)}, 100)
	if len(disk) != 2 || !approx(disk[0].V, 50) || !approx(disk[1].V, 75) {
		t.Fatalf("disk series = %+v, want [50 75]", disk)
	}
}

func memStream(base int64, vals ...float64) []metrics.SampleStream {
	pairs := make([]metrics.SamplePair, len(vals))
	for i, v := range vals {
		pairs[i] = metrics.SamplePair{Timestamp: metrics.TimeFromUnix(base + int64(i)*60), Value: metrics.SampleValue(v)}
	}
	return []metrics.SampleStream{{Values: pairs}}
}

func TestMemPercent(t *testing.T) {
	const mb = 1024 * 1024
	// total = 1024 MB = 1073741824 bytes; available 268435456 (256MB) -> used 75%.
	got, ok := memPercent(memStream(1000, 512*mb, 256*mb), 1024)
	if !ok || !approx(got, 75) {
		t.Fatalf("got %v ok=%v, want 75 true", got, ok)
	}
	if _, ok := memPercent(nil, 1024); ok {
		t.Fatal("empty series should be invalid")
	}
	if _, ok := memPercent(memStream(1000, 1*mb), 0); ok {
		t.Fatal("zero total should be invalid")
	}
}

func fsStream(mount string, base int64, vals ...float64) metrics.SampleStream {
	pairs := make([]metrics.SamplePair, len(vals))
	for i, v := range vals {
		pairs[i] = metrics.SamplePair{Timestamp: metrics.TimeFromUnix(base + int64(i)*60), Value: metrics.SampleValue(v)}
	}
	return metrics.SampleStream{Metric: metrics.Metric{"mountpoint": metrics.LabelValue(mount)}, Values: pairs}
}

func TestDiskPercent(t *testing.T) {
	const gib = 1024 * 1024 * 1024
	// Prefer the root mount. total = 100 GiB; free on / is 25 GiB -> used 75%.
	streams := []metrics.SampleStream{
		fsStream("/boot", 1000, 1*gib, 1*gib),
		fsStream("/", 1000, 30*gib, 25*gib),
	}
	got, ok := diskPercent(streams, 100)
	if !ok || !approx(got, 75) {
		t.Fatalf("got %v ok=%v, want 75 true (root mount)", got, ok)
	}
	if _, ok := diskPercent(nil, 100); ok {
		t.Fatal("empty series should be invalid")
	}
	// free exceeding total (unit mismatch) clamps to 0, stays valid.
	clamped, ok := diskPercent([]metrics.SampleStream{fsStream("/", 1000, 200*gib, 200*gib)}, 100)
	if !ok || clamped != 0 {
		t.Fatalf("expected clamp to 0, got %v ok=%v", clamped, ok)
	}
}
