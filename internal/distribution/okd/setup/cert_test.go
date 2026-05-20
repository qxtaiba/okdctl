package setup

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGenerateSelfSignedCert_FilePerms(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	root := t.TempDir()
	certPath, keyPath := IgnitionCertPaths(root)

	if _, _, err := generateSelfSignedCert(certPath, keyPath, "192.0.2.1"); err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}

	dir := filepath.Dir(certPath)
	di, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if got := di.Mode().Perm(); got != 0o700 {
		t.Errorf("dir perm = %04o, want 0700", got)
	}

	ci, err := os.Stat(certPath)
	if err != nil {
		t.Fatalf("stat cert: %v", err)
	}
	if got := ci.Mode().Perm(); got != 0o644 {
		t.Errorf("cert perm = %04o, want 0644", got)
	}

	ki, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key: %v", err)
	}
	if got := ki.Mode().Perm(); got != 0o600 {
		t.Errorf("key perm = %04o, want 0600", got)
	}
}

func TestLoadExistingCert_ExpiredReturnsNotOK(t *testing.T) {
	root := t.TempDir()
	certPath, keyPath := IgnitionCertPaths(root)

	if err := os.MkdirAll(filepath.Dir(certPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	past := time.Now().Add(-48 * time.Hour)
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "192.0.2.1"},
		NotBefore:             past.Add(-24 * time.Hour),
		NotAfter:              past,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("192.0.2.1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, _ := x509.MarshalECPrivateKey(priv)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	_, _, ok := loadExistingCert(certPath, keyPath, "192.0.2.1")
	if ok {
		t.Error("loadExistingCert returned ok=true for expired cert; want false")
	}
}

func TestLoadExistingCert_IPMismatchReturnsNotOK(t *testing.T) {
	root := t.TempDir()
	certPath, keyPath := IgnitionCertPaths(root)

	if _, _, err := generateSelfSignedCert(certPath, keyPath, "192.0.2.1"); err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}

	_, _, ok := loadExistingCert(certPath, keyPath, "192.0.2.2")
	if ok {
		t.Error("loadExistingCert returned ok=true for non-matching IP; want false")
	}
}

func TestLoadExistingCert_MatchingIPSANReturnsOK(t *testing.T) {
	root := t.TempDir()
	certPath, keyPath := IgnitionCertPaths(root)

	if _, _, err := generateSelfSignedCert(certPath, keyPath, "192.0.2.1"); err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}

	_, _, ok := loadExistingCert(certPath, keyPath, "192.0.2.1")
	if !ok {
		t.Error("loadExistingCert returned ok=false for matching IP-SAN; want true")
	}
}

func TestGenerateSelfSignedCert_RejectsNonIP(t *testing.T) {
	root := t.TempDir()
	certPath, keyPath := IgnitionCertPaths(root)

	_, _, err := generateSelfSignedCert(certPath, keyPath, "not-an-ip")
	if err == nil {
		t.Fatal("generateSelfSignedCert accepted non-IP argument; want error")
	}
}

func TestGenerateSelfSignedCert_X509RoundTrip(t *testing.T) {
	root := t.TempDir()
	certPath, keyPath := IgnitionCertPaths(root)

	certPEM, _, err := generateSelfSignedCert(certPath, keyPath, "192.0.2.1")
	if err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("returned certPEM is not valid PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("x509.ParseCertificate: %v", err)
	}
	if !cert.BasicConstraintsValid {
		t.Error("cert.BasicConstraintsValid = false; want true")
	}
	if !cert.IsCA {
		t.Error("cert.IsCA = false; want true")
	}
}
