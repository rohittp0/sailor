package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHostsRoundTripUpsertPerms(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	h, err := LoadHosts()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := h.Get(42); ok {
		t.Fatal("empty store should not have profiles")
	}

	if err := h.Set(42, Profile{User: "root", Key: "/home/x/.ssh/id_ed25519"}); err != nil {
		t.Fatal(err)
	}
	// Upsert: overwrite the same droplet.
	if err := h.Set(42, Profile{User: "deploy", Key: "/home/x/.ssh/id_rsa"}); err != nil {
		t.Fatal(err)
	}

	// File perms must be 0600.
	path := filepath.Join(dir, "sailor", "hosts.toml")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file perm = %o, want 600", perm)
	}

	// Reload from disk and confirm the upserted value persisted.
	h2, err := LoadHosts()
	if err != nil {
		t.Fatal(err)
	}
	p, ok := h2.Get(42)
	if !ok || p.User != "deploy" || p.Key != "/home/x/.ssh/id_rsa" {
		t.Fatalf("reloaded profile = %+v ok=%v, want deploy/id_rsa", p, ok)
	}
}

func TestLoadHostsMissingFileIsEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	h, err := LoadHosts()
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if _, ok := h.Get(1); ok {
		t.Fatal("expected empty store")
	}
}
