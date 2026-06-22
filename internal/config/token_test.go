package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTokenEnvWins(t *testing.T) {
	t.Setenv(EnvVar, "  env-token  ")
	// Even with a doctl config present, env should win.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	got, err := Token()
	if err != nil || got != "env-token" {
		t.Fatalf("got %q err=%v, want env-token", got, err)
	}
}

func TestTokenFromDoctlConfig(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "default context uses top-level token",
			yaml: "access-token: dop_default\ncontext: default\n",
			want: "dop_default",
		},
		{
			name: "no context field uses top-level token",
			yaml: "access-token: dop_top\n",
			want: "dop_top",
		},
		{
			name: "named context uses auth-contexts",
			yaml: "access-token: dop_default\ncontext: work\nauth-contexts:\n  work: dop_work\n",
			want: "dop_work",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tokenFromDoctlConfig([]byte(tt.yaml))
			if err != nil || got != tt.want {
				t.Fatalf("got %q err=%v, want %q", got, err, tt.want)
			}
		})
	}
}

func TestTokenReadsConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "doctl")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "config.yaml"), []byte("access-token: dop_file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvVar, "")
	t.Setenv("XDG_CONFIG_HOME", dir)
	got, err := Token()
	if err != nil || got != "dop_file" {
		t.Fatalf("got %q err=%v, want dop_file", got, err)
	}
}

func TestTokenMissing(t *testing.T) {
	t.Setenv(EnvVar, "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// Point HOME and UserConfigDir at empty temp dirs so no real config leaks in.
	t.Setenv("HOME", t.TempDir())
	if _, err := Token(); err != ErrNoToken {
		t.Fatalf("err = %v, want ErrNoToken", err)
	}
}
