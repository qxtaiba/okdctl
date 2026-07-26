package deploy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/infrastructure"
)

func prodDir(root string) string {
	return filepath.Join(root, "infrastructure", "terraform", "environments", "production")
}

// writeNodeOpsRoot lays down both managed files carrying their markers so the
// root content-sniffs as node-ops capable (no manifest).
func writeNodeOpsRoot(t *testing.T, root string) {
	t.Helper()
	dir := prodDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(`variable "worker_count" { default = 3 }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("module \"okd\" {\n  worker_count = var.worker_count\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTerraformRootSupportsNodeOps(t *testing.T) {
	t.Run("missing workspace is not an error", func(t *testing.T) {
		ok, err := TerraformRootSupportsNodeOps(t.TempDir())
		if err != nil || ok {
			t.Fatalf("want (false,nil) for empty workspace, got (%v,%v)", ok, err)
		}
	})

	t.Run("old root lacks marker", func(t *testing.T) {
		root := t.TempDir()
		dir := prodDir(root)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(`variable "cluster_name" {}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`module "okd" {}`), 0o644); err != nil {
			t.Fatal(err)
		}
		ok, err := TerraformRootSupportsNodeOps(root)
		if err != nil || ok {
			t.Fatalf("old root must report unsupported, got (%v,%v)", ok, err)
		}
	})

	// A crash between the sequential migration writes leaves variables.tf
	// carrying the marker while main.tf never threads it through. The old
	// single-file check passed such a root forever; requiring both markers
	// re-offers the idempotent migration instead.
	t.Run("half-migrated root is unsupported", func(t *testing.T) {
		root := t.TempDir()
		dir := prodDir(root)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(`variable "worker_count" { default = 3 }`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`module "okd" {}`), 0o644); err != nil {
			t.Fatal(err)
		}
		ok, err := TerraformRootSupportsNodeOps(root)
		if err != nil || ok {
			t.Fatalf("half-migrated root must report unsupported, got (%v,%v)", ok, err)
		}
	})

	t.Run("widened root has markers in both files", func(t *testing.T) {
		root := t.TempDir()
		writeNodeOpsRoot(t, root)
		ok, err := TerraformRootSupportsNodeOps(root)
		if err != nil || !ok {
			t.Fatalf("widened root must report supported, got (%v,%v)", ok, err)
		}
	})
}

func TestManifestRoundTrip(t *testing.T) {
	root := t.TempDir()
	writeNodeOpsRoot(t, root)
	if err := stampRootManifest(root, nodeOpsRootFormat); err != nil {
		t.Fatalf("stampRootManifest: %v", err)
	}

	m, err := readRootManifest(root)
	if err != nil {
		t.Fatalf("readRootManifest: %v", err)
	}
	if m == nil {
		t.Fatal("expected a manifest")
	}
	if m.SchemaVersion != rootManifestSchema || m.Format != nodeOpsRootFormat {
		t.Fatalf("manifest schema/format = (%d,%d), want (%d,%d)", m.SchemaVersion, m.Format, rootManifestSchema, nodeOpsRootFormat)
	}
	for _, rel := range nodeOpsRootFiles {
		embedded, err := infrastructure.TerraformFS.ReadFile(rel)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(embedded)
		if m.Files[rel] != hex.EncodeToString(sum[:]) {
			t.Errorf("%s: recorded hash does not match embedded content", rel)
		}
	}

	ok, err := TerraformRootSupportsNodeOps(root)
	if err != nil || !ok {
		t.Fatalf("stamped root must report supported, got (%v,%v)", ok, err)
	}
}

