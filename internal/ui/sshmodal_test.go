package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"sailor/internal/config"
	"sailor/internal/do"
)

func key(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

func TestSSHModalValidation(t *testing.T) {
	d := do.Droplet{Name: "web", PublicIP: "1.2.3.4"}

	// Empty key is allowed (ssh falls back to agent/config) -> connect.
	m := newSSHModal(d, config.Profile{User: "root", Key: ""})
	m, act, _ := m.update(key(tea.KeyEnter))
	if act != sshConnect {
		t.Fatalf("empty key should connect, got %v (err=%q)", act, m.errMsg)
	}

	// Missing key file -> blocked with an error, not a connect.
	m = newSSHModal(d, config.Profile{User: "root", Key: "/no/such/key"})
	m, act, _ = m.update(key(tea.KeyEnter))
	if act != sshNone || m.errMsg == "" {
		t.Fatalf("missing key should block, got act=%v err=%q", act, m.errMsg)
	}

	// Existing key file -> connect.
	kf := filepath.Join(t.TempDir(), "id_test")
	if err := os.WriteFile(kf, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	m = newSSHModal(d, config.Profile{User: "root", Key: kf})
	if _, act, _ = m.update(key(tea.KeyEnter)); act != sshConnect {
		t.Fatalf("existing key should connect, got %v", act)
	}

	// Esc cancels; Tab toggles focus.
	m = newSSHModal(d, config.Profile{})
	if _, act, _ = m.update(key(tea.KeyEsc)); act != sshCancel {
		t.Fatalf("esc should cancel, got %v", act)
	}
	m = newSSHModal(d, config.Profile{})
	if m2, _, _ := m.update(key(tea.KeyTab)); m2.focus != 1 {
		t.Fatalf("tab should move focus to key field, got %d", m2.focus)
	}
}

func TestStartSSHRouting(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	hosts, err := config.LoadHosts()
	if err != nil {
		t.Fatal(err)
	}
	base := Model{hosts: hosts, state: viewDetail}

	// No public IP -> transient error, no modal.
	m := base
	m.detail = newDetailModel(do.Droplet{ID: 1, Name: "x", PublicIP: ""}, 80, 24)
	res, _ := m.startSSH(false)
	if rm := res.(Model); rm.sshOpen || rm.sshErr == "" {
		t.Fatalf("no-IP should set error without opening modal: open=%v err=%q", rm.sshOpen, rm.sshErr)
	}

	// Has IP, no stored profile -> modal opens prefilled with defaults.
	m = base
	m.detail = newDetailModel(do.Droplet{ID: 2, Name: "y", PublicIP: "5.6.7.8"}, 80, 24)
	res, _ = m.startSSH(false)
	rm := res.(Model)
	if !rm.sshOpen || rm.ssh.user.Value() != "root" {
		t.Fatalf("expected modal with default user root, open=%v user=%q", rm.sshOpen, rm.ssh.user.Value())
	}

	// Has IP + stored profile, no forceModal -> connects immediately (no modal).
	if err := hosts.Set(3, config.Profile{User: "deploy", Key: ""}); err != nil {
		t.Fatal(err)
	}
	m = base
	m.detail = newDetailModel(do.Droplet{ID: 3, Name: "z", PublicIP: "9.9.9.9"}, 80, 24)
	res, cmd := m.startSSH(false)
	if rm := res.(Model); rm.sshOpen {
		t.Fatal("stored profile should connect without opening the modal")
	}
	if cmd == nil {
		t.Fatal("expected an ssh connect command")
	}
}
