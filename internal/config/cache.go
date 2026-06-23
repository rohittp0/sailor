package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/rohittp0/sailor/internal/do"
)

// DropletCacheTTL is how long a cached Droplet list is trusted for an instant
// launch. Stats are never cached — only the list (names, sizes, status, IPs).
const DropletCacheTTL = 24 * time.Hour

type dropletCache struct {
	SavedAt  time.Time    `json:"saved_at"`
	Droplets []do.Droplet `json:"droplets"`
}

func dropletCachePath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "droplets.json"), nil
}

// LoadDropletCache returns the cached Droplet list if present and younger than
// the TTL. ok is false on any miss (no file, parse error, expired, or empty).
func LoadDropletCache() (droplets []do.Droplet, ok bool) {
	p, err := dropletCachePath()
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	var c dropletCache
	if json.Unmarshal(data, &c) != nil {
		return nil, false
	}
	if time.Since(c.SavedAt) > DropletCacheTTL || len(c.Droplets) == 0 {
		return nil, false
	}
	return c.Droplets, true
}

// SaveDropletCache persists the Droplet list (best-effort, file mode 0600).
func SaveDropletCache(droplets []do.Droplet) error {
	p, err := dropletCachePath()
	if err != nil {
		return err
	}
	data, err := json.Marshal(dropletCache{SavedAt: time.Now(), Droplets: droplets})
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
