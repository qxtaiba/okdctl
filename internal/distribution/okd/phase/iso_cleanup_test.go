package phase

import (
	"encoding/json"
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

func TestShellSingleQuote(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"simple", "'simple'"},
		{"with space", "'with space'"},
		{"it's", `'it'\''s'`},
		{"$(reboot)", "'$(reboot)'"},
		{"; id ;", "'; id ;'"},
		{"fedora-coreos-40.iso", "'fedora-coreos-40.iso'"},
		{"a'b", `'a'\''b'`},
	}
	for _, tc := range cases {
		got := shellSingleQuote(tc.input)
		if got != tc.want {
			t.Errorf("shellSingleQuote(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestValidateISODir(t *testing.T) {
	valid := []string{
		"/var/lib/vz/template/iso",
		"/mnt/iso",
	}
	for _, d := range valid {
		if err := validateISODir(d); err != nil {
			t.Errorf("expected valid isoDir %q to pass: %v", d, err)
		}
	}

	invalid := []string{
		"relative/path",
		"/var/lib/vz;rm -rf /",
		"/var/lib/vz/$(id)",
		"/var/lib/vz/`id`",
		"/var/lib/vz/iso|tee /tmp/x",
	}
	for _, d := range invalid {
		if err := validateISODir(d); err == nil {
			t.Errorf("expected invalid isoDir %q to be rejected", d)
		}
	}
}

func makeTestVM(fields map[string]string) map[string]json.RawMessage {
	m := make(map[string]json.RawMessage)
	for k, v := range fields {
		b, _ := json.Marshal(v)
		m[k] = b
	}
	return m
}

func TestVmDevicesReferenceISO(t *testing.T) {
	vm := makeTestVM(map[string]string{
		"ide2": "local:iso/fedora-coreos-40.iso,media=cdrom",
	})
	if !vmDevicesReferenceISO(vm, "fedora-coreos-40.iso") {
		t.Error("expected vm with ide2 cdrom to reference fedora-coreos-40.iso")
	}
	if vmDevicesReferenceISO(vm, "fedora-coreos-38.iso") {
		t.Error("expected vm to NOT reference fedora-coreos-38.iso")
	}

	// .old suffix must not false-match against the base filename.
	vmOld := makeTestVM(map[string]string{
		"ide2": "local:iso/fedora-coreos-40.iso.old,media=cdrom",
	})
	if vmDevicesReferenceISO(vmOld, "fedora-coreos-40.iso") {
		t.Error("fedora-coreos-40.iso.old must not match fedora-coreos-40.iso")
	}

	// description field must not trigger a match.
	vmDesc := makeTestVM(map[string]string{
		"description": "has fedora-coreos-40.iso in the notes",
	})
	if vmDevicesReferenceISO(vmDesc, "fedora-coreos-40.iso") {
		t.Error("description field must not trigger iso match")
	}
}
