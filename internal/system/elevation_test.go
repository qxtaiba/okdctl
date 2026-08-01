package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/testutil"
)

// TestChownTreeToInvokingUser_SymlinkEscape drives the real function against
// a symlink-escape fixture. The dangling symlink is the tripwire: Lchown on
// the link itself succeeds, while a rewrite that follows symlinks (os.Chown /
// walking outside os.Root) hits ENOENT on the dangling target and surfaces
// in the joined error.
func TestChownTreeToInvokingUser_SymlinkEscape(t *testing.T) {
	inside := t.TempDir()
	outside := t.TempDir()

	if err := os.WriteFile(filepath.Join(inside, "real.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(inside, "escape")); err != nil {
		t.Skip("cannot create symlink:", err)
	}
	if err := os.Symlink(filepath.Join(outside, "gone"), filepath.Join(inside, "dangling")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Chown-to-self is always permitted, so the real function can run
	// unprivileged with the invoking user pinned to the current user.
	t.Setenv("SUDO_UID", strconv.Itoa(os.Getuid()))
	t.Setenv("SUDO_GID", strconv.Itoa(os.Getgid()))

	if err := ChownTreeToInvokingUser(inside); err != nil {
		t.Fatalf("ChownTreeToInvokingUser: %v (a symlink-following rewrite fails here via the dangling link)", err)
	}

	if _, err := os.Lstat(filepath.Join(outside, "secret.txt")); err != nil {
		t.Fatalf("escape target disturbed: %v", err)
	}
}

// TestChownToInvokingUser_DoesNotFollowSymlink pins Lchown semantics via a
// dangling symlink: Lchown on the link itself succeeds, while a rewrite back
// to symlink-following os.Chown resolves the missing target and hits ENOENT.
func TestChownToInvokingUser_DoesNotFollowSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "dangling")
	if err := os.Symlink(filepath.Join(dir, "gone"), link); err != nil {
		t.Skip("cannot create symlink:", err)
	}

	// Chown-to-self is always permitted, so the real function can run
	// unprivileged with the invoking user pinned to the current user.
	t.Setenv("SUDO_UID", strconv.Itoa(os.Getuid()))
	t.Setenv("SUDO_GID", strconv.Itoa(os.Getgid()))

	if err := ChownToInvokingUser(link); err != nil {
		t.Fatalf("ChownToInvokingUser on dangling symlink: %v (a symlink-following rewrite fails here)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "gone")); !os.IsNotExist(err) {
		t.Fatalf("dangling target materialized: %v", err)
	}
}

func TestChownFileToInvokingUser(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "sink.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	t.Run("no-op without sudo env", func(t *testing.T) {
		t.Setenv("SUDO_UID", "")
		t.Setenv("SUDO_GID", "")
		if err := ChownFileToInvokingUser(f); err != nil {
			t.Errorf("expected no-op without SUDO_UID/GID; got %v", err)
		}
	})

	t.Run("chowns through the descriptor", func(t *testing.T) {
		t.Setenv("SUDO_UID", strconv.Itoa(os.Getuid()))
		t.Setenv("SUDO_GID", strconv.Itoa(os.Getgid()))
		if err := ChownFileToInvokingUser(f); err != nil {
			t.Errorf("fd chown to self: %v", err)
		}
	})
}

func TestInvokingUser(t *testing.T) {
	t.Run("SUDO_USER unset falls back to current user", func(t *testing.T) {
		t.Setenv("SUDO_USER", "")
		u, err := invokingUser()
		if err != nil {
			t.Fatal(err)
		}
		if u.Username == "" {
			t.Errorf("empty Username on fallback")
		}
	})

	t.Run("SUDO_USER naming unknown user falls back", func(t *testing.T) {
		t.Setenv("SUDO_USER", "this-user-certainly-does-not-exist-42")
		u, err := invokingUser()
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

	t.Run("chown-fails-after-write returns the chown error and file persists", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("running as root: chown to 65534 succeeds, cannot exercise failure path")
		}
		t.Setenv("SUDO_UID", strconv.Itoa(65534))
		t.Setenv("SUDO_GID", strconv.Itoa(65534))

		dir := t.TempDir()
		path := filepath.Join(dir, "x")
		err := WriteAsInvokingUser(path, []byte("content"), 0o600)
		if err == nil {
			t.Fatal("expected chown error; got nil")
		}
		if _, statErr := os.Stat(path); statErr != nil {
			t.Errorf("file must persist after failed chown; Stat: %v", statErr)
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

// TestHasPasswordlessSudo_CtxCancelSurfacesCtxErr locks the executor.run
// convention: a ctx-killed sudo probe reports the cancellation, not the
// opaque *exec.ExitError from the delivered signal.
func TestHasPasswordlessSudo_CtxCancelSurfacesCtxErr(t *testing.T) {
	testutil.InstallFakeBin(t, "sudo", "#!/bin/sh\nexec /bin/sleep 5\n")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := HasPasswordlessSudo(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v; want context.DeadlineExceeded", err)
	}
}

// TestHasPasswordlessSudo_PassAndFail covers the two uncancelled outcomes:
// exit 0 maps to nil, non-zero exit propagates as a non-ctx error.
func TestHasPasswordlessSudo_PassAndFail(t *testing.T) {
	testutil.InstallFakeBin(t, "sudo", "#!/bin/sh\nexit 0\n")
	if err := HasPasswordlessSudo(context.Background()); err != nil {
		t.Fatalf("passwordless probe: err = %v; want nil", err)
	}

	testutil.InstallFakeBin(t, "sudo", "#!/bin/sh\nexit 1\n")
	err := HasPasswordlessSudo(context.Background())
	if err == nil {
		t.Fatal("expected error for non-zero sudo exit")
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v; must not be a ctx error", err)
	}
}
