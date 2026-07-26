package proxmox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
)

func TestProvider_ZeroizeEnv(t *testing.T) {
	t.Run("secret keys blanked and slice nil after call", func(t *testing.T) {
		p := New(WithEnv([]string{
			"PROXMOX_VE_PASSWORD=hunter2",
			"PROXMOX_VE_API_TOKEN=tok-fake",
			"KUBECONFIG=/etc/kube",
		}))
		p.ZeroizeEnv()
		if p.env != nil {
			t.Errorf("env not nil after ZeroizeEnv; got %v", p.env)
		}
	})

	t.Run("secret entries blanked before clear, non-secret also zeroed", func(t *testing.T) {
		p := New(WithEnv([]string{
			"PROXMOX_VE_API_TOKEN=tok-fake",
			"KUBECONFIG=/etc/kube",
		}))
		snap := p.env
		p.ZeroizeEnv()
		if snap[0] != "" {
			t.Errorf("secret entry not blanked before clear; got %q", snap[0])
		}
		if snap[1] != "" {
			t.Errorf("non-secret entry not zeroed by clear; got %q", snap[1])
		}
	})

	t.Run("nil and empty env are no-ops", func(_ *testing.T) {
		p1 := New()
		p1.ZeroizeEnv()

		p2 := New(WithEnv([]string{}))
		p2.ZeroizeEnv()
	})

	t.Run("idempotent second call", func(t *testing.T) {
		p := New(WithEnv([]string{"PROXMOX_VE_PASSWORD=hunter2"}))
		p.ZeroizeEnv()
		p.ZeroizeEnv()
		if p.env != nil {
			t.Errorf("env not nil after second ZeroizeEnv; got %v", p.env)
		}
	})

	t.Run("non-secret-keyed entries survive blanking pass but are cleared", func(t *testing.T) {
		p := New(WithEnv([]string{
			"PROXMOX_VE_ENDPOINT=https://pve.example.test:8006",
			"PROXMOX_VE_API_TOKEN=tok-fake",
		}))
		snap := p.env
		p.ZeroizeEnv()
		if strings.Contains(snap[0], "pve.example.test") {
			t.Errorf("non-secret entry not wiped by clear; got %q", snap[0])
		}
		if p.env != nil {
			t.Errorf("env not nil after ZeroizeEnv; got %v", p.env)
		}
	})
}

