package phase

import (
	"testing"
)

func TestRefuseUnsafeISOPath(t *testing.T) {
	const isoDir = "/var/lib/vz/template/iso"

	safe := []string{
		"/var/lib/vz/template/iso/fedora-coreos-40.20240101.iso",
		"/var/lib/vz/template/iso/fedora-coreos-41.3.1-x86_64.iso",
	}
	for _, p := range safe {
		if err := refuseUnsafeISOPath(isoDir, p); err != nil {
			t.Errorf("expected safe path %q to pass: %v", p, err)
		}
	}

	unsafe := []string{
		"/",
		"/var/lib/vz",
		"/var/lib/vz/template/iso",
		"/var/lib/vz/template/iso/",
		"/var/lib/vz/template/iso/fedora-coreos-40.iso/../../../etc/passwd",
		"/var/lib/vz/template/iso/other-distro.iso",
		"/var/lib/vz/template/iso/fedora-coreos-40.img",
		"/tmp/fedora-coreos-40.iso",
		"",
	}
	for _, p := range unsafe {
		if err := refuseUnsafeISOPath(isoDir, p); err == nil {
			t.Errorf("expected unsafe path %q to be rejected", p)
		}
	}
}
