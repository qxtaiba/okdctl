package setup

import (
	"errors"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func cfgWithIgnitionIP(ip string, port int) *config.Config {
	cfg := config.DefaultConfig()
	cfg.HTTPServer.IgnitionServerIP = ip
	cfg.HTTPServer.Port = port
	return cfg
}

func TestBuildIgnitionURLForNode_accept(t *testing.T) {
	cases := []struct {
		name    string
		ip      string
		port    int
		role    phase.NodeRole
		wantURL string
	}{
		{
			name:    "rfc1918 10.x default port",
			ip:      "10.0.0.1",
			port:    0,
			role:    phase.RoleMaster,
			wantURL: "https://10.0.0.1/ignition/master.ign",
		},
		{
			name:    "rfc1918 192.168.x default port",
			ip:      "192.168.1.20",
			port:    0,
			role:    phase.RoleWorker,
			wantURL: "https://192.168.1.20/ignition/worker.ign",
		},
		{
			name:    "rfc1918 172.16.x default port",
			ip:      "172.16.0.5",
			port:    0,
			role:    phase.RoleBootstrap,
			wantURL: "https://172.16.0.5/ignition/bootstrap.ign",
		},
		{
			name:    "loopback 127.0.0.1 default port",
			ip:      "127.0.0.1",
			port:    0,
			role:    phase.RoleMaster,
			wantURL: "https://127.0.0.1/ignition/master.ign",
		},
		{
			name:    "link-local 169.254.0.1 default port",
			ip:      "169.254.0.1",
			port:    0,
			role:    phase.RoleMaster,
			wantURL: "https://169.254.0.1/ignition/master.ign",
		},
		{
			name:    "ipv6 ula fd00::1 default port",
			ip:      "fd00::1",
			port:    0,
			role:    phase.RoleMaster,
			wantURL: "https://fd00::1/ignition/master.ign",
		},
		{
			name:    "ipv6 link-local fe80::1 default port",
			ip:      "fe80::1",
			port:    0,
			role:    phase.RoleMaster,
			wantURL: "https://fe80::1/ignition/master.ign",
		},
		{
			name:    "explicit port 443 elided",
			ip:      "10.0.0.1",
			port:    443,
			role:    phase.RoleMaster,
			wantURL: "https://10.0.0.1/ignition/master.ign",
		},
		{
			name:    "custom port 8080 included",
			ip:      "192.168.1.20",
			port:    8080,
			role:    phase.RoleWorker,
			wantURL: "https://192.168.1.20:8080/ignition/worker.ign",
		},
		{
			name:    "custom port 9443 included",
			ip:      "10.0.0.1",
			port:    9443,
			role:    phase.RoleBootstrap,
			wantURL: "https://10.0.0.1:9443/ignition/bootstrap.ign",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cfgWithIgnitionIP(tt.ip, tt.port)
			got, err := BuildIgnitionURLForNode(cfg, tt.role)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantURL {
				t.Errorf("got %q, want %q", got, tt.wantURL)
			}
		})
	}
}

func TestBuildIgnitionURLForNode_reject(t *testing.T) {
	cases := []struct {
		name string
		ip   string
	}{
		{"public ipv4 8.8.8.8", "8.8.8.8"},
		{"public ipv4 1.1.1.1", "1.1.1.1"},
		{"documentation ipv6 2001:db8::1", "2001:db8::1"},
		{"empty string", ""},
		{"hostname not ip", "bastion.local"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cfgWithIgnitionIP(tt.ip, 0)
			_, err := BuildIgnitionURLForNode(cfg, phase.RoleMaster)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var cfgErr *errtypes.ConfigError
			if !errors.As(err, &cfgErr) {
				t.Errorf("error type = %T, want *errtypes.ConfigError", err)
			}
		})
	}
}

func TestBuildLiveKargs_golden(t *testing.T) {
	cases := []struct {
		name   string
		params LiveKargsParams
		want   []string
	}{
		{
			name: "standard static config",
			params: LiveKargsParams{
				NodeIP:      "192.168.1.10",
				Gateway:     "192.168.1.1",
				Netmask:     "255.255.255.0",
				DNS:         "192.168.1.20",
				Interface:   "ens3",
				IgnitionURL: "https://192.168.1.20/ignition/master.ign",
			},
			want: []string{
				"coreos.inst.ignition_url=https://192.168.1.20/ignition/master.ign",
				"ip=192.168.1.10::192.168.1.1:255.255.255.0::ens3:none",
				"nameserver=192.168.1.20",
			},
		},
		{
			name: "custom port in ignition url",
			params: LiveKargsParams{
				NodeIP:      "10.0.0.5",
				Gateway:     "10.0.0.1",
				Netmask:     "255.255.0.0",
				DNS:         "10.0.0.2",
				Interface:   "eth0",
				IgnitionURL: "https://10.0.0.2:8080/ignition/worker.ign",
			},
			want: []string{
				"coreos.inst.ignition_url=https://10.0.0.2:8080/ignition/worker.ign",
				"ip=10.0.0.5::10.0.0.1:255.255.0.0::eth0:none",
				"nameserver=10.0.0.2",
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildLiveKargs(&tt.params)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d; got %v", len(got), len(tt.want), got)
			}
			for i, w := range tt.want {
				if got[i] != w {
					t.Errorf("karg[%d]: got %q, want %q", i, got[i], w)
				}
			}
		})
	}
}

func TestBuildDestKargs_golden(t *testing.T) {
	cases := []struct {
		name   string
		params LiveKargsParams
		want   []string
	}{
		{
			name: "standard static config",
			params: LiveKargsParams{
				NodeIP:    "192.168.1.10",
				Gateway:   "192.168.1.1",
				Netmask:   "255.255.255.0",
				DNS:       "192.168.1.20",
				Interface: "ens3",
			},
			want: []string{
				"ip=192.168.1.10::192.168.1.1:255.255.255.0::ens3:none",
				"nameserver=192.168.1.20",
			},
		},
		{
			name: "bond interface",
			params: LiveKargsParams{
				NodeIP:    "10.0.0.11",
				Gateway:   "10.0.0.1",
				Netmask:   "255.255.0.0",
				DNS:       "10.0.0.2",
				Interface: "bond0",
			},
			want: []string{
				"ip=10.0.0.11::10.0.0.1:255.255.0.0::bond0:none",
				"nameserver=10.0.0.2",
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildDestKargs(&tt.params)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d; got %v", len(got), len(tt.want), got)
			}
			for i, w := range tt.want {
				if got[i] != w {
					t.Errorf("karg[%d]: got %q, want %q", i, got[i], w)
				}
			}
		})
	}
}
