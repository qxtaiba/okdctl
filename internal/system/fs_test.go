package system

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// refuseSymlinkDst asserts the symlink-refusal contract shared by CopyFileMode and AtomicWrite.
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
		path, err := WriteTempFile("okdctl-test-*.txt", 0o600, func(_ *os.File) error {
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

	t.Run("refuses symlink at dst", func(t *testing.T) {
		refuseSymlinkDst(t, dir, func(link string) error {
			return CopyFileMode(src, link, 0o600)
		})
	})
}

func TestCopyFile(t *testing.T) {
	t.Run("preserves source permission bits", func(t *testing.T) {
		for _, mode := range []os.FileMode{0o640, 0o600, 0o644} {
			t.Run(mode.String(), func(t *testing.T) {
				dir := t.TempDir()
				src := filepath.Join(dir, "src")
				if err := os.WriteFile(src, []byte("payload"), mode); err != nil {
					t.Fatal(err)
				}
				// WriteFile's mode is masked by umask; Chmod pins it exactly so
				// the preservation assertion isn't loosened.
				if err := os.Chmod(src, mode); err != nil {
					t.Fatal(err)
				}
				dst := filepath.Join(dir, "dst")
				if err := CopyFile(src, dst); err != nil {
					t.Fatalf("CopyFile: %v", err)
				}
				fi, err := os.Stat(dst)
				if err != nil {
					t.Fatal(err)
				}
				if perm := fi.Mode().Perm(); perm != mode {
					t.Errorf("dst perm = %#o, want %#o", perm, mode)
				}
				body, _ := os.ReadFile(dst)
				if string(body) != "payload" {
					t.Errorf("body = %q, want payload", body)
				}
			})
		}
	})

	t.Run("missing source errors without creating dst", func(t *testing.T) {
		dir := t.TempDir()
		dst := filepath.Join(dir, "dst")
		if err := CopyFile(filepath.Join(dir, "missing.src"), dst); err == nil {
			t.Fatal("expected error for missing source")
		}
		if _, err := os.Stat(dst); !os.IsNotExist(err) {
			t.Errorf("dst created despite missing src: %v", err)
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

func TestSafeRemove(t *testing.T) {
	dir := t.TempDir()

	t.Run("missing", func(t *testing.T) {
		err := SafeRemove(filepath.Join(dir, "does-not-exist"))
		if err != nil {
			t.Fatalf("missing path: got %v, want nil", err)
		}
	})

	t.Run("regular_file", func(t *testing.T) {
		f := filepath.Join(dir, "file.txt")
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := SafeRemove(f); err != nil {
			t.Fatalf("regular file: %v", err)
		}
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("file still present after SafeRemove")
		}
	})

	t.Run("directory_tree", func(t *testing.T) {
		sub := filepath.Join(dir, "tree", "nested")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "leaf.txt"), []byte("y"), 0o600); err != nil {
			t.Fatal(err)
		}
		root := filepath.Join(dir, "tree")
		if err := SafeRemove(root); err != nil {
			t.Fatalf("directory tree: %v", err)
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Errorf("tree still present after SafeRemove")
		}
	})

	// os.RemoveAll on a symlink removes the link itself, not the target; this
	// proves SafeRemove doesn't follow it either.
	t.Run("symlink_to_target", func(t *testing.T) {
		target := filepath.Join(dir, "target.txt")
		if err := os.WriteFile(target, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(dir, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if err := SafeRemove(link); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		if _, err := os.Lstat(link); !os.IsNotExist(err) {
			t.Errorf("symlink still present after SafeRemove")
		}
		if _, err := os.Stat(target); err != nil {
			t.Errorf("target removed by SafeRemove; want it intact: %v", err)
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

// Dangling symlink is the tripwire: Lchown succeeds without resolving it, while
// os.Chown would hit ENOENT.
func TestChownByName_DoesNotFollowSymlink(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		t.Skipf("lookup group %s: %v", u.Gid, err)
	}
	dir := t.TempDir()
	link := filepath.Join(dir, "dangling")
	if err := os.Symlink(filepath.Join(dir, "gone"), link); err != nil {
		t.Skip("cannot create symlink:", err)
	}

	// Chown-to-self is always permitted, so this runs unprivileged.
	if err := ChownByName(link, u.Username+":"+g.Name); err != nil {
		t.Fatalf("ChownByName on dangling symlink: %v (a symlink-following rewrite fails here)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "gone")); !os.IsNotExist(err) {
		t.Fatalf("dangling target materialized: %v", err)
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

func TestExpandPath(t *testing.T) {
	t.Run("tilde_slash expands to invoking user home", func(t *testing.T) {
		cur, err := user.Current()
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("SUDO_USER", cur.Username)
		got := ExpandPath("~/x")
		want := filepath.Join(cur.HomeDir, "x")
		if got != want {
			t.Errorf("ExpandPath(~/x) = %q, want %q", got, want)
		}
	})

	t.Run("bare tilde passes through unchanged", func(t *testing.T) {
		t.Setenv("SUDO_USER", "")
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
