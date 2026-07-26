package deploy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/infrastructure"
	"github.com/qxtaiba/okdctl/internal/tui"
)

func prodDir(root string) string {
	return filepath.Join(root, "infrastructure", "terraform", "environments", "production")
}

// writeProdRoot lays down both production env files so a stamped manifest has
// managed files to record.
func writeProdRoot(t *testing.T, root string) {
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

func TestManifestRoundTrip(t *testing.T) {
	root := t.TempDir()
	writeProdRoot(t, root)
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
	for _, rel := range []string{
		"terraform/environments/production/variables.tf",
		"terraform/environments/production/main.tf",
	} {
		embedded, err := infrastructure.TerraformFS.ReadFile(rel)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(embedded)
		if m.Files[rel] != hex.EncodeToString(sum[:]) {
			t.Errorf("%s: recorded hash does not match embedded content", rel)
		}
	}
}

// TestReadRootManifestUnknownSchemaIgnored pins that a manifest written by a
// newer binary (unrecognised SchemaVersion) is never trusted: readRootManifest
// returns nil, and the mismatch is logged rather than silently ignored.
func TestReadRootManifestUnknownSchemaIgnored(t *testing.T) {
	root := t.TempDir()
	writeProdRoot(t, root)

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
	if err := tui.ConfigureLoggers("warn", "text", io.Discard, &buf, false); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tui.ConfigureLoggers("info", "text", os.Stdout, os.Stderr, false) }()

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
}

// writeTestManifest writes a manifest recording the sha256 of each supplied
// byte slice, decoupled from stampRootManifest (which records embedded bytes)
// so a test can drive the Stale vs Unverified split directly.
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

func TestStampRootManifestCoversModuleFiles(t *testing.T) {
	root := t.TempDir()
	writeProdRoot(t, root)
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
