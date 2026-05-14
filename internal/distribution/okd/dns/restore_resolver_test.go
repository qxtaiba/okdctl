package dns

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

func TestRestoreSystemResolver_MissingDropIn_IsNoOp(t *testing.T) {
	dir := t.TempDir()
	orig := resolvedConf
	resolvedConf = filepath.Join(dir, "dnsmasq.conf")
	t.Cleanup(func() { resolvedConf = orig })

	if err := RestoreSystemResolver(context.Background(), logutil.NopLogger); err != nil {
		t.Fatalf("RestoreSystemResolver: %v", err)
	}

	if _, err := os.Stat(resolvedConf); !os.IsNotExist(err) {
		t.Errorf("expected drop-in to remain absent, got stat err: %v", err)
	}
}

func TestRestoreSystemResolver_PresentDropIn_IsRemoved(t *testing.T) {
	dir := t.TempDir()
	orig := resolvedConf
	resolvedConf = filepath.Join(dir, "dnsmasq.conf")
	t.Cleanup(func() { resolvedConf = orig })

	if err := os.WriteFile(resolvedConf, []byte("[Resolve]\nDNS=127.0.0.1\n"), 0o644); err != nil {
		t.Fatalf("seed drop-in: %v", err)
	}

	if err := RestoreSystemResolver(context.Background(), logutil.NopLogger); err != nil {
		t.Fatalf("RestoreSystemResolver: %v", err)
	}

	if _, err := os.Stat(resolvedConf); !os.IsNotExist(err) {
		t.Errorf("expected drop-in to be removed, got stat err: %v", err)
	}
}

func TestRestoreSystemResolver_RemoveAllError_LoggedNotPropagated(t *testing.T) {
	dir := t.TempDir()
	origConf := resolvedConf
	resolvedConf = filepath.Join(dir, "dnsmasq.conf")
	t.Cleanup(func() { resolvedConf = origConf })

	if err := os.WriteFile(resolvedConf, []byte("[Resolve]\nDNS=127.0.0.1\n"), 0o644); err != nil {
		t.Fatalf("seed drop-in: %v", err)
	}

	sentinel := errors.New("injected remove error")
	origFn := removeAllFn
	removeAllFn = func(_ string) error { return sentinel }
	t.Cleanup(func() { removeAllFn = origFn })

	if err := RestoreSystemResolver(context.Background(), logutil.NopLogger); err != nil {
		t.Fatalf("RestoreSystemResolver must not propagate RemoveAll error, got: %v", err)
	}
}
