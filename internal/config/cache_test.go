package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sailor/internal/do"
)

func TestDropletCacheRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if _, ok := LoadDropletCache(); ok {
		t.Fatal("no cache file should be a miss")
	}

	want := []do.Droplet{
		{ID: 1, Name: "web", Status: "active", MemoryMB: 2048, DiskGB: 50, PublicIP: "1.2.3.4"},
		{ID: 2, Name: "db", Status: "off"},
	}
	if err := SaveDropletCache(want); err != nil {
		t.Fatal(err)
	}
	got, ok := LoadDropletCache()
	if !ok || len(got) != 2 || got[0].Name != "web" || got[1].Status != "off" {
		t.Fatalf("round-trip = %+v ok=%v, want the saved list", got, ok)
	}

	// File perms are 0600.
	path := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "sailor", "droplets.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("cache perm = %o, want 600", perm)
	}
}

func TestDropletCacheExpiry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg := filepath.Join(dir, "sailor")
	if err := os.MkdirAll(cfg, 0o700); err != nil {
		t.Fatal(err)
	}
	// Write a cache older than the TTL.
	stale := dropletCache{SavedAt: time.Now().Add(-2 * DropletCacheTTL), Droplets: []do.Droplet{{ID: 1}}}
	data, _ := json.Marshal(stale)
	if err := os.WriteFile(filepath.Join(cfg, "droplets.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := LoadDropletCache(); ok {
		t.Fatal("expired cache should be a miss")
	}
}
