package main

import (
	"context"
	"fmt"
	"time"

	"github.com/digitalocean/godo/metrics"
	"sailor/internal/do"
)

// runProbe dumps raw metric responses for the first few active Droplets so we
// can diagnose why derivation yields n/a. Invoked via `sailor --probe`.
func runProbe(client *do.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	droplets, err := client.ListDroplets(ctx)
	if err != nil {
		fmt.Println("list error:", err)
		return
	}
	fmt.Printf("listed %d droplets\n", len(droplets))

	end := time.Now()
	start := end.Add(-30 * time.Minute)
	fmt.Printf("window: %s … %s\n\n", start.Format(time.RFC3339), end.Format(time.RFC3339))

	shown := 0
	for _, d := range droplets {
		if !d.Active() {
			continue
		}
		id := do.IDString(d.ID)
		fmt.Printf("== droplet %d %q (mem=%dMB disk=%dGB ip=%s) ==\n", d.ID, d.Name, d.MemoryMB, d.DiskGB, d.PublicIP)

		cpu, errC := client.CPUSeries(ctx, id, start, end)
		dump("cpu", cpu, errC)
		mem, errM := client.MemAvailableSeries(ctx, id, start, end)
		dump("memory_available", mem, errM)
		fs, errF := client.FSFreeSeries(ctx, id, start, end)
		dump("filesystem_free", fs, errF)

		u := do.DeriveUsage(d, cpu, mem, fs, time.Now())
		fmt.Printf("  derived: state=%d cpu=%.1f(%v) mem=%.1f(%v) disk=%.1f(%v)\n\n",
			u.State, u.CPU, u.CPUValid, u.Mem, u.MemValid, u.Disk, u.DiskValid)

		shown++
		if shown >= 3 {
			break
		}
	}
	if shown == 0 {
		fmt.Println("no active droplets to probe")
	}
}

func dump(name string, streams []metrics.SampleStream, err error) {
	if err != nil {
		fmt.Printf("  %-18s ERROR: %v\n", name, err)
		return
	}
	fmt.Printf("  %-18s %d stream(s)\n", name, len(streams))
	for i, s := range streams {
		last := "—"
		if n := len(s.Values); n > 0 {
			last = fmt.Sprintf("%v @ %s (n=%d)", s.Values[n-1].Value, s.Values[n-1].Timestamp.Time().Format("15:04:05"), n)
		}
		fmt.Printf("      [%d] labels=%v last=%s\n", i, s.Metric, last)
	}
}
