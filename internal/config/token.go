// Package config resolves the DigitalOcean API token and stores per-Droplet
// SSH connection profiles.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	yaml "gopkg.in/yaml.v3"
)

// ErrNoToken is returned when no token can be found in the environment or in a
// doctl config. main renders this as the missing-token screen.
var ErrNoToken = errors.New("no DigitalOcean API token found")

// EnvVar is the environment variable checked first (godo's convention).
const EnvVar = "DIGITALOCEAN_ACCESS_TOKEN"

// Token resolves the API token: the environment variable first, then a doctl
// config, else ErrNoToken.
func Token() (string, error) {
	if t := strings.TrimSpace(os.Getenv(EnvVar)); t != "" {
		return t, nil
	}
	for _, p := range doctlConfigPaths() {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if t, err := tokenFromDoctlConfig(data); err == nil && t != "" {
			return t, nil
		}
	}
	return "", ErrNoToken
}

// doctlConfigPaths lists candidate doctl config locations, most-specific first.
func doctlConfigPaths() []string {
	var paths []string
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		paths = append(paths, filepath.Join(xdg, "doctl", "config.yaml"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "doctl", "config.yaml"))
	}
	if ucd, err := os.UserConfigDir(); err == nil { // macOS: ~/Library/Application Support
		paths = append(paths, filepath.Join(ucd, "doctl", "config.yaml"))
	}
	return paths
}

// doctlConfig mirrors the fields of doctl's config.yaml we care about.
type doctlConfig struct {
	AccessToken  string            `yaml:"access-token"`
	Context      string            `yaml:"context"`
	AuthContexts map[string]string `yaml:"auth-contexts"`
}

// tokenFromDoctlConfig extracts the access token for the current context.
func tokenFromDoctlConfig(data []byte) (string, error) {
	var c doctlConfig
	if err := yaml.Unmarshal(data, &c); err != nil {
		return "", err
	}
	ctx := strings.TrimSpace(c.Context)
	if ctx == "" || ctx == "default" {
		return strings.TrimSpace(c.AccessToken), nil
	}
	return strings.TrimSpace(c.AuthContexts[ctx]), nil
}
