package provision

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/qxtaiba/okdctl/internal/system"
)

const (
	certValidityYears = 2
	certDirName       = "certs/ignition"
	certFileName      = "server.crt"
	certKeyName       = "server.key"
)

// IgnitionCertPaths returns the on-disk cert and private-key paths under projectRoot.
func IgnitionCertPaths(projectRoot string) (certPath, keyPath string) {
	dir := filepath.Join(projectRoot, certDirName)
	return filepath.Join(dir, certFileName), filepath.Join(dir, certKeyName)
}

// EnsureIgnitionCert returns the PEM-encoded cert and key for the ignition
// HTTPS server, reusing a valid unexpired cert covering ip or generating a
// new ECDSA P-256 self-signed one. The cert doubles as the CA embedded into
// each node's ISO via --ignition-ca, so a self-signed leaf suffices since
// it's presented only to that one baked-in trust anchor.
func EnsureIgnitionCert(projectRoot, ip string) (certPEM, keyPEM []byte, err error) {
	certPath, keyPath := IgnitionCertPaths(projectRoot)

	if existing, key, ok := loadExistingCert(certPath, keyPath, ip); ok {
		return existing, key, nil
	}

	return generateSelfSignedCert(certPath, keyPath, ip)
}

func loadExistingCert(certPath, keyPath, ip string) (certPEM, keyPEM []byte, ok bool) {
	certRaw, err := os.ReadFile(certPath)
	if err != nil {
		return nil, nil, false
	}
	keyRaw, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, false
	}

	block, _ := pem.Decode(certRaw)
	if block == nil {
		return nil, nil, false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, false
	}

	if time.Now().After(cert.NotAfter) {
		return nil, nil, false
	}

	if want := net.ParseIP(ip); want != nil && slices.ContainsFunc(cert.IPAddresses, want.Equal) {
		return certRaw, keyRaw, true
	}
	if cert.Subject.CommonName == ip {
		return certRaw, keyRaw, true
	}

	return nil, nil, false
}

func generateSelfSignedCert(certPath, keyPath, ip string) (certPEM, keyPEM []byte, err error) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return nil, nil, fmt.Errorf("ignition cert host %q is not a valid IP address", ip)
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate ecdsa key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate serial: %w", err)
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: ip},
		NotBefore:             now.Add(-1 * time.Minute),
		NotAfter:              now.Add(certValidityYears * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{parsed},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal ec key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	dir := filepath.Dir(certPath)
	if err := system.EnsureDir(dir); err != nil {
		return nil, nil, fmt.Errorf("ensure cert dir: %w", err)
	}
	// Runs as root over a path the unprivileged invoker owns; os.Chmod follows
	// a pre-planted symlink, so refuse one first, matching system/fs.go.
	if info, err := os.Lstat(dir); err != nil {
		return nil, nil, fmt.Errorf("lstat cert dir: %w", err)
	} else if info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, fmt.Errorf("cert dir %q is a symlink; refusing to chmod: %w", dir, os.ErrPermission)
	}
	// 0o700: keep the private key non-enumerable by other users on the bastion.
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("tighten cert dir perms: %w", err)
	}
	if err := system.AtomicWrite(certPath, certPEM, 0o644); err != nil {
		return nil, nil, fmt.Errorf("write cert: %w", err)
	}
	// 0o600: private key must not be group- or world-readable.
	if err := system.AtomicWrite(keyPath, keyPEM, 0o600); err != nil {
		return nil, nil, fmt.Errorf("write key: %w", err)
	}

	return certPEM, keyPEM, nil
}
