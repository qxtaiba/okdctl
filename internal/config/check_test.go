package config

import "testing"

func TestCheckers(t *testing.T) {
	cases := []struct {
		name  string
		check func(string) error
		good  string
		bad   string
	}{
		{"cluster name", CheckClusterName, "my-lab-1", "Lab.prod"},
		{"domain", CheckDomain, "okd.lab.example.com", "k8s_.local"},
		{"endpoint", CheckEndpoint, "https://pve:8006", "pve"},
		{"cpu type", CheckCPUType, "x86-64-v3", "x86-64-v2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.check(tc.good); err != nil {
				t.Errorf("%s(%q) = %v; want nil", tc.name, tc.good, err)
			}
			if err := tc.check(tc.bad); err == nil {
				t.Errorf("%s(%q) accepted", tc.name, tc.bad)
			}
			if err := tc.check(""); err == nil {
				t.Errorf("%s(\"\") accepted", tc.name)
			}
		})
	}
}

func TestFloorFor(t *testing.T) {
	if got := FloorFor(RoleMaster); got != (Floor{CPU: 4, MemoryMiB: 16384, RootGiB: 100}) {
		t.Errorf("FloorFor(master) = %+v", got)
	}
	if got := FloorFor(RoleWorker); got != (Floor{CPU: 2, MemoryMiB: 8192, RootGiB: 100}) {
		t.Errorf("FloorFor(worker) = %+v", got)
	}
}
