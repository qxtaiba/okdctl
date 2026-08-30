package hostssh

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestValidateProxmoxName locks the [A-Za-z0-9_-] allowlist (leading digits
// included) against silent relaxation; its reject list doubles as the
// injection-payload set for the node atom pveshRun interpolates.
func TestValidateProxmoxName(t *testing.T) {
	accept := []string{"pve-1", "node_a", "PVE0", "1pve", "A"}
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
		"pve\ttab",
		"..",
		"/",
		"node|pipe",
		"node&bg",
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
		"/var/lib/vz/template/iso/fedora-coreos-41.3.1-x86_64.iso",
		"/var/lib/vz/template/iso/scos-10.0.20251103-0-live-iso.x86_64.iso",
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
		"/var/lib/vz/template/iso/scos.iso",
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
		{"fedora-coreos-40.iso", "'fedora-coreos-40.iso'"},
		{"with space", "'with space'"},
		{"it's", `'it'\''s'`},
		{"$(reboot)", "'$(reboot)'"},
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
		if err := ValidateISODir(d); err != nil {
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
		if err := ValidateISODir(d); err == nil {
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

func TestParseVMIDsFromSummary(t *testing.T) {
	t.Run("only running", func(t *testing.T) {
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
	})

	t.Run("empty", func(t *testing.T) {
		ids, err := parseVMIDsFromSummary([]byte(`[]`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ids) != 0 {
			t.Errorf("expected empty vmid list, got %v", ids)
		}
	})
}

// TestRemoveFCOSISOFromProxmox_findCmdMatchesBothShapesLocally runs the real
// find command through a local shell (no ssh) to prove the \( -o \) grouping
// is valid POSIX find(1) syntax.
func TestRemoveFCOSISOFromProxmox_findCmdMatchesBothShapesLocally(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh/find")
	}
	dir := t.TempDir()
	want := map[string]bool{
		"fedora-coreos-40.20240101.3.0-x86_64.iso": true,
		"scos-10.0.20251103-0-live-iso.x86_64.iso": true,
		"other-distro.iso":                         false,
		"fedora-coreos-40.img":                     false,
		"scos.iso":                                 false,
	}
	for name := range want {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	findCmd := "find " + shellSingleQuote(dir) + " -maxdepth 1 " + findCoreOSISONameClause() + " -type f -print0 2>/dev/null || true"
	out, err := exec.CommandContext(context.Background(), "sh", "-c", findCmd).Output()
	if err != nil {
		t.Fatalf("sh -c %q: %v", findCmd, err)
	}

	got := parseNullDelimitedFileList(string(out))
	gotSet := make(map[string]bool, len(got))
	for _, f := range got {
		gotSet[filepath.Base(f)] = true
	}
	for name, wantMatch := range want {
		if gotSet[name] != wantMatch {
			t.Errorf("find matched %q = %v, want %v", name, gotSet[name], wantMatch)
		}
	}
}
