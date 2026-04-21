package system

import (
	"os"
	"path/filepath"
	"testing"
)

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
