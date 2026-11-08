package system

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// refuseSymlinkDst asserts AtomicWrite refuses a symlink at the destination.
func refuseSymlinkDst(t *testing.T, dir string, op func(link string) error) {
	t.Helper()
	target := filepath.Join(dir, "symlink-target.txt")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "symlink-dst.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	err := op(link)
	if err == nil {
		t.Fatal("write via symlink dst: expected error, got nil")
	}
	var authErr *errtypes.AuthError
	if !errors.As(err, &authErr) {
		t.Errorf("err type = %T, want *errtypes.AuthError", err)
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("err does not wrap os.ErrPermission: %v", err)
	}
	body, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read target: %v", readErr)
	}
	if string(body) != "original" {
		t.Errorf("target body = %q; symlink target was overwritten", body)
	}
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()

	t.Run("writes data at perm, survives reads", func(t *testing.T) {
		path := filepath.Join(dir, "a.txt")
		if err := AtomicWrite(path, []byte("payload"), 0o600); err != nil {
			t.Fatal(err)
		}
		fi, _ := os.Stat(path)
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Errorf("perm = %#o, want 0o600", perm)
		}
		body, _ := os.ReadFile(path)
		if string(body) != "payload" {
			t.Errorf("body = %q", body)
		}
	})

	t.Run("creates parent directory", func(t *testing.T) {
		path := filepath.Join(dir, "nested", "deep", "a.txt")
		if err := AtomicWrite(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("AtomicWrite: %v", err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("file not created: %v", err)
		}
	})

	t.Run("overwrites existing file atomically", func(t *testing.T) {
		path := filepath.Join(dir, "replace.txt")
		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := AtomicWrite(path, []byte("new"), 0o600); err != nil {
			t.Fatal(err)
		}
		body, _ := os.ReadFile(path)
		if string(body) != "new" {
			t.Errorf("body = %q", body)
		}
	})

	t.Run("refuses symlink target", func(t *testing.T) {
		refuseSymlinkDst(t, dir, func(link string) error {
			return AtomicWrite(link, []byte("overwrite"), 0o600)
		})
	})

	t.Run("no .tmp-* leftovers after success", func(t *testing.T) {
		path := filepath.Join(dir, "leftover-probe.txt")
		if err := AtomicWrite(path, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".tmp-") {
				t.Errorf("temp leftover: %s", e.Name())
			}
		}
	})
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !FileExists(f) {
		t.Errorf("FileExists(%q) = false", f)
	}
	if FileExists(dir) {
		t.Errorf("FileExists(dir) = true; want false")
	}
}

func TestExpandPath(t *testing.T) {
	t.Run("tilde_slash expands to the user home", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		if got, want := ExpandPath("~/x"), filepath.Join(home, "x"); got != want {
			t.Errorf("ExpandPath(~/x) = %q, want %q", got, want)
		}
	})

	t.Run("bare tilde passes through unchanged", func(t *testing.T) {
		if got := ExpandPath("~"); got != "~" {
			t.Errorf("ExpandPath(~) = %q, want ~", got)
		}
	})
}

func TestMakeExecutable(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name     string
		initial  os.FileMode
		wantPerm os.FileMode
	}{
		{"0o600_becomes_0o711", 0o600, 0o711},
		{"0o644_becomes_0o755", 0o644, 0o755},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name)
			if err := os.WriteFile(path, []byte("binary"), tc.initial); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, tc.initial); err != nil {
				t.Fatal(err)
			}
			if err := MakeExecutable(path); err != nil {
				t.Fatalf("MakeExecutable: %v", err)
			}
			fi, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if perm := fi.Mode().Perm(); perm != tc.wantPerm {
				t.Errorf("perm = %#o, want %#o", perm, tc.wantPerm)
			}
		})
	}

	t.Run("missing_path_error_contains_path", func(t *testing.T) {
		missing := filepath.Join(dir, "does-not-exist")
		err := MakeExecutable(missing)
		if err == nil {
			t.Fatal("expected error for missing path, got nil")
		}
		if !strings.Contains(err.Error(), missing) {
			t.Errorf("error %q does not contain path %q", err.Error(), missing)
		}
	})
}
