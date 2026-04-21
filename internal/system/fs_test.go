package system

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteTempFile(t *testing.T) {
	t.Run("success path creates file at mode and invokes writeFn", func(t *testing.T) {
		path, err := WriteTempFile("okdctl-test-*.txt", 0o600, func(f *os.File) error {
			_, err := f.WriteString("hello")
			return err
		})
		if err != nil {
			t.Fatalf("WriteTempFile: %v", err)
		}
		defer os.Remove(path)

		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Errorf("perm = %#o, want 0o600", perm)
		}
		body, _ := os.ReadFile(path)
		if string(body) != "hello" {
			t.Errorf("body = %q, want hello", body)
		}
	})

	t.Run("writeFn error triggers cleanup", func(t *testing.T) {
		sentinel := errors.New("boom")
		path, err := WriteTempFile("okdctl-test-*.txt", 0o600, func(f *os.File) error {
			return sentinel
		})
		if err == nil || !errors.Is(err, sentinel) {
			t.Errorf("err = %v, want sentinel propagated", err)
		}
		if path != "" {
			t.Errorf("path = %q, want empty on error", path)
		}
	})

}

func TestCopyFileMode(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(src, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("creates dst at requested mode, never permissive", func(t *testing.T) {
		dst := filepath.Join(dir, "sub", "dst.txt")
		if err := CopyFileMode(src, dst, 0o600); err != nil {
			t.Fatalf("CopyFileMode: %v", err)
		}
		fi, err := os.Stat(dst)
		if err != nil {
			t.Fatal(err)
		}
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Errorf("perm = %#o, want 0o600", perm)
		}
		body, _ := os.ReadFile(dst)
		if string(body) != "payload" {
			t.Errorf("body = %q", body)
		}
	})

	t.Run("pre-existing dst with wider perms is tightened", func(t *testing.T) {
		dst := filepath.Join(dir, "pre-existing.txt")
		if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := CopyFileMode(src, dst, 0o600); err != nil {
			t.Fatalf("CopyFileMode: %v", err)
		}
		fi, _ := os.Stat(dst)
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Errorf("pre-existing dst perm = %#o, want 0o600 (tightened)", perm)
		}
	})

	t.Run("missing source returns error without touching dst", func(t *testing.T) {
		dst := filepath.Join(dir, "nope.txt")
		err := CopyFileMode(filepath.Join(dir, "missing.src"), dst, 0o600)
		if err == nil {
			t.Fatal("expected error")
		}
		if _, err := os.Stat(dst); !os.IsNotExist(err) {
			t.Errorf("dst created despite src failure: %v", err)
		}
	})
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

func TestChownByName(t *testing.T) {
	cases := []struct {
		name    string
		spec    string
		wantErr bool
	}{
		{"rejects empty", "", true},
		{"rejects no colon", "alice", true},
		{"rejects empty user", ":group", true},
		{"rejects empty group", "user:", true},
		{"rejects numeric-only user", "1000:staff", true},
		{"rejects numeric-only group", "alice:1000", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ChownByName("/tmp/does-not-matter", tc.spec)
			if tc.wantErr && err == nil {
				t.Errorf("expected error for spec %q", tc.spec)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for spec %q: %v", tc.spec, err)
			}
		})
	}
}

func TestFileExists_DirExists(t *testing.T) {
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
	if !DirExists(dir) {
		t.Errorf("DirExists(dir) = false")
	}
	if DirExists(f) {
		t.Errorf("DirExists(file) = true")
	}
}
