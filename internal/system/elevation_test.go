package system

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func setSudoEnv(t *testing.T, uid, gid string) {
	t.Helper()
	t.Setenv("SUDO_UID", uid)
	t.Setenv("SUDO_GID", gid)
}

// setSudoSelf pins to the current user; chown-to-self needs no privilege,
// letting these tests run unprivileged.
func setSudoSelf(t *testing.T) {
	t.Helper()
	setSudoEnv(t, strconv.Itoa(os.Getuid()), strconv.Itoa(os.Getgid()))
}

// Dangling symlink is the tripwire: Lchown succeeds without resolving it,
// while symlink-following os.Chown would hit ENOENT.
func TestChownToInvokingUser_DoesNotFollowSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "dangling")
	if err := os.Symlink(filepath.Join(dir, "gone"), link); err != nil {
		t.Skip("cannot create symlink:", err)
	}

	setSudoSelf(t)

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
		setSudoEnv(t, "", "")
		if err := ChownFileToInvokingUser(f); err != nil {
			t.Errorf("expected no-op without SUDO_UID/GID; got %v", err)
		}
	})

	t.Run("chowns through the descriptor", func(t *testing.T) {
		setSudoSelf(t)
		if err := ChownFileToInvokingUser(f); err != nil {
			t.Errorf("fd chown to self: %v", err)
		}
	})
}

func TestInvokingUserIDs(t *testing.T) {
	t.Run("both unset returns (nil, nil)", func(t *testing.T) {
		setSudoEnv(t, "", "")
		ids, err := invokingUserIDs()
		if err != nil {
			t.Fatal(err)
		}
		if ids != nil {
			t.Errorf("ids = %+v; want nil", ids)
		}
	})

	t.Run("both set and numeric returns struct", func(t *testing.T) {
		setSudoEnv(t, "1000", "1001")
		ids, err := invokingUserIDs()
		if err != nil {
			t.Fatal(err)
		}
		if ids == nil || ids.uid != 1000 || ids.gid != 1001 {
			t.Errorf("ids = %+v; want {1000, 1001}", ids)
		}
	})

	t.Run("non-numeric SUDO_UID returns error", func(t *testing.T) {
		setSudoEnv(t, "bogus", "1001")
		_, err := invokingUserIDs()
		if err == nil {
			t.Errorf("expected parse error for bogus SUDO_UID")
		}
	})

	t.Run("non-numeric SUDO_GID returns error", func(t *testing.T) {
		setSudoEnv(t, "1000", "bogus")
		_, err := invokingUserIDs()
		if err == nil {
			t.Errorf("expected parse error for bogus SUDO_GID")
		}
	})

	t.Run("only SUDO_UID set returns (nil, nil)", func(t *testing.T) {
		setSudoEnv(t, "1000", "")
		ids, err := invokingUserIDs()
		if err != nil {
			t.Fatal(err)
		}
		if ids != nil {
			t.Errorf("expected nil when only half set; got %+v", ids)
		}
	})
}
