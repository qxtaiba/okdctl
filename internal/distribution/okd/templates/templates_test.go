package templates

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestRenderChronyConf(t *testing.T) {
	got, err := RenderChronyConf(ChronyConfData{Server: "192.168.1.20"})
	if err != nil {
		t.Fatalf("RenderChronyConf: %v", err)
	}

	want := `# Managed by okdctl. Edits here are overwritten on the next MachineConfig
# rollout — set networking.ntp_server in okdctl.yaml instead.
server 192.168.1.20 iburst
driftfile /var/lib/chrony/drift
rtcsync

# Proxmox pause/resume can jump the guest clock by minutes in one shot;
# chrony's default slew-only correction takes hours to converge, during
# which etcd leader elections fail and node certs read "not yet valid".
# Step unconditionally instead of slewing.
makestep 1.0 -1
`
	if got != want {
		t.Errorf("RenderChronyConf() = %q; want %q", got, want)
	}

	// makestep 1.0 -1 steps the clock on every correction, unbounded — the
	// structural fix for VM pause/resume clock jumps. A regression here
	// would silently reintroduce the slew-only etcd/TLS failure mode.
	if !strings.Contains(got, "makestep 1.0 -1") {
		t.Errorf("RenderChronyConf() missing makestep 1.0 -1 directive: %q", got)
	}
}

func TestRenderChronyMachineConfig(t *testing.T) {
	conf, err := RenderChronyConf(ChronyConfData{Server: "10.0.0.5"})
	if err != nil {
		t.Fatalf("RenderChronyConf: %v", err)
	}
	source := "data:text/plain;charset=utf-8;base64," + base64.StdEncoding.EncodeToString([]byte(conf))

	for _, role := range []string{"master", "worker"} {
		name := "99-" + role + "-chrony-configuration"
		got, err := RenderChronyMachineConfig(ChronyMachineConfigData{
			Role:   role,
			Name:   name,
			Source: source,
		})
		if err != nil {
			t.Fatalf("RenderChronyMachineConfig(%s): %v", role, err)
		}

		want := `apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: ` + role + `
  name: ` + name + `
spec:
  config:
    ignition:
      version: 3.2.0
    storage:
      files:
        - path: /etc/chrony.conf
          mode: 420
          overwrite: true
          contents:
            source: ` + source + `
`
		if got != want {
			t.Errorf("RenderChronyMachineConfig(%s) = %q; want %q", role, got, want)
		}

		decoded, decErr := base64.StdEncoding.DecodeString(strings.TrimPrefix(source, "data:text/plain;charset=utf-8;base64,"))
		if decErr != nil {
			t.Fatalf("decode embedded chrony.conf: %v", decErr)
		}
		if string(decoded) != conf {
			t.Errorf("embedded chrony.conf source = %q; want %q", decoded, conf)
		}
	}
}

func TestRenderFstrimMachineConfig(t *testing.T) {
	for _, role := range []string{"master", "worker"} {
		name := "99-" + role + "-fstrim-configuration"
		got, err := RenderFstrimMachineConfig(FstrimMachineConfigData{
			Role: role,
			Name: name,
		})
		if err != nil {
			t.Fatalf("RenderFstrimMachineConfig(%s): %v", role, err)
		}

		want := `apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: ` + role + `
  name: ` + name + `
spec:
  config:
    ignition:
      version: 3.2.0
    systemd:
      units:
        - name: fstrim.timer
          mask: true
        - name: okdctl-fstrim.service
          contents: |
            [Unit]
            Description=Trim discardable blocks on FCOS mountpoints (fstrim.timer workaround for coreos/fedora-coreos-tracker#468)
            Documentation=https://github.com/coreos/fedora-coreos-tracker/issues/468
            [Service]
            Type=oneshot
            ExecStart=/usr/sbin/fstrim --verbose /
            ExecStart=-/usr/sbin/fstrim --verbose /boot
            ExecStart=-/usr/sbin/fstrim --verbose /boot/efi
        - name: okdctl-fstrim.timer
          enabled: true
          contents: |
            [Unit]
            Description=Periodically trim discardable blocks on FCOS mountpoints
            [Timer]
            OnCalendar=weekly
            AccuracySec=1h
            RandomizedDelaySec=3600
            Persistent=true
            [Install]
            WantedBy=timers.target
`
		if got != want {
			t.Errorf("RenderFstrimMachineConfig(%s) = %q; want %q", role, got, want)
		}

		// coreos/fedora-coreos-tracker#468 is the documented reason the stock
		// fstrim.timer is masked instead of relied on — a regression here
		// would silently drop that trail and mask: true is the actual fix
		// (disable can be re-enabled by the MCO/user; mask cannot).
		if !strings.Contains(got, "#468") {
			t.Errorf("RenderFstrimMachineConfig(%s) missing #468 tracker reference: %q", role, got)
		}
		if !strings.Contains(got, "mask: true") {
			t.Errorf("RenderFstrimMachineConfig(%s) missing mask: true for stock fstrim.timer: %q", role, got)
		}
	}
}
