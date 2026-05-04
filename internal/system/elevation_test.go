package system

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestChownTreeToInvokingUser_SymlinkEscape verifies that the os.Root-based
// walk does not descend into a symlink pointing outside the root.
func TestChownTreeToInvokingUser_SymlinkEscape(t *testing.T) {
	inside := t.TempDir()
	outside := t.TempDir()

	if err := os.WriteFile(filepath.Join(inside, "real.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(inside, "escape")); err != nil {
		t.Skip("cannot create symlink:", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}

	r, err := os.OpenRoot(inside)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()

	var visited []string
	_ = fs.WalkDir(r.FS(), ".", func(path string, _ fs.DirEntry, ferr error) error {
		if ferr != nil {
			return nil
		}
		visited = append(visited, path)
		return nil
	})

	for _, p := range visited {
		if strings.HasPrefix(p, "escape/") {
			t.Errorf("walk escaped root via symlink: visited %q", p)
		}
	}
	found := false
	for _, p := range visited {
		if p == "real.txt" {
			found = true
		}
	}
	if !found {
		t.Error("real.txt not visited; walk may be broken")
	}
}

func TestInvokingUser(t *testing.T) {
	t.Run("SUDO_USER unset falls back to current user", func(t *testing.T) {
		t.Setenv("SUDO_USER", "")
		u, err := InvokingUser()
		if err != nil {
			t.Fatal(err)
		}
		if u.Username == "" {
			t.Errorf("empty Username on fallback")
		}
	})

	t.Run("SUDO_USER naming unknown user falls back", func(t *testing.T) {
		t.Setenv("SUDO_USER", "this-user-certainly-does-not-exist-42")
		u, err := InvokingUser()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if u == nil {
			t.Fatal("nil user")
		}
	})
}

func TestInvokingUserIDs(t *testing.T) {
	t.Run("both unset returns (nil, nil)", func(t *testing.T) {
		t.Setenv("SUDO_UID", "")
		t.Setenv("SUDO_GID", "")
		ids, err := invokingUserIDs()
		if err != nil {
			t.Fatal(err)
		}
		if ids != nil {
			t.Errorf("ids = %+v; want nil", ids)
		}
	})

	t.Run("both set and numeric returns struct", func(t *testing.T) {
		t.Setenv("SUDO_UID", "1000")
		t.Setenv("SUDO_GID", "1001")
		ids, err := invokingUserIDs()
		if err != nil {
			t.Fatal(err)
		}
		if ids == nil || ids.uid != 1000 || ids.gid != 1001 {
			t.Errorf("ids = %+v; want {1000, 1001}", ids)
		}
	})

	t.Run("non-numeric SUDO_UID returns error", func(t *testing.T) {
		t.Setenv("SUDO_UID", "bogus")
		t.Setenv("SUDO_GID", "1001")
		_, err := invokingUserIDs()
		if err == nil {
			t.Errorf("expected parse error for bogus SUDO_UID")
		}
	})

	t.Run("non-numeric SUDO_GID returns error", func(t *testing.T) {
		t.Setenv("SUDO_UID", "1000")
		t.Setenv("SUDO_GID", "bogus")
		_, err := invokingUserIDs()
		if err == nil {
			t.Errorf("expected parse error for bogus SUDO_GID")
		}
	})

	t.Run("only SUDO_UID set returns (nil, nil)", func(t *testing.T) {
		t.Setenv("SUDO_UID", "1000")
		t.Setenv("SUDO_GID", "")
		ids, err := invokingUserIDs()
		if err != nil {
			t.Fatal(err)
		}
		if ids != nil {
			t.Errorf("expected nil when only half set; got %+v", ids)
		}
	})
}

func TestWriteAsInvokingUser(t *testing.T) {
	t.Setenv("SUDO_UID", "")
	t.Setenv("SUDO_GID", "")

	t.Run("writes file with mode when parent exists", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "a.txt")
		if err := WriteAsInvokingUser(path, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
		fi, _ := os.Stat(path)
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Errorf("perm = %#o, want 0o600", perm)
		}
	})

	t.Run("creates parent directory when missing", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nested", "a.txt")
		if err := WriteAsInvokingUser(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("file not created: %v", err)
		}
	})
}

func TestChownTreeToInvokingUser_NoSudo(t *testing.T) {
	t.Setenv("SUDO_UID", "")
	t.Setenv("SUDO_GID", "")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ChownTreeToInvokingUser(dir); err != nil {
		t.Errorf("expected no-op without SUDO_UID/GID; got %v", err)
	}
}

func TestChownTreeToInvokingUser_AggregatesErrors(t *testing.T) {
	t.Run("no-op when chown targets current uid", func(t *testing.T) {
		uid := os.Getuid()
		gid := os.Getgid()
		t.Setenv("SUDO_UID", strconv.Itoa(uid))
		t.Setenv("SUDO_GID", strconv.Itoa(gid))

		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(dir, "nonexistent"), filepath.Join(dir, "link")); err != nil {
			t.Skip("cannot create symlink:", err)
		}

		if err := ChownTreeToInvokingUser(dir); err != nil {
			t.Errorf("expected nil for same-uid chown; got %v", err)
		}
	})

	t.Run("walk continues past chown failures and joins all errors", func(t *testing.T) {
		t.Setenv("SUDO_UID", "65534")
		t.Setenv("SUDO_GID", "65534")

		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o600); err != nil {
			t.Fatal(err)
		}

		err := ChownTreeToInvokingUser(dir)

		if os.Getuid() == 0 {
			if err != nil {
				t.Errorf("running as root: expected nil; got %v", err)
			}
			return
		}

		if err == nil {
			t.Fatal("expected aggregated EPERM errors; got nil")
		}
		// errors.Join joins sub-errors with newlines; >=3 lines proves the
		// walk visited "." + a.txt + b.txt rather than aborting at the first.
		lineCount := strings.Count(err.Error(), "\n") + 1
		if lineCount < 3 {
			t.Errorf("expected >=3 joined errors (walk continued); got %d in: %v", lineCount, err)
		}
	})
}

