package config

import (
	"net/netip"
	"slices"
	"testing"
)

func TestParseNodeKey(t *testing.T) {
	cfg := DefaultConfig()
	cases := []struct {
		key   string
		role  Role
		index int
		ok    bool
	}{
		{"lab-master0", RoleMaster, 0, true},
		{"lab-master2", RoleMaster, 2, true},
		{"lab-master3", "", 0, false},
		{"lab-worker0", RoleWorker, 0, true},
		{"lab-worker17", RoleWorker, 17, true},
		{"lab-worker01", "", 0, false},
		{"lab-worker-1", "", 0, false},
		{"lab-worker", "", 0, false},
		{"worker0", "", 0, false},
		{"other-master0", "", 0, false},
	}
	for _, tc := range cases {
		role, index, ok := cfg.ParseNodeKey(tc.key)
		if role != tc.role || index != tc.index || ok != tc.ok {
			t.Errorf("ParseNodeKey(%q) = %q, %d, %v; want %q, %d, %v", tc.key, role, index, ok, tc.role, tc.index, tc.ok)
		}
	}
}

func TestWorkerIndices(t *testing.T) {
	cases := []struct {
		name  string
		count int
		keys  []string
		want  []int
	}{
		{"fills from zero", 2, nil, []int{0, 1}},
		{"no workers", 0, nil, nil},
		{"negative count is empty", -3, []string{"lab-worker0"}, nil},
		{"keeps the named hole", 2, []string{"lab-worker0", "lab-worker2"}, []int{0, 2}},
		{"fills the hole when count grows", 3, []string{"lab-worker0", "lab-worker2"}, []int{0, 1, 2}},
		{"ignores masters and foreign keys", 1, []string{"lab-master1", "other-worker5"}, []int{0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Workers.Count = tc.count
			cfg.Nodes = map[string]NodeOverride{}
			for _, k := range tc.keys {
				cfg.Nodes[k] = NodeOverride{}
			}
			if got := cfg.WorkerIndices(); !slices.Equal(got, tc.want) {
				t.Errorf("WorkerIndices = %v; want %v", got, tc.want)
			}
		})
	}
}

func TestNodeLayout(t *testing.T) {
	cfg := DefaultConfig()
	cases := []struct {
		role  Role
		index int
		name  string
		vmid  int
		addr  string
	}{
		{RoleMaster, 0, "lab-master0", 6010, "192.168.1.150"},
		{RoleMaster, 2, "lab-master2", 6012, "192.168.1.152"},
		{RoleWorker, 0, "lab-worker0", 6100, "192.168.1.160"},
		{RoleWorker, 4, "lab-worker4", 6104, "192.168.1.164"},
	}
	for _, tc := range cases {
		if got := cfg.NodeName(tc.role, tc.index); got != tc.name {
			t.Errorf("NodeName(%s, %d) = %q; want %q", tc.role, tc.index, got, tc.name)
		}
		if got := cfg.NodeVMID(tc.role, tc.index); got != tc.vmid {
			t.Errorf("NodeVMID(%s, %d) = %d; want %d", tc.role, tc.index, got, tc.vmid)
		}
		got, err := cfg.DerivedNodeAddress(tc.role, tc.index)
		if err != nil || got != netip.MustParseAddr(tc.addr) {
			t.Errorf("DerivedNodeAddress(%s, %d) = %s, %v; want %s", tc.role, tc.index, got, err, tc.addr)
		}
	}
	cfg.Networking.NodeIPStart = "nope"
	if _, err := cfg.DerivedNodeAddress(RoleMaster, 0); err == nil {
		t.Error("DerivedNodeAddress accepted an unparsable node_ip_start")
	}
}
