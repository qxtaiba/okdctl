package nodetypes

import "testing"

func TestIsCoreOSISOName(t *testing.T) {
	accept := []string{
		"fedora-coreos-40.20240101.3.0-x86_64.iso",
		"fedora-coreos-41.3.1-x86_64.iso",
		"scos-10.0.20251103-0-live-iso.x86_64.iso",
		"scos-9.0.20250510-0.iso",
	}
	for _, name := range accept {
		if !IsCoreOSISOName(name) {
			t.Errorf("IsCoreOSISOName(%q) = false, want true", name)
		}
	}

	reject := []string{
		"",
		"other-distro.iso",
		"fedora-coreos-40.img",
		"scos.iso",
		"fedora-coreos.iso",
		"fcos-40.iso",
		"scos-40.iso.old",
		"../scos-40.iso",
		"fedora-coreos-40.iso/../../../etc/passwd",
	}
	for _, name := range reject {
		if IsCoreOSISOName(name) {
			t.Errorf("IsCoreOSISOName(%q) = true, want false", name)
		}
	}
}
