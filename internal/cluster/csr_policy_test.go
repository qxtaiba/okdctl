package cluster

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net"
	"slices"
	"testing"

	certificatesv1 "k8s.io/api/certificates/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/model"
)

func TestExpectedCSRIdentities(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Workers.Count = 1
	inv, err := model.Build(cfg, "lab-x7k2q")
	if err != nil {
		t.Fatal(err)
	}
	identities, err := ExpectedCSRIdentities(&inv, "lab.k8s.local")
	if err != nil {
		t.Fatal(err)
	}
	if len(identities) != 4 {
		t.Fatalf("identities = %+v", identities)
	}
	want := CSRIdentity{Names: []string{"lab-worker0", "lab-worker0.lab.k8s.local"}, IP: "192.168.1.160"}
	if got := identities[3]; !slices.Equal(got.Names, want.Names) || got.IP != want.IP {
		t.Errorf("worker identity = %+v; want %+v", got, want)
	}
	if _, err := ExpectedCSRIdentities(&model.Inventory{}, "lab.k8s.local"); err == nil {
		t.Error("empty inventory accepted")
	}
	if _, err := ExpectedCSRIdentities(nil, "lab.k8s.local"); err == nil {
		t.Error("nil inventory accepted")
	}
}

func testCSRIdentity() CSRIdentity {
	return CSRIdentity{Names: []string{"lab-worker0", "lab-worker0.lab.k8s.local"}, IP: "192.0.2.10"}
}

func signedCSR(t *testing.T, mutate func(*x509.CertificateRequest)) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	request := &x509.CertificateRequest{Subject: pkix.Name{CommonName: "system:node:lab-worker0", Organization: []string{"system:nodes"}}}
	if mutate != nil {
		mutate(request)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, request, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
}

func testCSR(t *testing.T) certificatesv1.CertificateSigningRequest {
	t.Helper()
	return certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "csr-1", UID: "observed-uid", ResourceVersion: "17"},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request: signedCSR(t, nil), SignerName: certificatesv1.KubeAPIServerClientKubeletSignerName,
			Username: "system:serviceaccount:openshift-machine-config-operator:node-bootstrapper",
			Groups:   []string{"system:serviceaccounts:openshift-machine-config-operator", "system:serviceaccounts", "system:authenticated"},
			Usages:   []certificatesv1.KeyUsage{certificatesv1.UsageDigitalSignature, certificatesv1.UsageClientAuth},
		},
	}
}

func TestCSRPolicy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(*certificatesv1.CertificateSigningRequest)
		allowed bool
	}{
		{"bootstrap", func(*certificatesv1.CertificateSigningRequest) {}, true},
		{"rotation", func(c *certificatesv1.CertificateSigningRequest) {
			c.Spec.Username = "system:node:lab-worker0"
			c.Spec.Groups = []string{"system:nodes", "system:authenticated"}
		}, true},
		{"foreign signer", func(c *certificatesv1.CertificateSigningRequest) { c.Spec.SignerName = "example.com/other" }, false},
		{"foreign requester", func(c *certificatesv1.CertificateSigningRequest) { c.Spec.Username = "intruder" }, false},
		{"extra group", func(c *certificatesv1.CertificateSigningRequest) {
			c.Spec.Groups = append(c.Spec.Groups, "system:masters")
		}, false},
		{"extra usage", func(c *certificatesv1.CertificateSigningRequest) {
			c.Spec.Usages = append(c.Spec.Usages, certificatesv1.UsageCertSign)
		}, false},
		{"foreign node", func(c *certificatesv1.CertificateSigningRequest) {
			c.Spec.Request = signedCSR(t, func(r *x509.CertificateRequest) { r.Subject.CommonName = "system:node:lab-worker9" })
		}, false},
		{"privileged organization", func(c *certificatesv1.CertificateSigningRequest) {
			c.Spec.Request = signedCSR(t, func(r *x509.CertificateRequest) { r.Subject.Organization = []string{"system:masters"} })
		}, false},
		{"client SAN", func(c *certificatesv1.CertificateSigningRequest) {
			c.Spec.Request = signedCSR(t, func(r *x509.CertificateRequest) { r.DNSNames = []string{"lab-worker0"} })
		}, false},
		{"malformed", func(c *certificatesv1.CertificateSigningRequest) { c.Spec.Request = []byte("invalid") }, false},
		{"missing version", func(c *certificatesv1.CertificateSigningRequest) { c.ResourceVersion = "" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testCSR(t)
			tc.mutate(&c)
			err := validateCSR(&c, []CSRIdentity{testCSRIdentity()})
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v error=%v", tc.allowed, err)
			}
		})
	}
}

func TestCSRServingPolicy(t *testing.T) {
	for _, ip := range []string{"192.0.2.10", "192.0.2.11"} {
		c := testCSR(t)
		c.Spec.SignerName = certificatesv1.KubeletServingSignerName
		c.Spec.Username = "system:node:lab-worker0"
		c.Spec.Groups = []string{"system:nodes", "system:authenticated"}
		c.Spec.Usages = []certificatesv1.KeyUsage{certificatesv1.UsageDigitalSignature, certificatesv1.UsageServerAuth}
		c.Spec.Request = signedCSR(t, func(r *x509.CertificateRequest) {
			r.DNSNames = []string{"lab-worker0"}
			r.IPAddresses = []net.IP{net.ParseIP(ip)}
		})
		if err := validateCSR(&c, []CSRIdentity{testCSRIdentity()}); (err == nil) != (ip == "192.0.2.10") {
			t.Fatalf("IP %s: %v", ip, err)
		}
		if err := validateCSR(&c, nil); err == nil {
			t.Fatal("empty policy accepted request")
		}
	}
}
