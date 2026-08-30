package cluster

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func signerCertPEM(t *testing.T, notAfter time.Time) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "kube-apiserver-to-kubelet-signer"},
		NotBefore:    notAfter.Add(-365 * 24 * time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func signerSecretJSON(t *testing.T, notAfter time.Time) []byte {
	t.Helper()
	secret, err := json.Marshal(map[string]any{
		"data": map[string]string{"tls.crt": base64.StdEncoding.EncodeToString(signerCertPEM(t, notAfter))},
	})
	if err != nil {
		t.Fatal(err)
	}
	return secret
}

func TestParseSignerNotAfter(t *testing.T) {
	want := time.Now().Add(400 * 24 * time.Hour).Truncate(time.Second)
	got, err := parseSignerNotAfter(signerSecretJSON(t, want))
	if err != nil {
		t.Fatalf("parseSignerNotAfter: %v", err)
	}
	if !got.Equal(want) {
		t.Errorf("NotAfter = %v; want %v", got, want)
	}
}

func TestParseSignerNotAfter_MissingCrt(t *testing.T) {
	if _, err := parseSignerNotAfter([]byte(`{"data":{}}`)); err == nil {
		t.Fatal("expected error when tls.crt absent")
	}
}
