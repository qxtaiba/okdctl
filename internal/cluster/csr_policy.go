package cluster

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"slices"
	"strings"

	certificatesv1 "k8s.io/api/certificates/v1"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

const (
	csrNodeGroup          = "system:nodes"
	csrAuthenticatedGroup = "system:authenticated"
)

// CSRIdentity bounds node certificate names and addresses to the active operation.
type CSRIdentity struct {
	Names []string
	IP    string
}

// ExpectedCSRIdentities derives provisioned identities, optionally restricted to one node.
func ExpectedCSRIdentities(cfg *config.Config, target string) ([]CSRIdentity, error) {
	nodes, err := nodetypes.ClusterNodes(cfg)
	if err != nil {
		return nil, err
	}
	var identities []CSRIdentity
	for _, node := range nodes {
		if node.Role == nodetypes.RoleBootstrap {
			continue
		}
		domain := cfg.Cluster.Name + "." + cfg.Cluster.Domain
		names := []string{node.Name(), node.PrefixedName(cfg.Cluster.Name), node.Name() + "." + domain, node.PrefixedName(cfg.Cluster.Name) + "." + domain}
		if target != "" && !slices.Contains(names, target) {
			continue
		}
		identities = append(identities, CSRIdentity{Names: names, IP: node.IP})
	}
	if len(identities) == 0 {
		return nil, errors.New("no expected node identities for CSR approval")
	}
	return identities, nil
}

func validateCSR(csr *certificatesv1.CertificateSigningRequest, identities []CSRIdentity) error {
	if csr.Name == "" || csr.UID == "" || csr.ResourceVersion == "" {
		return errors.New("missing CSR object identity")
	}
	block, rest := pem.Decode(csr.Spec.Request)
	if block == nil || block.Type != "CERTIFICATE REQUEST" || strings.TrimSpace(string(rest)) != "" {
		return errors.New("invalid CSR encoding")
	}
	request, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse CSR: %w", err)
	}
	if err := request.CheckSignature(); err != nil {
		return fmt.Errorf("verify CSR signature: %w", err)
	}
	if !slices.Equal(request.Subject.Organization, []string{csrNodeGroup}) || !strings.HasPrefix(request.Subject.CommonName, "system:node:") {
		return errors.New("unexpected certificate subject")
	}
	for _, extension := range request.Extensions {
		if extension.Id.String() != "2.5.29.17" {
			return errors.New("unexpected CSR extension")
		}
	}
	name := strings.TrimPrefix(request.Subject.CommonName, "system:node:")
	index := slices.IndexFunc(identities, func(identity CSRIdentity) bool { return slices.Contains(identity.Names, name) })
	if index < 0 {
		return errors.New("node outside active operation")
	}
	if len(request.EmailAddresses) > 0 || len(request.URIs) > 0 {
		return errors.New("unexpected email or URI SAN")
	}
	switch csr.Spec.SignerName {
	case certificatesv1.KubeAPIServerClientKubeletSignerName:
		if !validCSRUsages(csr.Spec.Usages, certificatesv1.UsageClientAuth) || len(request.DNSNames) > 0 || len(request.IPAddresses) > 0 {
			return errors.New("unexpected client certificate usages or SANs")
		}
		bootstrap := csr.Spec.Username == "system:serviceaccount:openshift-machine-config-operator:node-bootstrapper" &&
			sameStrings(csr.Spec.Groups, []string{"system:serviceaccounts:openshift-machine-config-operator", "system:serviceaccounts", csrAuthenticatedGroup})
		rotation := csr.Spec.Username == request.Subject.CommonName && sameStrings(csr.Spec.Groups, []string{csrNodeGroup, csrAuthenticatedGroup})
		if !bootstrap && !rotation {
			return errors.New("unexpected client requester")
		}
	case certificatesv1.KubeletServingSignerName:
		if csr.Spec.Username != request.Subject.CommonName || !sameStrings(csr.Spec.Groups, []string{csrNodeGroup, csrAuthenticatedGroup}) || !validCSRUsages(csr.Spec.Usages, certificatesv1.UsageServerAuth) {
			return errors.New("unexpected serving requester or usages")
		}
		return validateServingSANs(request, identities[index])
	default:
		return errors.New("unexpected CSR signer")
	}
	return nil
}

func validCSRUsages(usages []certificatesv1.KeyUsage, auth certificatesv1.KeyUsage) bool {
	if !slices.Contains(usages, certificatesv1.UsageDigitalSignature) || !slices.Contains(usages, auth) {
		return false
	}
	if len(usages) < 2 || len(usages) > 3 {
		return false
	}
	seen := make(map[certificatesv1.KeyUsage]bool, len(usages))
	for _, usage := range usages {
		if seen[usage] {
			return false
		}
		seen[usage] = true
		if usage != certificatesv1.UsageDigitalSignature && usage != certificatesv1.UsageKeyEncipherment && usage != auth {
			return false
		}
	}
	return true
}

func sameStrings(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

func validateServingSANs(request *x509.CertificateRequest, identity CSRIdentity) error {
	if len(request.DNSNames)+len(request.IPAddresses) == 0 {
		return errors.New("missing serving SANs")
	}
	for _, name := range request.DNSNames {
		if !slices.Contains(identity.Names, name) {
			return errors.New("DNS SAN outside expected node")
		}
	}
	for _, ip := range request.IPAddresses {
		if ip.String() != identity.IP {
			return errors.New("IP SAN outside expected node")
		}
	}
	return nil
}
