package runlock_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/runlock"
)

func TestLockBodyContainsHostname(t *testing.T) {
	dir := t.TempDir()
	lock, err := runlock.Acquire(dir, "deploy")
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}
	defer lock.Release()

	body, err := os.ReadFile(filepath.Join(dir, ".okdctl.lock"))
	if err != nil {
		t.Fatalf("read lock file: %v", err)
	}
	s := string(body)
	if !strings.Contains(s, "HOST=") {
		t.Fatalf("lock body missing HOST= field: %q", s)
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "unknown"
	}
	if !strings.Contains(s, "HOST="+host) {
		t.Fatalf("lock body HOST= value mismatch: want %q in %q", "HOST="+host, s)
	}
}

func TestConflictMessageContainsHostname(t *testing.T) {
	dir := t.TempDir()
	first, err := runlock.Acquire(dir, "deploy")
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	defer first.Release()

	_, err = runlock.Acquire(dir, "destroy")
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *errtypes.ConfigError, got %T: %v", err, err)
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "unknown"
	}
	if !strings.Contains(cfgErr.Msg, host) {
		t.Fatalf("conflict message missing hostname %q: %q", host, cfgErr.Msg)
	}
}

func TestAcquireAndRelease(t *testing.T) {
	dir := t.TempDir()
	lock, err := runlock.Acquire(dir, "deploy")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	lockPath := filepath.Join(dir, ".okdctl.lock")
	if _, statErr := os.Stat(lockPath); statErr != nil {
		t.Fatalf("lock file not created: %v", statErr)
	}
	lock.Release()
	if _, statErr := os.Stat(lockPath); !os.IsNotExist(statErr) {
		t.Fatal("lock file not removed after Release")
	}
}

func TestConflictReturnsConfigError(t *testing.T) {
	dir := t.TempDir()
	first, err := runlock.Acquire(dir, "deploy")
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	defer first.Release()

	_, err = runlock.Acquire(dir, "destroy")
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *errtypes.ConfigError, got %T: %v", err, err)
	}
}
