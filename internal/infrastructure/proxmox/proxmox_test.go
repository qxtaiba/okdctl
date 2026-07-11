package proxmox

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
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

func TestRetrieveProvisionResult(t *testing.T) {
	cases := []struct {
		name            string
		startIP         string
		masterCount     int
		workerCount     int
		cidr            string
		gateway         string
		wantBootstrap   string
		wantMasters     []string
		wantWorkers     []string
		wantAPIServerIP string
		wantErrContains string
	}{
		{
			name:          "1 master 0 workers no cidr",
			startIP:       "192.168.1.20",
			masterCount:   1,
			workerCount:   0,
			wantBootstrap: "192.168.1.20",
			wantMasters:   []string{"192.168.1.21"},
			wantWorkers:   []string{},
		},
		{
			name:          "3 masters 2 workers with cidr",
			startIP:       "192.168.1.20",
			masterCount:   3,
			workerCount:   2,
			cidr:          "192.168.1.0/24",
			wantBootstrap: "192.168.1.20",
			wantMasters:   []string{"192.168.1.21", "192.168.1.22", "192.168.1.23"},
			wantWorkers:   []string{"192.168.1.24", "192.168.1.25"},
		},
		{
			name:            "gateway sets APIServerIP",
			startIP:         "192.168.1.20",
			masterCount:     1,
			workerCount:     0,
			gateway:         "192.168.1.1",
			wantBootstrap:   "192.168.1.20",
			wantMasters:     []string{"192.168.1.21"},
			wantWorkers:     []string{},
			wantAPIServerIP: "192.168.1.1",
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
			wantErrContains: "IP range validation failed",
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
					Gateway:     tc.gateway,
				},
			}
			p := New()
			result, err := p.retrieveProvisionResult(cfg)
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
			if result.BootstrapIP != tc.wantBootstrap {
				t.Errorf("BootstrapIP = %q; want %q", result.BootstrapIP, tc.wantBootstrap)
			}
			if len(result.ControlPlaneIPs) != len(tc.wantMasters) {
				t.Fatalf("len(ControlPlaneIPs) = %d; want %d",
					len(result.ControlPlaneIPs), len(tc.wantMasters))
			}
			for i, want := range tc.wantMasters {
				if result.ControlPlaneIPs[i] != want {
					t.Errorf("ControlPlaneIPs[%d] = %q; want %q",
						i, result.ControlPlaneIPs[i], want)
				}
			}
			if len(result.WorkerIPs) != len(tc.wantWorkers) {
				t.Fatalf("len(WorkerIPs) = %d; want %d",
					len(result.WorkerIPs), len(tc.wantWorkers))
			}
			for i, want := range tc.wantWorkers {
				if result.WorkerIPs[i] != want {
					t.Errorf("WorkerIPs[%d] = %q; want %q",
						i, result.WorkerIPs[i], want)
				}
			}
			if tc.wantAPIServerIP != "" && result.APIServerIP != tc.wantAPIServerIP {
				t.Errorf("APIServerIP = %q; want %q", result.APIServerIP, tc.wantAPIServerIP)
			}
		})
	}
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
