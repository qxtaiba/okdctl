package runlock_test

import (
	"errors"
	"fmt"
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

	body, readErr := os.ReadFile(lockPath)
	if readErr != nil {
		t.Fatalf("read lock file: %v", readErr)
	}
	s := string(body)
	pid := fmt.Sprintf("PID=%d", os.Getpid())
	if !strings.Contains(s, pid) {
		t.Fatalf("lock body missing %s: %q", pid, s)
	}
	if !strings.Contains(s, "VERB=deploy") {
		t.Fatalf("lock body missing VERB=deploy: %q", s)
	}
	if !strings.Contains(s, "TIME=") {
		t.Fatalf("lock body missing TIME= field: %q", s)
	}

	lock.Release()
	if _, statErr := os.Stat(lockPath); !os.IsNotExist(statErr) {
		t.Fatal("lock file not removed after Release")
	}
}

func TestConflictMessageContainsPID(t *testing.T) {
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
	pid := fmt.Sprintf("PID=%d", os.Getpid())
	if !strings.Contains(cfgErr.Msg, pid) {
		t.Fatalf("conflict message missing holder PID %s: %q", pid, cfgErr.Msg)
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

func TestAcquire_RefusesSymlink(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses symlink restrictions; test is only meaningful as non-root")
	}
	dir := t.TempDir()
	link := filepath.Join(dir, ".okdctl.lock")
	if err := os.Symlink("/tmp/okdctl-sentinel", link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	_, err := runlock.Acquire(dir, "deploy")
	if err == nil {
		t.Fatal("Acquire accepted a symlink at the lock path; symlink-refusal guard regressed")
	}
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *errtypes.ConfigError, got %T: %v", err, err)
	}
	if !strings.Contains(cfgErr.Msg, "symlink") {
		t.Fatalf("ConfigError.Msg does not name rejection reason: %q", cfgErr.Msg)
	}
}
