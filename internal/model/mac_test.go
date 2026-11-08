package model

import "testing"

func TestMAC(t *testing.T) {
	for _, tc := range []struct {
		infraID string
		vmid    int
		want    string
	}{
		{"lab-x7k2q", 6010, "02:12:83:00:17:7a"},
		{"lab-x7k2q", 6103, "02:12:83:00:17:d7"},
		{"lab-x7k2q", 0xFFFFFF, "02:12:83:ff:ff:ff"},
	} {
		if got := MAC(tc.infraID, tc.vmid); got != tc.want {
			t.Errorf("MAC(%q, %d) = %q; want %q", tc.infraID, tc.vmid, got, tc.want)
		}
	}
}

func TestDiskSerial(t *testing.T) {
	if got := DiskSerial(6103, 1); got != "okd6103d1" {
		t.Errorf("DiskSerial(6103, 1) = %q; want okd6103d1", got)
	}
}
