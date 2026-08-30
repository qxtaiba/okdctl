package dns

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
)

const bootstrapGolden = `# dnsmasq configuration - bootstrap/install phase
#
# During bootstrap, api.* DNS points to the bastion IP where HAProxy load
# balances to the control plane nodes. kube-vip gets exclusive ownership of
# the VIP from the start - no ARP conflict with the bastion.

# listen on bastion IP and localhost (explicit binding overrides main config)
listen-address=127.0.0.1
listen-address=192.168.1.20
bind-interfaces

# upstream dns servers for external queries
server=192.168.1.1

# api endpoints (point to bastion IP where HAProxy load balances to masters)
address=/api.mycluster.k8s.local/192.168.1.20
address=/api-int.mycluster.k8s.local/192.168.1.20

# apps wildcard (haproxy on bastion routes to workers during bootstrap)
address=/.apps.mycluster.k8s.local/192.168.1.20

# cluster nodes
address=/mycluster-bastion.mycluster.k8s.local/192.168.1.20
address=/mycluster-bootstrap.mycluster.k8s.local/192.168.1.140
address=/mycluster-master0.mycluster.k8s.local/192.168.1.141
address=/mycluster-worker0.mycluster.k8s.local/192.168.1.142

# ptr records for reverse lookup
ptr-record=10.1.168.192.in-addr.arpa,api.mycluster.k8s.local
ptr-record=20.1.168.192.in-addr.arpa,mycluster-bastion.mycluster.k8s.local
ptr-record=140.1.168.192.in-addr.arpa,mycluster-bootstrap.mycluster.k8s.local
ptr-record=141.1.168.192.in-addr.arpa,mycluster-master0.mycluster.k8s.local
ptr-record=142.1.168.192.in-addr.arpa,mycluster-worker0.mycluster.k8s.local

# short name aliases
address=/mycluster-api/192.168.1.20
address=/mycluster-bastion/192.168.1.20
address=/mycluster-bootstrap/192.168.1.140
address=/mycluster-master0/192.168.1.141
address=/mycluster-worker0/192.168.1.142
`

const productionGolden = `# dnsmasq configuration - post-bootstrap/production
#
# use this after post-install. kube-vip handles api load balancing via vip.

# listen on bastion IP and localhost (explicit binding overrides main config)
listen-address=127.0.0.1
listen-address=192.168.1.20
bind-interfaces

# upstream dns servers for external queries
server=192.168.1.1

# api endpoints (kube-vip provides vip for control plane api)
address=/api.mycluster.k8s.local/192.168.1.10
address=/api-int.mycluster.k8s.local/192.168.1.10

# apps wildcard (default router)
address=/.apps.mycluster.k8s.local/192.168.1.30

# custom domain (lab.example.com)
address=/.lab.example.com/192.168.1.31

# cluster nodes
address=/mycluster-bastion.mycluster.k8s.local/192.168.1.20
address=/mycluster-master0.mycluster.k8s.local/192.168.1.141
address=/mycluster-worker0.mycluster.k8s.local/192.168.1.142

# ptr records for reverse lookup
ptr-record=10.1.168.192.in-addr.arpa,api.mycluster.k8s.local
ptr-record=20.1.168.192.in-addr.arpa,mycluster-bastion.mycluster.k8s.local
ptr-record=141.1.168.192.in-addr.arpa,mycluster-master0.mycluster.k8s.local
ptr-record=142.1.168.192.in-addr.arpa,mycluster-worker0.mycluster.k8s.local
ptr-record=30.1.168.192.in-addr.arpa,apps.mycluster.k8s.local
ptr-record=31.1.168.192.in-addr.arpa,lab.example.com

# short name aliases
address=/mycluster-bastion/192.168.1.20
address=/mycluster-api/192.168.1.10
address=/mycluster-apps/192.168.1.30
address=/mycluster-router/192.168.1.31
address=/mycluster-master0/192.168.1.141
address=/mycluster-worker0/192.168.1.142
`

func renderCfg() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Topology.ControlPlane.Count = 1
	cfg.Topology.Workers.Count = 1
	return cfg
}

func redirectConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := dnsmasqConfigDir
	dnsmasqConfigDir = dir
	t.Cleanup(func() { dnsmasqConfigDir = orig })
	return dir
}

func stubServiceFns(t *testing.T) (restarts *int) {
	t.Helper()
	restarts = new(int)
	stubDnsmasqFns(t,
		func(context.Context) error { return nil },
		func(context.Context) error { *restarts++; return nil })
	return restarts
}

