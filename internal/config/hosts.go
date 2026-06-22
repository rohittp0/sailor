package config

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/BurntSushi/toml"
)

// Profile is a Droplet's remembered SSH settings. It holds the identity-file
// path, never key material — so the store contains no secrets.
type Profile struct {
	User string `toml:"user"`
	Key  string `toml:"key"`
}

// Hosts is the per-Droplet Connection Profile store, persisted to hosts.toml
// keyed by Droplet ID.
type Hosts struct {
	path string
	mu   sync.Mutex
	m    map[string]Profile
}

type hostsFile struct {
	Hosts map[string]Profile `toml:"hosts"`
}

// configDir returns ~/.config/sailor (honoring XDG_CONFIG_HOME), creating it.
func configDir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	dir := filepath.Join(base, "sailor")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// LoadHosts reads the Connection Profile store, returning an empty store when
// the file does not yet exist.
func LoadHosts() (*Hosts, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	h := &Hosts{path: filepath.Join(dir, "hosts.toml"), m: map[string]Profile{}}
	data, err := os.ReadFile(h.path)
	if os.IsNotExist(err) {
		return h, nil
	}
	if err != nil {
		return nil, err
	}
	var f hostsFile
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.Hosts != nil {
		h.m = f.Hosts
	}
	return h, nil
}

// Get returns the stored Profile for a Droplet, if any.
func (h *Hosts) Get(id int) (Profile, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	p, ok := h.m[strconv.Itoa(id)]
	return p, ok
}

// Set upserts a Droplet's Profile and persists the store (file mode 0600).
func (h *Hosts) Set(id int, p Profile) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.m[strconv.Itoa(id)] = p
	return h.save()
}

func (h *Hosts) save() error {
	tmp := h.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := toml.NewEncoder(f).Encode(hostsFile{Hosts: h.m}); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, h.path)
}

// DefaultKeyPath returns the first conventional private key found in ~/.ssh,
// or "" if none. Used to prefill the SSH prompt.
func DefaultKeyPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, name := range []string{"id_ed25519", "id_rsa", "id_ecdsa"} {
		p := filepath.Join(home, ".ssh", name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
