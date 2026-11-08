package netutil

import "testing"

func TestCalculateVMIP(t *testing.T) {
	cases := []struct {
		start   string
		index   int
		want    string
		wantErr bool
	}{
		{"192.168.1.10", 0, "192.168.1.10", false},
		{"192.168.1.10", 5, "192.168.1.15", false},
		{"192.168.1.255", 1, "192.168.2.0", false},
		{"255.255.255.254", 1, "255.255.255.255", false},
		{"255.255.255.255", 1, "", true},
		{"0.0.0.0", -1, "", true},
		{"not-an-ip", 0, "", true},
		{"2001:db8::1", 0, "", true},
	}
	for _, tc := range cases {
		got, err := CalculateVMIP(tc.start, tc.index)
		if tc.wantErr {
			if err == nil {
				t.Errorf("CalculateVMIP(%q, %d) = %q; want error", tc.start, tc.index, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("CalculateVMIP(%q, %d) unexpected error: %v", tc.start, tc.index, err)
			continue
		}
		if got != tc.want {
			t.Errorf("CalculateVMIP(%q, %d) = %q; want %q", tc.start, tc.index, got, tc.want)
		}
	}
}