func TestBuildConfigData_Rejections(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*config.Config)
	}{
		{"empty cluster name", func(c *config.Config) { c.Cluster.Name = "" }},
		{"invalid dns label", func(c *config.Config) { c.Cluster.Name = "My_Cluster" }},
		{"missing bastion ip", func(c *config.Config) { c.Networking.Bastion.IP = "" }},
		{"missing static start", func(c *config.Config) { c.Networking.StaticIP.Start = "" }},
		{"node range overflows machine cidr", func(c *config.Config) { c.Networking.StaticIP.Start = "192.168.1.254" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := renderCfg()
			tc.mutate(cfg)
			if _, err := buildConfigData(cfg); err == nil {
				t.Error("want error, got nil")
			}
		})
	}

	t.Run("nil config", func(t *testing.T) {
		if _, err := buildConfigData(nil); err == nil {
			t.Error("want error, got nil")
		}
	})
}

func TestGenerateBootstrapConfig_Golden(t *testing.T) {
	dir := t.TempDir()
	outputDir := filepath.Join(dir, "dns")

	path, content, err := GenerateBootstrapConfig(renderCfg(), outputDir)
	if err != nil {
		t.Fatalf("GenerateBootstrapConfig: %v", err)
	}

	if want := filepath.Join(outputDir, "dnsmasq-bootstrap.conf"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if content != bootstrapGolden {
		t.Errorf("rendered bootstrap config:\n%s\nwant:\n%s", content, bootstrapGolden)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written config: %v", err)
	}
	if string(onDisk) != content {
		t.Error("on-disk config differs from returned content")
	}
}

func TestDeployBootstrap_WritesConfAndRestarts(t *testing.T) {
	dir := redirectConfigDir(t)
	restarts := stubServiceFns(t)

	if err := DeployBootstrap(context.Background(), renderCfg()); err != nil {
		t.Fatalf("DeployBootstrap: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "okd-mycluster.conf"))
	if err != nil {
		t.Fatalf("cluster conf not written: %v", err)
	}
	if string(got) != bootstrapGolden {
		t.Errorf("deployed bootstrap config:\n%s\nwant:\n%s", got, bootstrapGolden)
	}
	if *restarts != 1 {
		t.Errorf("dnsmasq restarted %d times, want 1", *restarts)
	}
}

func TestDeployProduction_WritesConfAndRestarts(t *testing.T) {
	dir := redirectConfigDir(t)
	restarts := stubServiceFns(t)

	customDomains := []templates.DNSCustomDomain{{Domain: "lab.example.com", IP: "192.168.1.31"}}
	if err := DeployProduction(context.Background(), renderCfg(), "192.168.1.30", "", customDomains); err != nil {
		t.Fatalf("DeployProduction: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "okd-mycluster.conf"))
	if err != nil {
		t.Fatalf("cluster conf not written: %v", err)
	}
	if string(got) != productionGolden {
		t.Errorf("deployed production config:\n%s\nwant:\n%s", got, productionGolden)
	}
	if *restarts != 1 {
		t.Errorf("dnsmasq restarted %d times, want 1", *restarts)
	}
}

func TestDeployProduction_ExplicitVIPOverridesDerived(t *testing.T) {
	dir := redirectConfigDir(t)
	stubServiceFns(t)

	if err := DeployProduction(context.Background(), renderCfg(), "", "192.168.1.99", nil); err != nil {
		t.Fatalf("DeployProduction: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "okd-mycluster.conf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"address=/api.mycluster.k8s.local/192.168.1.99\n",
		"address=/api-int.mycluster.k8s.local/192.168.1.99\n",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("config missing %q", want)
		}
	}
}

func TestDeployProduction_InvalidIPsRejected(t *testing.T) {
	cases := []struct {
		name          string
		appsIP        string
		kubeVipIP     string
		customDomains []templates.DNSCustomDomain
	}{
		{name: "invalid apps ip", appsIP: "not-an-ip"},
		{name: "invalid kube-vip ip", kubeVipIP: "999.1.1.1"},
		{name: "invalid custom domain ip", customDomains: []templates.DNSCustomDomain{{Domain: "lab.example.com", IP: "192.168.1"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := redirectConfigDir(t)
			restarts := stubServiceFns(t)

			err := DeployProduction(context.Background(), renderCfg(), tc.appsIP, tc.kubeVipIP, tc.customDomains)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			entries, readErr := os.ReadDir(dir)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if len(entries) != 0 || *restarts != 0 {
				t.Errorf("rejected deploy must not write config or restart dnsmasq (files=%d restarts=%d)", len(entries), *restarts)
			}
		})
	}
}
