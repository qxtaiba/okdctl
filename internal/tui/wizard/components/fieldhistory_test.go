package components

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestFieldHistoryBoundsAndFilters(t *testing.T) {
	h := NewFieldHistoryFrom(map[string][]string{"basics/cluster_name": {"older", "newer"}, "proxmox/password": {"secret"}})
	for i := 0; i < 10; i++ {
		h.Record("basics/cluster_name", fmt.Sprintf("value-%d", i))
	}
	h.Record("basics/cluster_name", "value-9")
	h.Record("proxmox/ssh_key", "private-key")
	h.Record("basics/cluster_name", "ssh-ed25519 AAAA")
	h.Record("basics/domain", "-----BEGIN PRIVATE KEY-----")

	got := h.Values("basics/cluster_name")
	if len(got) != 8 || got[0] != "value-9" || got[7] != "value-2" {
		t.Fatalf("history = %v, want newest eight unique values", got)
	}
	if len(h.Values("proxmox/password")) != 0 || len(h.Values("proxmox/ssh_key")) != 0 {
		t.Fatal("sensitive field history was retained")
	}
	got[0] = "mutated"
	if h.Values("basics/cluster_name")[0] != "value-9" {
		t.Fatal("Values exposed the internal history slice")
	}
}

func TestInputFieldRecallsSafeHistory(t *testing.T) {
	h := NewFieldHistory(8)
	f := NewInputField("cluster name", "")
	f.SetFieldHistory("basics/cluster_name", h)
	f.Focus()
	f.SetValue("cluster-a")
	f.Blur()
	f.Focus()
	f.SetValue("cluster-b")
	f.Blur()
	f.Focus()
	f.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	f.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := f.Value(); got != "cluster-a" {
		t.Fatalf("ctrl+r value = %q, want prior value cluster-a", got)
	}
}
