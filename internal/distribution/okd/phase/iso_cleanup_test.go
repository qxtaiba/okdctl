package phase

import (
	"encoding/json"
	"testing"
)

// TestValidateProxmoxName locks the current [A-Za-z0-9_-] allowlist so a
// future refactor (e.g. swapping to a regex with a wrong anchor) cannot
// silently relax the gate. Leading-digit names are accepted today; if a
// hardening pass tightens that, both this test and the impl move together.
func TestValidateProxmoxName(t *testing.T) {
	accept := []string{"pve", "pve-1", "node_a", "PVE0", "1pve"}
	for _, name := range accept {
		if err := validateProxmoxName(name); err != nil {
			t.Errorf("validateProxmoxName(%q) rejected; want nil: %v", name, err)
		}
	}

	reject := []string{
		"",
		"pve.example",
		"pve/etc",
		"pve;rm",
		"pve`id`",
		"pve$(id)",
		"pve space",
		"pvé",
		"pve\x00",
	}
	for _, name := range reject {
		if err := validateProxmoxName(name); err == nil {
			t.Errorf("validateProxmoxName(%q) accepted; want error", name)
		}
	}
}

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
		"/var/lib/vz/iso with space",
		"/var/lib/vz/iso\twith\ttab",
		"/var/lib/vz/iso'quoted",
	}
	for _, d := range invalid {
		if err := validateISODir(d); err == nil {
			t.Errorf("expected invalid isoDir %q to be rejected", d)
		}
	}
}

func TestValidateRemoteFilename(t *testing.T) {
	accept := []string{
		"coreos.iso",
		"fedora-coreos-40.20240101.3.0-x86_64.iso",
		"custom_image-v1.iso",
		"MY-ISO.ISO",
	}
	for _, name := range accept {
		if err := ValidateRemoteFilename(name); err != nil {
			t.Errorf("ValidateRemoteFilename(%q) rejected; want nil: %v", name, err)
		}
	}

	reject := []string{
		"",
		"..",
		"foo/bar.iso",
		"foo\\bar.iso",
		"../etc/passwd",
		"foo;rm -rf /.iso",
		"foo|tee /tmp/x.iso",
		"foo&background.iso",
		"foo`id`.iso",
		"foo$(id).iso",
		"foo bar.iso",
		"foo\t.iso",
		"foo\n.iso",
		"foo$.iso",
		"foo!.iso",
	}
	for _, name := range reject {
		if err := ValidateRemoteFilename(name); err == nil {
			t.Errorf("ValidateRemoteFilename(%q) accepted; want error", name)
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
	if !vmDevicesReferenceISO(vm, "iso/fedora-coreos-40.iso") {
		t.Error("expected vm with ide2 cdrom to reference fedora-coreos-40.iso")
	}
	if vmDevicesReferenceISO(vm, "iso/fedora-coreos-38.iso") {
		t.Error("expected vm to NOT reference iso/fedora-coreos-38.iso")
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

func TestParseVMIDsFromSummary_onlyRunning(t *testing.T) {
	// Realistic summary response: three VMs, one stopped, two running.
	summaryJSON := []byte(`[
		{"vmid":100,"name":"okd-bootstrap","status":"running","mem":4096,"cpus":4,"uptime":3600},
		{"vmid":101,"name":"okd-master-0","status":"stopped","mem":16384,"cpus":8,"uptime":0},
		{"vmid":102,"name":"okd-master-1","status":"running","mem":16384,"cpus":8,"uptime":7200}
	]`)

	ids, err := parseVMIDsFromSummary(summaryJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 running vmids, got %d: %v", len(ids), ids)
	}
	if ids[0] != 100 || ids[1] != 102 {
		t.Errorf("expected vmids [100, 102], got %v", ids)
	}
}

func TestParseVMIDsFromSummary_empty(t *testing.T) {
	ids, err := parseVMIDsFromSummary([]byte(`[]`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected empty vmid list, got %v", ids)
	}
}

func TestConfigDevicesReferenceISO_found(t *testing.T) {
	// Per-vmid config shape from pvesh get /nodes/<node>/qemu/<vmid>/config:
	// top-level keys include device fields alongside non-device fields.
	configJSON := []byte(`{
		"ide2": "local:iso/fedora-coreos-40.iso,media=cdrom",
		"scsi0": "local-lvm:vm-100-disk-0,size=120G",
		"memory": 4096,
		"cores": 4
	}`)

	found, err := configDevicesReferenceISO(configJSON, "iso/fedora-coreos-40.iso")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Error("expected config with ide2 cdrom to reference fedora-coreos-40.iso")
	}
}

func TestConfigDevicesReferenceISO_notFound(t *testing.T) {
	configJSON := []byte(`{
		"scsi0": "local-lvm:vm-101-disk-0,size=120G",
		"memory": 16384,
		"cores": 8
	}`)

	found, err := configDevicesReferenceISO(configJSON, "iso/fedora-coreos-40.iso")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected config without iso reference to return false")
	}
}

func TestConfigDevicesReferenceISO_summaryShapeNoDevices(t *testing.T) {
	// The summary-list element shape (vmid/name/status/mem/cpus/uptime) must
	// not match device fields — confirms the per-vmid /config call is required.
	summaryElementJSON := []byte(`{
		"vmid": 100,
		"name": "okd-bootstrap",
		"status": "running",
		"mem": 4096,
		"cpus": 4,
		"uptime": 3600
	}`)

	found, err := configDevicesReferenceISO(summaryElementJSON, "fedora-coreos-40.iso")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("summary-list element shape must not match any device field")
	}
}
