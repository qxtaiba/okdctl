package setup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func TestIsAuthError(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"401 unauthorized", true},
		{"UNAUTHORIZED: access to the requested resource is not authorized", true},
		{"Forbidden", true},
		{"no basic auth provided", true},
		{"connection refused", false},
		{"", false},
	}
	for _, tt := range cases {
		if got := isAuthError(tt.msg); got != tt.want {
			t.Errorf("isAuthError(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

func makeOCTarGz(t *testing.T, ocContent []byte) (archive []byte, sha256Hex string) {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	hdr := &tar.Header{
		Name:     "oc",
		Typeflag: tar.TypeReg,
		Mode:     0o755,
		Size:     int64(len(ocContent)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("tar WriteHeader: %v", err)
	}
	if _, err := tw.Write(ocContent); err != nil {
		t.Fatalf("tar Write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip Close: %v", err)
	}
	raw := buf.Bytes()
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:])
}

func TestBootstrapOC_cachedSkipsFetch(t *testing.T) {
	dir := t.TempDir()
	cached := filepath.Join(dir, "oc")
	if err := os.WriteFile(cached, []byte("#!/bin/sh\necho cached oc"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Unroutable host: a real fetch would fail, forcing the cache path.
	oldBase := bootstrapOCBaseURL
	bootstrapOCBaseURL = "http://127.0.0.1:1"
	t.Cleanup(func() { bootstrapOCBaseURL = oldBase })

	p := newTestPhase(t)
	ocPath, err := p.bootstrapOC(context.Background(), dir)
	if err != nil {
		t.Fatalf("bootstrapOC: %v", err)
	}
	if ocPath != cached {
		t.Errorf("ocPath = %q, want %q", ocPath, cached)
	}
}

func TestBootstrapOC_checksumMismatch(t *testing.T) {
	tarball, _ := makeOCTarGz(t, []byte("tampered binary content"))
	wrongChecksum := "0000000000000000000000000000000000000000000000000000000000000000"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarball)
	}))
	t.Cleanup(srv.Close)

	oldBase := bootstrapOCBaseURL
	bootstrapOCBaseURL = srv.URL
	t.Cleanup(func() { bootstrapOCBaseURL = oldBase })

	oldChecksum := bootstrapOCChecksum
	bootstrapOCChecksum = wrongChecksum
	t.Cleanup(func() { bootstrapOCChecksum = oldChecksum })

	p := newTestPhase(t)
	_, err := p.bootstrapOC(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error for checksum mismatch, got nil")
	}
	var ne *errtypes.NetworkError
	if !errors.As(err, &ne) {
		t.Errorf("want *errtypes.NetworkError, got %T: %v", err, err)
	}
}
