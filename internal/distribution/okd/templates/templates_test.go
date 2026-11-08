package templates

import (
	"strings"
	"testing"
)

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

		// regression guard: mask (not disable) is the actual fix, see coreos/fedora-coreos-tracker#468.
		if !strings.Contains(got, "#468") {
			t.Errorf("RenderFstrimMachineConfig(%s) missing #468 tracker reference: %q", role, got)
		}
		if !strings.Contains(got, "mask: true") {
			t.Errorf("RenderFstrimMachineConfig(%s) missing mask: true for stock fstrim.timer: %q", role, got)
		}
	}
}
