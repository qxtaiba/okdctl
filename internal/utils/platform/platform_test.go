package platform

import (
	"fmt"
	"testing"
)

func TestParseOSRelease(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    OS
	}{
		{
			"fedora",
			"ID=fedora\nVERSION_ID=39\nID_LIKE=\"rhel centos fedora\"\n",
			OS{Family: "rhel", ID: "fedora", Version: "39"},
		},
		{
			"ubuntu",
			"ID=ubuntu\nVERSION_ID=\"24.04\"\nID_LIKE=debian\n",
			OS{Family: "debian", ID: "ubuntu", Version: "24.04"},
		},
		{
			"rocky",
			"ID=\"rocky\"\nVERSION_ID=\"9.3\"\nID_LIKE=\"rhel centos fedora\"\n",
			OS{Family: "rhel", ID: "rocky", Version: "9.3"},
		},
		{
			"debian",
			"ID=debian\nVERSION_ID=\"12\"\n",
			OS{Family: "debian", ID: "debian", Version: "12"},
		},
		{
			"alma",
			"ID=\"almalinux\"\nVERSION_ID=\"9.3\"\nID_LIKE=\"rhel centos fedora\"\n",
			OS{Family: "rhel", ID: "almalinux", Version: "9.3"},
		},
		{
			"unsupported",
			"ID=archlinux\nVERSION_ID=rolling\n",
			OS{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOSRelease(tt.content)
			if tt.name == "unsupported" {
				if err == nil {
					t.Error("expected error for unsupported OS")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseOSRelease() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("parseOSRelease() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNewPackageManager(t *testing.T) {
	tests := []struct {
		os       OS
		wantType string
	}{
		{OS{Family: "rhel"}, "*platform.DNFManager"},
		{OS{Family: "debian"}, "*platform.APTManager"},
	}
	for _, tt := range tests {
		t.Run(tt.os.Family, func(t *testing.T) {
			pm := NewPackageManager(tt.os)
			got := fmt.Sprintf("%T", pm)
			if got != tt.wantType {
				t.Errorf("NewPackageManager(%v) type = %s, want %s", tt.os, got, tt.wantType)
			}
		})
	}
}
