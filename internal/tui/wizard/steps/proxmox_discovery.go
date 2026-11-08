package steps

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/luthermonson/go-proxmox"

	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/httputil"
	okdproxmox "github.com/qxtaiba/okdctl/internal/proxmox"
)

const (
	proxmoxStatusOnline = "online"
	discoveryTimeout    = 10 * time.Second
)

type proxmoxNode struct {
	Name      string
	Status    string
	CPUs      int
	CPUsKnown bool
	MemGB     int
	MemKnown  bool
}

type proxmoxDiscovery struct {
	Nodes []proxmoxNode
}

// discoverProxmox lists the cluster's nodes with creds; parent bounds the
// fetch to the current wizard visit.
func discoverProxmox(parent context.Context, creds *credentials.ProxmoxCredentials) (*proxmoxDiscovery, error) {
	if creds.Endpoint == "" {
		return nil, errors.New("missing endpoint — enter it in the proxmox step")
	}
	if !creds.IsValid() {
		return nil, errors.New("missing api token — enter it in the proxmox step or set PROXMOX_VE_API_TOKEN in okdctl.env")
	}
	id, secret, ok := strings.Cut(string(creds.APIToken), "=")
	if !ok {
		return nil, errors.New("api token must be in user@realm!tokenname=secret form")
	}

	ctx, cancel := context.WithTimeout(parent, discoveryTimeout)
	defer cancel()
	client := proxmox.NewClient(
		okdproxmox.APIBaseURL(creds.Endpoint),
		proxmox.WithHTTPClient(httputil.NewOptionalInsecure(creds.Insecure, discoveryTimeout)),
		proxmox.WithAPIToken(id, secret),
	)

	rawNodes, err := client.Nodes(ctx)
	if err != nil {
		return nil, classifyError(err)
	}
	if len(rawNodes) == 0 {
		return nil, errors.New("no nodes found in cluster — check that the proxmox cluster has at least one node")
	}
	nodes := make([]proxmoxNode, 0, len(rawNodes))
	for _, n := range rawNodes {
		nodes = append(nodes, proxmoxNode{
			Name:      n.Node,
			Status:    n.Status,
			CPUs:      n.MaxCPU,
			CPUsKnown: true,
			MemGB:     int(n.MaxMem / (1024 * 1024 * 1024)), //nolint:gosec // G115: uint64→int is safe for GB-scale memory
			MemKnown:  true,
		})
	}
	return &proxmoxDiscovery{Nodes: nodes}, nil
}

// classifyError turns a raw discovery failure into an actionable message;
// each TLS cause gets its own safe remedy rather than a nudge to disable
// verification.
func classifyError(err error) error {
	var unknownAuthority x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthority) {
		return fmt.Errorf("tls certificate not trusted — install the proxmox host's ca certificate in your system trust store: %w", err)
	}
	var hostnameErr x509.HostnameError
	if errors.As(err, &hostnameErr) {
		return fmt.Errorf("tls certificate name mismatch — connect using a name on the certificate, or reissue the certificate for %q: %w", hostnameErr.Host, err)
	}
	var certInvalid x509.CertificateInvalidError
	if errors.As(err, &certInvalid) {
		if certInvalid.Reason == x509.Expired {
			return fmt.Errorf("tls certificate expired — renew the proxmox host's certificate: %w", err)
		}
		return fmt.Errorf("tls certificate invalid — check the proxmox host's certificate: %w", err)
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, "x509:") || strings.Contains(msg, "tls:"):
		return fmt.Errorf("tls handshake failed — check the proxmox host's certificate and tls configuration: %w", err)
	case strings.Contains(msg, "connection refused"):
		return errors.New("connection refused — check that the proxmox endpoint and port are correct")
	case strings.Contains(msg, "no such host"):
		return errors.New("host not found — check the proxmox endpoint")
	case strings.Contains(msg, "i/o timeout") || strings.Contains(msg, "context deadline"):
		return errors.New("connection timed out — check that the proxmox host is reachable")
	case strings.Contains(msg, "status 401") || strings.Contains(msg, "not authorized") || strings.Contains(msg, "authentication failure"):
		return errors.New("authentication failed — check the api token")
	default:
		return fmt.Errorf("connection failed — check proxmox connectivity and the api token: %w", err)
	}
}
