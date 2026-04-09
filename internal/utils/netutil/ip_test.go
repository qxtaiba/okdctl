package netutil

import "testing"

func TestValidateIPRangeInCIDR(t *testing.T) {
	tests := []struct {
		name    string
		startIP string
		count   int
		cidr    string
		wantErr bool
	}{
		{"fits in /24", "192.168.1.100", 10, "192.168.1.0/24", false},
		{"overflows /24", "192.168.1.250", 10, "192.168.1.0/24", true},
		{"fits in /16", "192.168.1.100", 300, "192.168.0.0/16", false},
		{"outside /25", "192.168.1.200", 5, "192.168.1.0/25", true},
		{"start outside cidr", "10.0.0.1", 1, "192.168.1.0/24", true},
		{"single ip fits", "192.168.1.1", 1, "192.168.1.0/24", false},
		{"zero count", "192.168.1.1", 0, "192.168.1.0/24", true},
		{"negative count", "192.168.1.1", -1, "192.168.1.0/24", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIPRangeInCIDR(tt.startIP, tt.count, tt.cidr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIPRangeInCIDR(%q, %d, %q) error = %v, wantErr %v",
					tt.startIP, tt.count, tt.cidr, err, tt.wantErr)
			}
		})
	}
}
