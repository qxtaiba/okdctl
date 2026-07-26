package okd

import "testing"

func TestProvisionerZeroizeEnv(t *testing.T) {
	t.Run("blanks pendingEnv and drains the executor env", func(t *testing.T) {
		p := New(WithEnv([]string{
			"PROXMOX_VE_PASSWORD=hunter2",
			"PROXMOX_VE_API_TOKEN=tok123",
			"PROXMOX_VE_ENDPOINT=https://pve:8006",
		}))
		snap := p.pendingEnv

		p.ZeroizeEnv()

		if p.pendingEnv != nil {
			t.Errorf("pendingEnv not nil after ZeroizeEnv; got %v", p.pendingEnv)
		}
		for i, entry := range snap {
			if entry != "" {
				t.Errorf("pendingEnv backing entry %d survived ZeroizeEnv; got %q", i, entry)
			}
		}
		if got := p.executor.SnapshotEnv(); len(got) != 0 {
			t.Errorf("executor env not drained; got %v", got)
		}
	})

	t.Run("nil executor does not panic", func(t *testing.T) {
		p := &Provisioner{pendingEnv: []string{"PROXMOX_VE_PASSWORD=hunter2"}}
		p.ZeroizeEnv()
		if p.pendingEnv != nil {
			t.Errorf("pendingEnv not nil after ZeroizeEnv; got %v", p.pendingEnv)
		}
	})

	t.Run("empty pendingEnv is a no-op", func(_ *testing.T) {
		New().ZeroizeEnv()
	})

	t.Run("idempotent second call", func(t *testing.T) {
		p := New(WithEnv([]string{"PROXMOX_VE_PASSWORD=hunter2"}))
		p.ZeroizeEnv()
		p.ZeroizeEnv()
		if p.pendingEnv != nil {
			t.Errorf("pendingEnv not nil after repeated ZeroizeEnv; got %v", p.pendingEnv)
		}
	})
}
