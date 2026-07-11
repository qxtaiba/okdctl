package deploy

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/qxtaiba/okdctl/infrastructure"
)

func embeddedTerraformPaths(t *testing.T) []string {
	t.Helper()
	var paths []string
	err := fs.WalkDir(infrastructure.TerraformFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk embedded FS: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("embedded terraform FS is empty")
	}
	return paths
}

func TestMaterializeTerraformEmptyDir(t *testing.T) {
	oldMask := syscall.Umask(0o022)
	defer syscall.Umask(oldMask)

	root := t.TempDir()
	created, err := MaterializeTerraform(root)
	if err != nil {
		t.Fatalf("MaterializeTerraform: %v", err)
	}

	want := embeddedTerraformPaths(t)
	if len(created) != len(want) {
		t.Fatalf("created %d files, want %d: %v", len(created), len(want), created)
	}
	for _, path := range want {
		target := filepath.Join(root, "infrastructure", filepath.FromSlash(path))
		info, statErr := os.Stat(target)
		if statErr != nil {
			t.Fatalf("expected %s to exist: %v", target, statErr)
		}
		if info.Mode().Perm() != 0o644 {
			t.Errorf("%s: perm = %v, want 0644", target, info.Mode().Perm())
		}
		got, readErr := os.ReadFile(target)
		if readErr != nil {
			t.Fatalf("read %s: %v", target, readErr)
		}
		embedded, _ := infrastructure.TerraformFS.ReadFile(path)
		if !bytes.Equal(got, embedded) {
			t.Errorf("%s: content differs from embedded source", target)
		}
	}

	lock := filepath.Join(root, "infrastructure", "terraform", "environments", "production", ".terraform.lock.hcl")
	if _, err := os.Stat(lock); err != nil {
		t.Errorf("provider lock file not materialized: %v", err)
	}
}

func TestMaterializeTerraformIdempotent(t *testing.T) {
	root := t.TempDir()
	if _, err := MaterializeTerraform(root); err != nil {
		t.Fatalf("first run: %v", err)
	}
	created, err := MaterializeTerraform(root)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(created) != 0 {
		t.Fatalf("second run created %d files, want 0: %v", len(created), created)
	}
}

func TestMaterializeTerraformPreservesModifiedFiles(t *testing.T) {
	root := t.TempDir()
	if _, err := MaterializeTerraform(root); err != nil {
		t.Fatalf("first run: %v", err)
	}

	modified := filepath.Join(root, "infrastructure", "terraform", "modules", "proxmox-okd", "main.tf")
	const sentinel = "# operator-modified\n"
	if err := os.WriteFile(modified, []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}
	removed := filepath.Join(root, "infrastructure", "terraform", "modules", "proxmox-okd", "output.tf")
	if err := os.Remove(removed); err != nil {
		t.Fatal(err)
	}

	created, err := MaterializeTerraform(root)
	if err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if len(created) != 1 || created[0] != removed {
		t.Fatalf("created = %v, want only %s", created, removed)
	}
	got, err := os.ReadFile(modified)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != sentinel {
		t.Errorf("modified file was overwritten: %q", got)
	}
}

// TestMaterializeTerraformSourceCheckoutPassthrough pins the dev-checkout
// contract: a pre-existing infrastructure/terraform tree is used as-is and
// never rewritten.
func TestMaterializeTerraformSourceCheckoutPassthrough(t *testing.T) {
	root := t.TempDir()
	const sentinel = "# checkout content\n"
	for _, path := range embeddedTerraformPaths(t) {
		target := filepath.Join(root, "infrastructure", filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(sentinel), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	created, err := MaterializeTerraform(root)
	if err != nil {
		t.Fatalf("MaterializeTerraform: %v", err)
	}
	if len(created) != 0 {
		t.Fatalf("created = %v, want none in a source checkout", created)
	}
	probe := filepath.Join(root, "infrastructure", "terraform", "environments", "production", "main.tf")
	got, err := os.ReadFile(probe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != sentinel {
		t.Errorf("checkout file was overwritten: %q", got)
	}
}

func TestMaterializeTerraformSkipsDanglingSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "infrastructure", "terraform", "environments", "production", "main.tf")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "does-not-exist"), target); err != nil {
		t.Fatal(err)
	}

	created, err := MaterializeTerraform(root)
	if err != nil {
		t.Fatalf("MaterializeTerraform: %v", err)
	}
	for _, c := range created {
		if c == target {
			t.Fatalf("symlink target %s was rewritten", target)
		}
	}
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("symlink was replaced by a regular file")
	}
}
