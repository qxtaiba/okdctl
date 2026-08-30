package netutil

import "testing"

func TestCIDRToNetmask(t *testing.T) {
	cases := []struct {
		cidr    string
		want    string
		wantErr bool
	}{
		{"0.0.0.0/0", "0.0.0.0", false},
		{"0.0.0.0/1", "128.0.0.0", false},
		{"192.168.1.0/24", "255.255.255.0", false},
		{"10.0.0.1/32", "255.255.255.255", false},
		{"2001:db8::/32", "", true},
		{"invalid", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.cidr, func(t *testing.T) {
			got, err := CIDRToNetmask(tc.cidr)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

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

func TestValidateIPRangeInCIDR(t *testing.T) {
	cases := []struct {
		name    string
		start   string
		count   int
		cidr    string
		wantErr bool
	}{
		{"range fits inside /24", "192.168.1.10", 6, "192.168.1.0/24", false},
		{"last IP exactly at boundary", "192.168.1.250", 6, "192.168.1.0/24", false},
		{"last IP beyond /24 boundary", "192.168.1.254", 4, "192.168.1.0/24", true},
		{"start outside CIDR", "192.168.2.10", 1, "192.168.1.0/24", true},
		{"zero count rejected", "192.168.1.10", 0, "192.168.1.0/24", true},
		{"ipv6 start rejected", "fe80::1", 1, "192.168.1.0/24", true},
		{"invalid CIDR rejected", "192.168.1.10", 1, "not-a-cidr", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateIPRangeInCIDR(tc.start, tc.count, tc.cidr)
			if tc.wantErr && err == nil {
				t.Errorf("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestResolveVIP(t *testing.T) {
	cases := []struct {
		name     string
		explicit string
		start    string
		want     string
		wantErr  bool
	}{
		{"explicit ipv4 wins", "192.168.1.50", "192.168.1.20", "192.168.1.50", false},
		{"explicit invalid rejected", "nope", "192.168.1.20", "", true},
		{"explicit ipv6 rejected", "2001:db8::1", "192.168.1.20", "", true},
		{"empty explicit derives from start", "", "192.168.1.20", "192.168.1.10", false},
		{"empty explicit, invalid start", "", "bogus", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveVIP(tc.explicit, tc.start)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIPInCIDR(t *testing.T) {
	cases := []struct {
		ip      string
		cidr    string
		want    bool
		wantErr bool
	}{
		{"192.168.1.5", "192.168.1.0/24", true, false},
		{"192.168.2.5", "192.168.1.0/24", false, false},
		{"not-an-ip", "10.0.0.0/8", false, true},
		{"10.0.0.1", "", false, true},
		{"::ffff:10.0.0.1", "10.0.0.0/8", false, false}, // 4-in-6 mapped addr doesn't match v4 prefix
		{"fe80::1", "10.0.0.0/8", false, false},         // family mismatch: no error
	}
	for _, tc := range cases {
		got, err := IPInCIDR(tc.ip, tc.cidr)
		if tc.wantErr {
			if err == nil {
				t.Errorf("IPInCIDR(%q,%q) = %v; want error", tc.ip, tc.cidr, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("IPInCIDR(%q,%q) error: %v", tc.ip, tc.cidr, err)
			continue
		}
		if got != tc.want {
			t.Errorf("IPInCIDR(%q,%q) = %v; want %v", tc.ip, tc.cidr, got, tc.want)
		}
	}
}

func TestCIDRsOverlap(t *testing.T) {
	cases := []struct {
		a, b    string
		want    bool
		wantErr bool
	}{
		{"192.168.1.0/24", "192.168.1.128/25", true, false},
		{"192.168.1.0/24", "192.168.2.0/24", false, false},
		{"bogus", "10.0.0.0/8", false, true},
		{"10.0.0.0/8", "", false, true},
		{"10.0.0.0/8", "fe80::/16", false, false}, // mixed family: no overlap, no error
	}
	for _, tc := range cases {
		got, err := CIDRsOverlap(tc.a, tc.b)
		if tc.wantErr {
			if err == nil {
				t.Errorf("CIDRsOverlap(%q,%q) = %v; want error", tc.a, tc.b, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("CIDRsOverlap(%q,%q) error: %v", tc.a, tc.b, err)
			continue
		}
		if got != tc.want {
			t.Errorf("CIDRsOverlap(%q,%q) = %v; want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
