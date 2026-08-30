package firewall

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
)

func TestHAProxyFrontendPorts(t *testing.T) {
	ports := HAProxyFrontendPorts()

	wantNumbers := map[int]bool{phase.KubeAPIPort: true, 22623: true, 80: true, 443: true}

	// tcp-only is the invariant: a same-number udp rule (e.g. dns udp/53) must
	// never slip into the HAProxy frontend set.
	for _, p := range ports {
		if !wantNumbers[p.Number] {
			t.Errorf("unexpected port number %d", p.Number)
		}
		if p.Protocol != "tcp" {
			t.Errorf("port %d: protocol=%q, want tcp", p.Number, p.Protocol)
		}
	}

	if len(ports) > 0 {
		ports[0].Number = 9999
		fresh := HAProxyFrontendPorts()
		if fresh[0].Number == 9999 {
			t.Error("HAProxyFrontendPorts returned reference into haproxyFrontends; want defensive copy")
		}
	}
}

func TestValidatePort(t *testing.T) {
	valid := []Port{
		{Number: 6443, Protocol: "tcp"},
		{Number: 53, Protocol: "udp"},
	}
	for _, p := range valid {
		if err := validatePort(p); err != nil {
			t.Errorf("validatePort(%v) = %v, want nil", p, err)
		}
	}

	invalidNumbers := []int{0, -1, 65536, 99999}
	for _, n := range invalidNumbers {
		p := Port{Number: n, Protocol: "tcp"}
		err := validatePort(p)
		if err == nil {
			t.Errorf("validatePort port=%d: want error, got nil", n)
			continue
		}
		if !strings.Contains(err.Error(), "invalid port number") {
			t.Errorf("validatePort port=%d: error %q missing 'invalid port number'", n, err)
		}
	}

	invalidProtocols := []string{"", "TCP", "sctp", "tcp/ip", "tcp; rm", "icmp"}
	for _, proto := range invalidProtocols {
		p := Port{Number: 80, Protocol: proto}
		err := validatePort(p)
		if err == nil {
			t.Errorf("validatePort proto=%q: want error, got nil", proto)
			continue
		}
		if !strings.Contains(err.Error(), "invalid protocol") {
			t.Errorf("validatePort proto=%q: error %q missing 'invalid protocol'", proto, err)
		}
	}
}
