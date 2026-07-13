package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func TestValidateResizeFlags(t *testing.T) {
	cases := []struct {
		name          string
		memoryMB, cpu int
		wantErr       bool
	}{
		{"memory only", 16384, 0, false},
		{"cpu only", 0, 8, false},
		{"both set", 16384, 8, false},
		{"neither set", 0, 0, true},
		{"negative values treated as unset", -1, -1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateResizeFlags(tc.memoryMB, tc.cpu)
			if tc.wantErr {
				var usageErr *errtypes.UsageError
				if !errors.As(err, &usageErr) {
					t.Fatalf("want *errtypes.UsageError, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("want nil error, got %v", err)
			}
		})
	}
}

// hashTree returns a stable digest of every regular file's relative path and
// content under root, so a caller can assert a directory tree is
// byte-identical before and after an operation without diffing file-by-file.
// Walks via os.Root (mirrors infrastructure/embed_test.go) rather than
// filepath.WalkDir + os.ReadFile so gosec's TOCTOU check on a raw path stays
// clean.
func hashTree(t *testing.T, root string) string {
	t.Helper()
	r, err := os.OpenRoot(root)
	if err != nil {
		t.Fatalf("open root %s: %v", root, err)
	}
	defer func() { _ = r.Close() }()
	rootFS := r.FS()

	var paths []string
	files := map[string][]byte{}
	err = fs.WalkDir(rootFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(rootFS, path)
		if err != nil {
			return err
		}
		paths = append(paths, path)
		files[path] = data
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, rel := range paths {
		h.Write([]byte(rel))
		h.Write(files[rel])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// writePreNodeOpsRoot seeds a production terraform root that predates the
// node-lifecycle widening (no worker_count marker), mirroring the on-disk
// shape MaterializeTerraform leaves behind for an old workspace.
func writePreNodeOpsRoot(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, "infrastructure", "terraform", "environments", "production")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"variables.tf": "variable \"cluster_name\" {}\n",
		"main.tf":      "module \"okd_cluster\" {}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestDryRunDoesNotMigrateTerraformRoot is the CLI-layer regression for
// keeping --dry-run --yes from rewriting operator HCL: against a pre-nodeops
// root it must refuse via a typed error and leave infrastructure/terraform/
// byte-identical, the CLI-layer analog of
// TestRemoveDryRunPreviewIsTruthfulAndInert in internal/node.
func TestDryRunDoesNotMigrateTerraformRoot(t *testing.T) {
	root := t.TempDir()
	writePreNodeOpsRoot(t, root)

	treeDir := filepath.Join(root, "infrastructure", "terraform")
	before := hashTree(t, treeDir)

	err := ensureNodeOpsWorkspace(context.Background(), root, true, true)

	var usageErr *errtypes.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("want *errtypes.UsageError, got %v (%T)", err, err)
	}
	const wantMsg = "terraform root needs node-ops migration; re-run without --dry-run to migrate (no changes made)"
	if usageErr.Msg != wantMsg {
		t.Errorf("error message = %q, want %q", usageErr.Msg, wantMsg)
	}

	after := hashTree(t, treeDir)
	if before != after {
		t.Fatalf("--dry-run --yes mutated the terraform root: before=%s after=%s", before, after)
	}
}

// TestDryRunDoesNotPromptForMigration guards the interactive path: with
// --dry-run and no --yes, the migration consent prompt must never fire —
// promptForConfirmation is unreachable from a test (it reads stdin), so its
// absence is asserted indirectly via the same typed-refusal contract.
func TestDryRunDoesNotPromptForMigration(t *testing.T) {
	root := t.TempDir()
	writePreNodeOpsRoot(t, root)

	err := ensureNodeOpsWorkspace(context.Background(), root, false, true)

	var usageErr *errtypes.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("want *errtypes.UsageError, got %v (%T)", err, err)
	}
}

func TestDescribeResizeChange(t *testing.T) {
	cases := []struct {
		name          string
		memoryMB, cpu int
		want          string
	}{
		{"memory only", 16384, 0, "16384 MiB"},
		{"cpu only", 0, 8, "8 vCPU"},
		{"both", 16384, 8, "16384 MiB, 8 vCPU"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := describeResizeChange(tc.memoryMB, tc.cpu); got != tc.want {
				t.Errorf("describeResizeChange(%d, %d) = %q, want %q", tc.memoryMB, tc.cpu, got, tc.want)
			}
		})
	}
}
