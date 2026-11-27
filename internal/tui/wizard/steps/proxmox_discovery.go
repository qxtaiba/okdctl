package steps

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"

	"github.com/qxtaiba/okdctl/internal/credentials"
	okdproxmox "github.com/qxtaiba/okdctl/internal/proxmox"
)

const (
	proxmoxStatusOnline = "online"
	discoveryTimeout    = 10 * time.Second
)

type proxmoxNode struct {
	Name   string
	Status string
	CPUs   int
	MemGB  int
}

type proxmoxDiscovery struct {
	Nodes []proxmoxNode
}

// discoverProxmox lists the cluster's nodes through the driver with creds
// under parent.
func discoverProxmox(parent context.Context, creds *credentials.ProxmoxCredentials) (*proxmoxDiscovery, error) {
	if creds.Endpoint == "" {
		return nil, errors.New("missing endpoint — enter it in the proxmox step")
	}
	if !creds.IsValid() {
		return nil, errors.New("missing api token — enter it in the proxmox step or set PROXMOX_VE_API_TOKEN in okdctl.env")
	}
	driver, err := okdproxmox.New(creds.Endpoint, creds.APIToken, creds.Insecure)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(parent, discoveryTimeout)
	defer cancel()
	infos, err := driver.Nodes(ctx)
	if err != nil {
		return nil, classifyError(err)
	}
	if len(infos) == 0 {
		return nil, errors.New("no nodes found in cluster — check that the proxmox cluster has at least one node")
	}
	nodes := make([]proxmoxNode, 0, len(infos))
	for i := range infos {
		n := &infos[i]
		nodes = append(nodes, proxmoxNode{
			Name:   n.Name,
			Status: n.Status,
			CPUs:   n.CPUs,
			MemGB:  n.MemTotalMiB / 1024,
		})
	}
	return &proxmoxDiscovery{Nodes: nodes}, nil
}

// classifyError prefixes each failure class with its remedy, each TLS cause
// getting its own rather than a nudge to disable verification; the cause
// stays in the chain and cancellation passes through untouched.
func classifyError(err error) error {
	var (
		unknownAuthority x509.UnknownAuthorityError
		hostnameErr      x509.HostnameError
		certInvalid      x509.CertificateInvalidError
		recordHeader     tls.RecordHeaderError
		alert            tls.AlertError
		dnsErr           *net.DNSError
		netErr           net.Error
	)
	switch {
	case errors.Is(err, context.Canceled):
		return err
	case errors.Is(err, okdproxmox.ErrPermission):
		return fmt.Errorf("authentication failed — check the api token: %w", err)
	case errors.As(err, &unknownAuthority):
		return fmt.Errorf("tls certificate not trusted — install the proxmox host's ca certificate in your system trust store: %w", err)
	case errors.As(err, &hostnameErr):
		return fmt.Errorf("tls certificate name mismatch — connect using a name on the certificate, or reissue the certificate for %q: %w", hostnameErr.Host, err)
	case errors.As(err, &certInvalid) && certInvalid.Reason == x509.Expired:
		return fmt.Errorf("tls certificate expired — renew the proxmox host's certificate: %w", err)
	case errors.As(err, &certInvalid):
		return fmt.Errorf("tls certificate invalid — check the proxmox host's certificate: %w", err)
	case errors.As(err, &recordHeader), errors.As(err, &alert):
		return fmt.Errorf("tls handshake failed — check the proxmox host's certificate and tls configuration: %w", err)
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return fmt.Errorf("connection timed out — check that the proxmox host is reachable: %w", err)
	case errors.As(err, &dnsErr):
		return fmt.Errorf("host not found — check the proxmox endpoint: %w", err)
	case errors.Is(err, syscall.ECONNREFUSED):
		return fmt.Errorf("connection refused — check that the proxmox endpoint and port are correct: %w", err)
	}
	return fmt.Errorf("connection failed — check proxmox connectivity and the api token: %w", err)
}
