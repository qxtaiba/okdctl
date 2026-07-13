package deploy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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

// hashInfraTree digests every regular file's relative path and content under
// <root>/infrastructure so a caller can assert the tree — the stamped manifest
// included — is byte-identical before and after an operation. The walk only
// collects relative paths; files are read after WalkDir returns so no
// filesystem operation runs inside the callback (gosec G122).
func hashInfraTree(t *testing.T, root string) string {
	t.Helper()
	base := filepath.Join(root, "infrastructure")
	var paths []string
	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", base, err)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, rel := range paths {
		data, readErr := os.ReadFile(filepath.Join(base, rel))
		if readErr != nil {
			t.Fatalf("read %s: %v", rel, readErr)
		}
		h.Write([]byte(rel))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// TestMaterializeSettledRootWritesNothing pins the deploy --dry-run contract at
// the materialize layer: a root that was already materialized and stamped is
// left byte-identical on a repeat call — no file created, no manifest re-stamped.
func TestMaterializeSettledRootWritesNothing(t *testing.T) {
	root := t.TempDir()
	if _, err := MaterializeTerraform(root); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if _, err := os.Stat(rootManifestPath(root)); err != nil {
		t.Fatalf("first run must stamp the manifest for a freshly created capable root: %v", err)
	}

	before := hashInfraTree(t, root)
	created, err := MaterializeTerraform(root)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(created) != 0 {
		t.Fatalf("second run created %v, want nothing", created)
	}
	if after := hashInfraTree(t, root); before != after {
		t.Fatalf("re-materialize mutated the settled root: before=%s after=%s", before, after)
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

// TestMaterializeTerraformLegacyCapableRootWithoutManifest composes
// materialization with manifest detection: a pre-existing checkout that
// already carries the real embedded content (so it content-sniffs as
// node-ops capable) never gets stamped by a no-op MaterializeTerraform run,
// and TerraformRootSupportsNodeOps still resolves via content-sniff.
func TestMaterializeTerraformLegacyCapableRootWithoutManifest(t *testing.T) {
	root := t.TempDir()
	for _, path := range embeddedTerraformPaths(t) {
		target := filepath.Join(root, "infrastructure", filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		data, err := infrastructure.TerraformFS.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	created, err := MaterializeTerraform(root)
	if err != nil {
		t.Fatalf("MaterializeTerraform: %v", err)
	}
	if len(created) != 0 {
		t.Fatalf("created = %v, want none for a fully pre-existing checkout", created)
	}
	if _, statErr := os.Stat(rootManifestPath(root)); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("no-op materialize must not stamp a manifest, stat err = %v", statErr)
	}

	ok, err := TerraformRootSupportsNodeOps(root)
	if err != nil || !ok {
		t.Fatalf("legacy capable root must be detected via content-sniff, got (%v,%v)", ok, err)
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
