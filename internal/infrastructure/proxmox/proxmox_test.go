package proxmox

import (
	"strings"
	"testing"
)

func TestProvider_ZeroizeEnv(t *testing.T) {
	t.Run("secret keys blanked and slice nil after call", func(t *testing.T) {
		p := New(WithEnv([]string{
			"PROXMOX_VE_PASSWORD=hunter2",
			"PROXMOX_VE_API_TOKEN=tok-fake",
			"KUBECONFIG=/etc/kube",
		}))
		p.ZeroizeEnv()
		if p.env != nil {
			t.Errorf("env not nil after ZeroizeEnv; got %v", p.env)
		}
	})

	t.Run("secret entries blanked before clear, non-secret also zeroed", func(t *testing.T) {
		p := New(WithEnv([]string{
			"PROXMOX_VE_API_TOKEN=tok-fake",
			"KUBECONFIG=/etc/kube",
		}))
		snap := p.env
		p.ZeroizeEnv()
		if snap[0] != "" {
			t.Errorf("secret entry not blanked before clear; got %q", snap[0])
		}
		if snap[1] != "" {
			t.Errorf("non-secret entry not zeroed by clear; got %q", snap[1])
		}
	})

	t.Run("nil and empty env are no-ops", func(t *testing.T) {
		p1 := New()
		p1.ZeroizeEnv()

		p2 := New(WithEnv([]string{}))
		p2.ZeroizeEnv()
	})

	t.Run("idempotent second call", func(t *testing.T) {
		p := New(WithEnv([]string{"PROXMOX_VE_PASSWORD=hunter2"}))
		p.ZeroizeEnv()
		p.ZeroizeEnv()
		if p.env != nil {
			t.Errorf("env not nil after second ZeroizeEnv; got %v", p.env)
		}
	})

	t.Run("non-secret-keyed entries survive blanking pass but are cleared", func(t *testing.T) {
		p := New(WithEnv([]string{
			"PROXMOX_VE_ENDPOINT=https://pve.example.test:8006",
			"PROXMOX_VE_API_TOKEN=tok-fake",
		}))
		snap := p.env
		p.ZeroizeEnv()
		if strings.Contains(snap[0], "pve.example.test") {
			t.Errorf("non-secret entry not wiped by clear; got %q", snap[0])
		}
		if p.env != nil {
			t.Errorf("env not nil after ZeroizeEnv; got %v", p.env)
		}
	})
}
