package hostssh

import (
	"context"
	"testing"
)

func TestRemoveCustomISOsFromProxmox(t *testing.T) {
	cases := []struct {
		name    string
		mode    string
		names   []string
		wantRM  string // "" means rm must never run
		blocked string
	}{
		{
			name: "removes unreferenced isos", mode: "no-ref",
			names: []string{"bootstrap.iso", "master0.iso"}, wantRM: "2",
		},
		{
			// Must match installFakeSSH's in-use fixture (fedora-coreos-40.20240101.iso verbatim).
			name: "skips iso referenced by running vm", mode: "in-use",
			names: []string{"fedora-coreos-40.20240101.iso"}, blocked: "iso is in use",
		},
		{
			name: "skips unsafe filenames", mode: "no-ref",
			names: []string{"../escape.iso", "sub/dir.iso"}, blocked: "every name is unsafe",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			counter, p := setupISORemoveFake(t, tc.mode)
			if err := RemoveCustomISOsFromProxmox(context.Background(), p, "/var/lib/vz/template/iso", tc.names); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertRMCalls(t, counter, tc.wantRM, tc.blocked)
		})
	}
}

func TestRemoveCustomISOsFromProxmox_rejectsUnsafeISODir(t *testing.T) {
	p := newTestISOParams(t)
	err := RemoveCustomISOsFromProxmox(context.Background(), p, "relative/dir", []string{"bootstrap.iso"})
	if err == nil {
		t.Fatal("expected error for non-absolute isoDir; got nil")
	}
}
