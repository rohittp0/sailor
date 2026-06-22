package ui

import (
	"testing"
	"time"

	"sailor/internal/do"
)

func sampleSeries(now time.Time) do.Series {
	var cpu, mem, disk []do.Point
	for i := 30; i >= 0; i-- {
		t := now.Add(-time.Duration(i) * time.Minute)
		cpu = append(cpu, do.Point{T: t, V: float64(40 + i%20)})
		mem = append(mem, do.Point{T: t, V: float64(60 + i%10)})
		disk = append(disk, do.Point{T: t, V: float64(45)})
	}
	return do.Series{CPU: cpu, Mem: mem, Disk: disk}
}

func TestDetailViewRenders(t *testing.T) {
	now := time.Now()
	d := do.Droplet{ID: 1, Name: "nyc1-web-01", Status: "active", Vcpus: 4, MemoryMB: 8192, DiskGB: 160}

	for _, size := range [][2]int{{120, 30}, {80, 24}, {40, 12}} {
		m := newDetailModel(d, size[0], size[1])
		m.setSeries(sampleSeries(now))
		out := m.view(time.Hour, now) // must not panic at any size
		if !contains(out, "SAILOR") || !contains(out, "nyc1-web-01") {
			t.Fatalf("detail view at %v missing header", size)
		}
		if !contains(out, "CPU") || !contains(out, "MEM") || !contains(out, "DISK") {
			t.Fatalf("detail view at %v missing a chart label", size)
		}
	}
}

func TestDetailLoadingAndError(t *testing.T) {
	now := time.Now()
	d := do.Droplet{ID: 1, Name: "box", Status: "active", Vcpus: 1, MemoryMB: 1024, DiskGB: 25}

	loading := newDetailModel(d, 100, 24)
	if !contains(loading.view(time.Hour, now), "Loading") {
		t.Fatal("expected loading placeholder before data arrives")
	}

	errd := newDetailModel(d, 100, 24)
	errd.setErr("droplet is off — no metrics available")
	if !contains(errd.view(time.Hour, now), "no metrics") {
		t.Fatal("expected error message in detail view")
	}
}

func TestWindowLabel(t *testing.T) {
	cases := map[time.Duration]string{time.Hour: "1h", 6 * time.Hour: "6h", 24 * time.Hour: "24h"}
	for d, want := range cases {
		if got := windowLabel(d); got != want {
			t.Fatalf("windowLabel(%v) = %q, want %q", d, got, want)
		}
	}
}