func TestPlanProvisionedNodes(t *testing.T) {
	cases := []struct {
		name            string
		startIP         string
		masterCount     int
		workerCount     int
		cidr            string
		wantNames       []string
		wantIPs         []string
		wantErrContains string
	}{
		{
			name:        "1 master 0 workers no cidr",
			startIP:     "192.168.1.20",
			masterCount: 1,
			workerCount: 0,
			wantNames:   []string{"bootstrap", "master0"},
			wantIPs:     []string{"192.168.1.20", "192.168.1.21"},
		},
		{
			name:        "3 masters 2 workers with cidr",
			startIP:     "192.168.1.20",
			masterCount: 3,
			workerCount: 2,
			cidr:        "192.168.1.0/24",
			wantNames:   []string{"bootstrap", "master0", "master1", "master2", "worker0", "worker1"},
			wantIPs:     []string{"192.168.1.20", "192.168.1.21", "192.168.1.22", "192.168.1.23", "192.168.1.24", "192.168.1.25"},
		},
		{
			name:            "empty startIP returns config error",
			startIP:         "",
			masterCount:     1,
			wantErrContains: "static IP start address",
		},
		{
			name:            "start ip outside cidr",
			startIP:         "192.168.2.10",
			masterCount:     3,
			workerCount:     2,
			cidr:            "192.168.1.0/24",
			wantErrContains: "static IP range does not fit in machine CIDR",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				Topology: config.TopologyConfig{
					ControlPlane: config.NodeConfig{Count: tc.masterCount},
					Workers:      config.NodeConfig{Count: tc.workerCount},
				},
				Networking: config.NetworkingConfig{
					StaticIP:    config.StaticIPConfig{Start: tc.startIP},
					MachineCIDR: tc.cidr,
				},
			}
			p := New()
			nodes, err := p.planProvisionedNodes(cfg)
			if tc.wantErrContains != "" {
				if err == nil {
					t.Fatalf("want error containing %q; got nil", tc.wantErrContains)
				}
				if !strings.Contains(err.Error(), tc.wantErrContains) {
					t.Errorf("error = %q; want substring %q", err.Error(), tc.wantErrContains)
				}
				var cfgErr *errtypes.ConfigError
				if !errors.As(err, &cfgErr) {
					t.Errorf("error type = %T; want *errtypes.ConfigError", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(nodes) != len(tc.wantNames) {
				t.Fatalf("len(nodes) = %d; want %d", len(nodes), len(tc.wantNames))
			}
			for i, n := range nodes {
				if n.name != tc.wantNames[i] || n.ip != tc.wantIPs[i] {
					t.Errorf("nodes[%d] = {%q %q}; want {%q %q}", i, n.name, n.ip, tc.wantNames[i], tc.wantIPs[i])
				}
			}
		})
	}
}

func TestProvider_Disconnect(t *testing.T) {
	p := New()
	p.connected = true
	p.terraformExec = terraform.New(t.TempDir())
	if err := p.Disconnect(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.connected {
		t.Error("expected p.connected = false")
	}
	if p.terraformExec != nil {
		t.Error("expected p.terraformExec = nil")
	}
}

func TestProvider_setupTerraform_idempotent(t *testing.T) {
	p := New()
	root := t.TempDir()
	p.setupTerraform(root, "production")
	first := p.terraformExec
	p.setupTerraform(root, "production")
	if p.terraformExec != first {
		t.Error("setupTerraform reinitialized executor for identical projectRoot/tfEnv")
	}
	p.setupTerraform(root, "staging")
	if p.terraformExec == first {
		t.Error("setupTerraform did not reinitialize executor when tfEnv changed")
	}
}

func TestProvider_Provision_Guards(t *testing.T) {
	t.Run("not connected", func(t *testing.T) {
		p := New()
		err := p.Provision(context.Background(), &config.Config{}, ProvisionOptions{})
		if !errors.Is(err, ErrNotConnected) {
			t.Fatalf("err = %v; want ErrNotConnected", err)
		}
	})

	t.Run("terraform not configured", func(t *testing.T) {
		p := New()
		p.connected = true
		err := p.Provision(context.Background(), &config.Config{}, ProvisionOptions{})
		if !errors.Is(err, ErrTerraformNotConfigured) {
			t.Fatalf("err = %v; want ErrTerraformNotConfigured", err)
		}
	})
}

func TestProvider_PlanPreview_Guards(t *testing.T) {
	t.Run("not connected", func(t *testing.T) {
		p := New()
		_, err := p.PlanPreview(context.Background(), &config.Config{}, ProvisionOptions{})
		if !errors.Is(err, ErrNotConnected) {
			t.Fatalf("err = %v; want ErrNotConnected", err)
		}
	})

	t.Run("terraform not configured", func(t *testing.T) {
		p := New()
		p.connected = true
		_, err := p.PlanPreview(context.Background(), &config.Config{}, ProvisionOptions{})
		if !errors.Is(err, ErrTerraformNotConfigured) {
			t.Fatalf("err = %v; want ErrTerraformNotConfigured", err)
		}
	})
}

func TestProvider_PlanPreview(t *testing.T) {
	setupWorkDir := func(t *testing.T) string {
		t.Helper()
		root := t.TempDir()
		tfDir := filepath.Join(root, "infrastructure", "terraform", "environments", "production")
		if err := os.MkdirAll(tfDir, 0o755); err != nil {
			t.Fatal(err)
		}
		return root
	}

	t.Run("no changes returns nil without parsing show", func(t *testing.T) {
		root := setupWorkDir(t)
		installFakeTerraformDispatch(t, 0, "")
		p := New()
		p.connected = true
		changes, err := p.PlanPreview(context.Background(), &config.Config{}, ProvisionOptions{ProjectRoot: root, TerraformEnv: "production"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if changes != nil {
			t.Errorf("changes = %+v; want nil", changes)
		}
	})

	t.Run("changes present are parsed and plan file removed", func(t *testing.T) {
		root := setupWorkDir(t)
		showJSON := `{"resource_changes":[{"address":"module.okd_cluster.proxmox_virtual_environment_vm.worker[0]","change":{"actions":["update"]}}]}`
		installFakeTerraformDispatch(t, 2, showJSON)
		p := New()
		p.connected = true
		changes, err := p.PlanPreview(context.Background(), &config.Config{}, ProvisionOptions{ProjectRoot: root, TerraformEnv: "production"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(changes) != 1 || changes[0].Action != terraform.PlanActionUpdate {
			t.Fatalf("changes = %+v; want 1 update", changes)
		}
		tfDir := filepath.Join(root, "infrastructure", "terraform", "environments", "production")
		if _, statErr := os.Stat(filepath.Join(tfDir, planPreviewFileName)); !os.IsNotExist(statErr) {
			t.Errorf("plan file not cleaned up: %v", statErr)
		}
	})
}

// installFakeTerraformDispatch writes a POSIX "terraform" fake on PATH that
// answers "init" with exit 0, "plan" with planExit (mirroring
// -detailed-exitcode: 0=no changes, 2=changes), and "show" with showStdout.
func installFakeTerraformDispatch(t *testing.T, planExit int, showStdout string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake terraform script relies on POSIX sh")
	}
	dir := t.TempDir()
	script := fmt.Sprintf(`#!/bin/sh
cmd="$1"
case "$cmd" in
  init) exit 0 ;;
  plan)
    for arg in "$@"; do
      case "$arg" in
        -out=*) planfile="${arg#-out=}" ;;
      esac
    done
    if [ -n "$planfile" ]; then : > "$planfile"; fi
    exit %d ;;
  show) cat <<'EOF'
%s
EOF
    exit 0 ;;
esac
exit 0
`, planExit, showStdout)
	path := filepath.Join(dir, "terraform")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// installFakePvesh writes a POSIX "ssh" fake on PATH that always answers
// with the contents of response, regardless of the pvesh subcommand/path
// it was invoked with.
func installFakePvesh(t *testing.T, response string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-ssh script relies on POSIX sh")
	}
	dir := t.TempDir()
	respFile := filepath.Join(dir, "response.json")
	if err := os.WriteFile(respFile, []byte(response), 0o644); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf("#!/bin/sh\ncat %q\n", respFile)
	sshPath := filepath.Join(dir, "ssh")
	if err := os.WriteFile(sshPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestProvider_ProbeVMEnumeration(t *testing.T) {
	cfg := &config.Config{Topology: config.TopologyConfig{VMIDBase: 100}}

	t.Run("no ssh exec skips probe", func(t *testing.T) {
		p := New()
		p.node = "pve-01"
		if got := p.probeVMEnumeration(context.Background(), cfg); got != enumProbeSkipped {
			t.Errorf("got %v; want enumProbeSkipped", got)
		}
	})

	t.Run("pvesh run error (invalid node) skips probe", func(t *testing.T) {
		p := New()
		p.node = "bad;node"
		p.sshExec = executor.New()
		if got := p.probeVMEnumeration(context.Background(), cfg); got != enumProbeSkipped {
			t.Errorf("got %v; want enumProbeSkipped", got)
		}
	})

	t.Run("malformed json payload skips probe", func(t *testing.T) {
		installFakePvesh(t, "not json")
		p := New()
		p.host, p.node = "10.0.0.1", "pve-01"
		p.sshExec = executor.New()
		if got := p.probeVMEnumeration(context.Background(), cfg); got != enumProbeSkipped {
			t.Errorf("got %v; want enumProbeSkipped", got)
		}
	})

	t.Run("vmid found", func(t *testing.T) {
		installFakePvesh(t, `[{"vmid":100},{"vmid":101}]`)
		p := New()
		p.host, p.node = "10.0.0.1", "pve-01"
		p.sshExec = executor.New()
		if got := p.probeVMEnumeration(context.Background(), cfg); got != enumYes {
			t.Errorf("got %v; want enumYes", got)
		}
	})

	t.Run("vmid not found", func(t *testing.T) {
		installFakePvesh(t, `[{"vmid":200}]`)
		p := New()
		p.host, p.node = "10.0.0.1", "pve-01"
		p.sshExec = executor.New()
		if got := p.probeVMEnumeration(context.Background(), cfg); got != enumNo {
			t.Errorf("got %v; want enumNo", got)
		}
	})

	t.Run("default vmid base used when topology.VMIDBase is zero", func(t *testing.T) {
		installFakePvesh(t, fmt.Sprintf(`[{"vmid":%d}]`, config.DefaultVMIDBase))
		p := New()
		p.host, p.node = "10.0.0.1", "pve-01"
		p.sshExec = executor.New()
		if got := p.probeVMEnumeration(context.Background(), &config.Config{}); got != enumYes {
			t.Errorf("got %v; want enumYes", got)
		}
	})
}

func TestInitIsRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"context canceled", context.Canceled, false},
		{"context deadline exceeded", context.DeadlineExceeded, false},
		{"config error", &errtypes.ConfigError{Msg: "bad config"}, false},
		{"auth error", &errtypes.AuthError{Msg: "bad token"}, false},
		{"wrapped context canceled", fmt.Errorf("outer: %w", context.Canceled), false},
		{"wrapped config error", fmt.Errorf("outer: %w", &errtypes.ConfigError{Msg: "x"}), false},
		{"generic error is retryable", errors.New("connection reset"), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := initIsRetryable(tc.err); got != tc.want {
				t.Errorf("initIsRetryable(%v) = %v; want %v", tc.err, got, tc.want)
			}
		})
	}
}