// TestReadRootManifestUnknownSchemaFallsBackToContentSniff pins that a
// manifest written by a newer binary (unrecognised SchemaVersion) is never
// trusted: readRootManifest returns nil so detection falls back to
// content-sniffing, and the mismatch is logged rather than silently ignored.
func TestReadRootManifestUnknownSchemaFallsBackToContentSniff(t *testing.T) {
	root := t.TempDir()
	writeNodeOpsRoot(t, root)

	m := terraformRootManifest{SchemaVersion: rootManifestSchema + 1, Format: nodeOpsRootFormat, Files: map[string]string{}}
	data, err := json.MarshalIndent(&m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(rootManifestPath(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rootManifestPath(root), data, 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	oldDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(oldDefault)

	got, err := readRootManifest(root)
	if err != nil {
		t.Fatalf("readRootManifest: %v", err)
	}
	if got != nil {
		t.Fatalf("readRootManifest = %+v, want nil for an unrecognised schema version", got)
	}
	if !strings.Contains(buf.String(), "unknown schema") {
		t.Errorf("expected a warn log for the unrecognised schema version, got: %s", buf.String())
	}

	ok, err := TerraformRootSupportsNodeOps(root)
	if err != nil || !ok {
		t.Fatalf("detection must still succeed via content-sniff fallback, got (%v,%v)", ok, err)
	}
}

func TestLegacyRootWithoutManifestDetected(t *testing.T) {
	root := t.TempDir()
	writeNodeOpsRoot(t, root)
	if _, err := os.Stat(rootManifestPath(root)); !os.IsNotExist(err) {
		t.Fatalf("expected no manifest, stat err = %v", err)
	}
	ok, err := TerraformRootSupportsNodeOps(root)
	if err != nil || !ok {
		t.Fatalf("legacy manifest-less root must be detected via content sniff, got (%v,%v)", ok, err)
	}
}

func TestTerraformRootFormatReporting(t *testing.T) {
	t.Run("legacy root reports unstamped", func(t *testing.T) {
		root := t.TempDir()
		writeNodeOpsRoot(t, root)
		format, stamped, err := TerraformRootFormat(root)
		if err != nil {
			t.Fatal(err)
		}
		if stamped || format != 0 {
			t.Fatalf("legacy root: got (format=%d, stamped=%v), want (0,false)", format, stamped)
		}
	})

	t.Run("older stamped format is reported for the mismatch message", func(t *testing.T) {
		root := t.TempDir()
		writeNodeOpsRoot(t, root)
		// Stamp an older format than this binary expects; detection must treat
		// it as unsupported and the caller can report "format N, expects M".
		if err := stampRootManifest(root, nodeOpsRootFormat-1); err != nil {
			t.Fatal(err)
		}
		ok, err := TerraformRootSupportsNodeOps(root)
		if err != nil || ok {
			t.Fatalf("older stamped format must be unsupported, got (%v,%v)", ok, err)
		}
		format, stamped, err := TerraformRootFormat(root)
		if err != nil {
			t.Fatal(err)
		}
		if !stamped || format != nodeOpsRootFormat-1 {
			t.Fatalf("got (format=%d, stamped=%v), want (%d,true)", format, stamped, nodeOpsRootFormat-1)
		}
		if ExpectedTerraformRootFormat() != nodeOpsRootFormat {
			t.Fatalf("ExpectedTerraformRootFormat = %d, want %d", ExpectedTerraformRootFormat(), nodeOpsRootFormat)
		}
	})
}

// TestCrashWindowSelfHeal simulates a crash mid-migration: variables.tf already
// carries its real embedded content, main.tf is absent, no manifest. Detection
// must re-offer migration; the migration completes, stamps the root, and — on a
// second migrate after the stamp is lost ("crash") — produces no duplicate
// backup because both files already equal the embedded copy (bytes.Equal skip).
func TestCrashWindowSelfHeal(t *testing.T) {
	root := t.TempDir()
	dir := prodDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	embeddedVars, err := infrastructure.TerraformFS.ReadFile("terraform/environments/production/variables.tf")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "variables.tf"), embeddedVars, 0o644); err != nil {
		t.Fatal(err)
	}

	ok, err := TerraformRootSupportsNodeOps(root)
	if err != nil || ok {
		t.Fatalf("half-written root must be unsupported, got (%v,%v)", ok, err)
	}

	if _, err := MigrateTerraformRoot(root); err != nil {
		t.Fatalf("MigrateTerraformRoot: %v", err)
	}

	ok, err = TerraformRootSupportsNodeOps(root)
	if err != nil || !ok {
		t.Fatalf("healed root must be supported, got (%v,%v)", ok, err)
	}
	if _, err := os.Stat(rootManifestPath(root)); err != nil {
		t.Errorf("migration must stamp the manifest: %v", err)
	}
	for _, rel := range nodeOpsRootFiles {
		got, err := os.ReadFile(filepath.Join(root, "infrastructure", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		embedded, _ := infrastructure.TerraformFS.ReadFile(rel)
		if !bytes.Equal(got, embedded) {
			t.Errorf("%s: not rewritten to embedded content", rel)
		}
	}

	// Crash after the second-run stamp would otherwise be lost: drop the manifest
	// and migrate again. Every managed file already equals the embedded copy, so
	// the bytes.Equal skip fires and no *.pre-nodeops.bak is written.
	if err := os.Remove(rootManifestPath(root)); err != nil {
		t.Fatal(err)
	}
	migrated, err := MigrateTerraformRoot(root)
	if err != nil {
		t.Fatalf("second MigrateTerraformRoot: %v", err)
	}
	if len(migrated) != 0 {
		t.Errorf("re-migrate rewrote %v, want nothing (all files already embedded)", migrated)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "pre-nodeops.bak") {
			t.Errorf("re-migrate produced a duplicate backup: %s", e.Name())
		}
	}
}

// writeTestManifest writes a manifest recording the sha256 of each supplied
// byte slice, decoupled from stampRootManifest (which records embedded bytes) so
// a test can drive the Refresh vs OperatorModified split directly.
func writeTestManifest(t *testing.T, root string, recorded map[string][]byte) {
	t.Helper()
	files := make(map[string]string, len(recorded))
	for rel, data := range recorded {
		sum := sha256.Sum256(data)
		files[rel] = hex.EncodeToString(sum[:])
	}
	m := terraformRootManifest{SchemaVersion: rootManifestSchema, Format: nodeOpsRootFormat, Files: files}
	data, err := json.MarshalIndent(&m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(rootManifestPath(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rootManifestPath(root), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPreviewTerraformRootMigration(t *testing.T) {
	root := t.TempDir()
	dir := prodDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Both files diverge from the current embedded copy, so a migration would
	// rewrite them. main.tf still matches the hash the manifest recorded
	// (pristine since stamp → Refresh); variables.tf does not (operator edit →
	// OperatorModified).
	varsPath := filepath.Join(dir, "variables.tf")
	mainPath := filepath.Join(dir, "main.tf")
	mainContent := []byte("# pristine since stamp\n")
	if err := os.WriteFile(mainPath, mainContent, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(varsPath, []byte("# operator edit — diverged from recorded hash\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestManifest(t, root, map[string][]byte{
		"terraform/environments/production/main.tf":      mainContent,
		"terraform/environments/production/variables.tf": []byte("# what okdctl originally stamped\n"),
	})

	prev, err := PreviewTerraformRootMigration(root)
	if err != nil {
		t.Fatalf("PreviewTerraformRootMigration: %v", err)
	}
	if len(prev.OperatorModified) != 1 || prev.OperatorModified[0] != varsPath {
		t.Fatalf("OperatorModified = %v, want [%s]", prev.OperatorModified, varsPath)
	}
	if len(prev.Refresh) != 1 || prev.Refresh[0] != mainPath {
		t.Fatalf("Refresh = %v, want [%s]", prev.Refresh, mainPath)
	}
}

// TestPreviewLegacyRootNeverAssertsEdit pins that without a manifest okdctl
// classifies divergent files as Refresh, never as an operator edit it can't prove.
func TestPreviewLegacyRootNeverAssertsEdit(t *testing.T) {
	root := t.TempDir()
	dir := prodDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte("# old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("# old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev, err := PreviewTerraformRootMigration(root)
	if err != nil {
		t.Fatalf("PreviewTerraformRootMigration: %v", err)
	}
	if len(prev.OperatorModified) != 0 {
		t.Fatalf("legacy root must not assert operator edits, got %v", prev.OperatorModified)
	}
	if len(prev.Refresh) != 2 {
		t.Fatalf("Refresh = %v, want both managed files", prev.Refresh)
	}
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

	ok, err := TerraformRootSupportsNodeOps(root)
	if err != nil || !ok {
		t.Fatalf("post-migration root must support node ops, got (%v,%v)", ok, err)
	}
}

func TestStampRootManifestCoversModuleFiles(t *testing.T) {
	root := t.TempDir()
	writeNodeOpsRoot(t, root)
	if err := stampRootManifest(root, nodeOpsRootFormat); err != nil {
		t.Fatalf("stampRootManifest: %v", err)
	}
	m, err := readRootManifest(root)
	if err != nil || m == nil {
		t.Fatalf("readRootManifest: (%+v, %v)", m, err)
	}
	const moduleMain = "terraform/modules/proxmox-okd/main.tf"
	if m.Files[moduleMain] == "" {
		t.Errorf("manifest has no hash for %s; module files must be covered", moduleMain)
	}
}

func TestDetectEmbeddedDrift(t *testing.T) {
	const moduleMain = "terraform/modules/proxmox-okd/main.tf"
	embedded, err := infrastructure.TerraformFS.ReadFile(moduleMain)
	if err != nil {
		t.Fatal(err)
	}
	oldContent := []byte("# module written by an older okdctl\n")
	writeModule := func(t *testing.T, root string, content []byte) string {
		t.Helper()
		target := filepath.Join(root, "infrastructure", filepath.FromSlash(moduleMain))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			t.Fatal(err)
		}
		return target
	}

	t.Run("empty root reports nothing", func(t *testing.T) {
		drift, err := DetectEmbeddedDrift(t.TempDir())
		if err != nil {
			t.Fatalf("DetectEmbeddedDrift: %v", err)
		}
		if len(drift.Stale) != 0 || len(drift.Unverified) != 0 {
			t.Errorf("drift = %+v; want empty for a not-yet-materialized root", drift)
		}
	})

	t.Run("copy matching embedded reports nothing", func(t *testing.T) {
		root := t.TempDir()
		writeModule(t, root, embedded)
		drift, err := DetectEmbeddedDrift(root)
		if err != nil {
			t.Fatalf("DetectEmbeddedDrift: %v", err)
		}
		if len(drift.Stale) != 0 || len(drift.Unverified) != 0 {
			t.Errorf("drift = %+v; want empty for an up-to-date file", drift)
		}
	})

	t.Run("pristine stale file reported as stale", func(t *testing.T) {
		root := t.TempDir()
		target := writeModule(t, root, oldContent)
		writeTestManifest(t, root, map[string][]byte{moduleMain: oldContent})
		drift, err := DetectEmbeddedDrift(root)
		if err != nil {
			t.Fatalf("DetectEmbeddedDrift: %v", err)
		}
		if len(drift.Stale) != 1 || drift.Stale[0] != target {
			t.Errorf("Stale = %v; want [%s]", drift.Stale, target)
		}
		if len(drift.Unverified) != 0 {
			t.Errorf("Unverified = %v; want empty", drift.Unverified)
		}
	})

	t.Run("operator edit stays silent", func(t *testing.T) {
		root := t.TempDir()
		writeModule(t, root, []byte("# operator hand-edit\n"))
		writeTestManifest(t, root, map[string][]byte{moduleMain: embedded})
		drift, err := DetectEmbeddedDrift(root)
		if err != nil {
			t.Fatalf("DetectEmbeddedDrift: %v", err)
		}
		if len(drift.Stale) != 0 || len(drift.Unverified) != 0 {
			t.Errorf("drift = %+v; want empty for a proven operator edit", drift)
		}
	})

	t.Run("divergence without manifest reports unverified", func(t *testing.T) {
		root := t.TempDir()
		target := writeModule(t, root, oldContent)
		drift, err := DetectEmbeddedDrift(root)
		if err != nil {
			t.Fatalf("DetectEmbeddedDrift: %v", err)
		}
		if len(drift.Unverified) != 1 || drift.Unverified[0] != target {
			t.Errorf("Unverified = %v; want [%s]", drift.Unverified, target)
		}
		if len(drift.Stale) != 0 {
			t.Errorf("Stale = %v; want empty", drift.Stale)
		}
	})
}
