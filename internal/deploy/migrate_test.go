package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func prodDir(root string) string {
	return filepath.Join(root, "infrastructure", "terraform", "environments", "production")
}

func TestTerraformRootSupportsNodeOps(t *testing.T) {
	root := t.TempDir()
	dir := prodDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("missing workspace is not an error", func(t *testing.T) {
		ok, err := TerraformRootSupportsNodeOps(t.TempDir())
		if err != nil || ok {
			t.Fatalf("want (false,nil) for empty workspace, got (%v,%v)", ok, err)
		}
	})

	t.Run("old root lacks marker", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(`variable "cluster_name" {}`), 0o644); err != nil {
			t.Fatal(err)
		}
		ok, err := TerraformRootSupportsNodeOps(root)
		if err != nil || ok {
			t.Fatalf("old root must report unsupported, got (%v,%v)", ok, err)
		}
	})

	t.Run("widened root has marker", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(`variable "worker_count" { default = 3 }`), 0o644); err != nil {
			t.Fatal(err)
		}
		ok, err := TerraformRootSupportsNodeOps(root)
		if err != nil || !ok {
			t.Fatalf("widened root must report supported, got (%v,%v)", ok, err)
		}
	})
}

func TestMigrateTerraformRootBacksUpAndWidens(t *testing.T) {
	root := t.TempDir()
	dir := prodDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := []byte("# pre-nodeops root\nvariable \"cluster_name\" {}\n")
	if err := os.WriteFile(filepath.Join(dir, "variables.tf"), old, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("module \"okd_cluster\" {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	migrated, err := MigrateTerraformRoot(root)
	if err != nil {
		t.Fatalf("MigrateTerraformRoot: %v", err)
	}
	if len(migrated) == 0 {
		t.Fatal("expected files to be migrated")
	}

	// backup of the original content exists
	entries, _ := os.ReadDir(dir)
	foundBackup := false
	for _, e := range entries {
		if strings.Contains(e.Name(), "pre-nodeops.bak") {
			foundBackup = true
		}
	}
	if !foundBackup {
		t.Fatal("expected a *.pre-nodeops.bak backup")
	}

	// widened variables.tf now carries the node-ops marker
	ok, err := TerraformRootSupportsNodeOps(root)
	if err != nil || !ok {
		t.Fatalf("post-migration root must support node ops, got (%v,%v)", ok, err)
	}
}
