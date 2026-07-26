package deploy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okdctl/infrastructure"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// nodeOpsRootFiles are the production-root files whose embedded copies gained
// the node-lifecycle variables (worker_count, master_memory_mb,
// master_cpu_cores). Their paths are relative to the embedded FS root and
// mirror the on-disk workspace layout under <root>/infrastructure/.
var nodeOpsRootFiles = []string{
	"terraform/environments/production/variables.tf",
	"terraform/environments/production/main.tf",
}

// nodeOpsRootMarkers maps each node-ops root file to a string present only in
// its widened embedded copy. A root supports node ops only when EVERY file
// carries its marker: variables.tf then main.tf are written sequentially, so a
// crash between the two writes leaves a partial root that declares
// worker_count but never threads it into the module. Requiring both markers is
// what makes such a half-migration fail detection and get re-offered the
// idempotent migration.
var nodeOpsRootMarkers = map[string]string{
	"terraform/environments/production/variables.tf": `variable "worker_count"`,
	"terraform/environments/production/main.tf":      `worker_count = var.worker_count`,
}

const (
	// terraformRootManifestName is the stamp materialization/migration writes
	// under <root>/infrastructure/terraform/ recording the root's format and
	// per-file hashes. Detection prefers it over content-sniffing.
	terraformRootManifestName = ".okdctl-terraform-root.json"

	// rootManifestSchema versions the manifest JSON layout itself; bump it when
	// the struct shape changes so an older binary can reject a newer manifest.
	rootManifestSchema = 1

	// nodeOpsRootFormat is the root-format generation this binary materializes
	// and migrates toward. A stamped root at this format (or newer) supports
	// node ops without a content-sniff.
	nodeOpsRootFormat = 1
)

// terraformRootManifest records the format generation of a materialized
// terraform root plus a sha256 of every managed file, written LAST so its
// presence certifies that all managed files already landed. Detection trusts a
// present manifest; migration diffs the recorded hashes against on-disk content
// to tell operator edits from pristine files.
type terraformRootManifest struct {
	SchemaVersion int               `json:"schema_version"`
	Format        int               `json:"format"`
	Files         map[string]string `json:"files"`
}

func rootManifestPath(root string) string {
	return filepath.Join(root, "infrastructure", "terraform", terraformRootManifestName)
}

// readRootManifest returns the root's manifest, or (nil, nil) when detection
// must fall back to content-sniffing: a legacy root that predates manifest
// stamping (no file on disk), or a manifest whose SchemaVersion this build does
// not recognise (a newer binary wrote it) — the latter is logged at Warn rather
// than trusted. It is read-only: detection must never write.
func readRootManifest(root string) (*terraformRootManifest, error) {
	data, err := os.ReadFile(rootManifestPath(root))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read terraform root manifest: %w", err)
	}
	var m terraformRootManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse terraform root manifest: %w", err)
	}
	if m.SchemaVersion != rootManifestSchema {
		tui.Warn("terraform root manifest has unknown schema; using content detection",
			tui.LF("path", rootManifestPath(root)),
			tui.LF("schema_version", m.SchemaVersion),
			tui.LF("expected", rootManifestSchema))
		return nil, nil
	}
	return &m, nil
}

func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// stampRootManifest records the sha256 of the EMBEDDED bytes okdctl writes —
// the source of truth — never the on-disk content. Hashing what was written
// (not what is currently on disk) is what lets a later refresh tell a
// pristine file from an operator edit: an edit made before a re-stamp can
// never be absorbed as the recorded baseline. Every embedded file is hashed
// (module included), so drift detection covers the whole managed tree, not
// just the node-ops env files. Callers MUST invoke it only after every
// managed file has landed: the manifest is the completion marker detection
// trusts, so stamping before the last file lands would itself open a crash
// window.
func stampRootManifest(root string, format int) error {
	files := make(map[string]string)
	err := fs.WalkDir(infrastructure.TerraformFS, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		data, readErr := infrastructure.TerraformFS.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		files[path] = contentHash(data)
		return nil
	})
	if err != nil {
		return fmt.Errorf("hash embedded terraform sources: %w", err)
	}
	m := terraformRootManifest{SchemaVersion: rootManifestSchema, Format: format, Files: files}
	data, err := json.MarshalIndent(&m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode terraform root manifest: %w", err)
	}
	data = append(data, '\n')
	if err := system.AtomicWrite(rootManifestPath(root), data, 0o644); err != nil {
		return fmt.Errorf("write terraform root manifest: %w", err)
	}
	return nil
}

// rootIsNodeOpsCapable content-sniffs every managed file for its marker. It is
// the legacy fallback for roots without a manifest and the guard that keeps a
// half-migrated root (marker in one file, missing from the other) unsupported.
func rootIsNodeOpsCapable(root string) (bool, error) {
	for _, rel := range nodeOpsRootFiles {
		path := filepath.Join(root, "infrastructure", filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return false, nil
			}
			return false, fmt.Errorf("read terraform root: %w", err)
		}
		if !bytes.Contains(data, []byte(nodeOpsRootMarkers[rel])) {
			return false, nil
		}
	}
	return true, nil
}

// TerraformRootSupportsNodeOps reports whether the on-disk production root
// declares the node-lifecycle variables. It prefers the root manifest and
// falls back to content-sniffing every managed file for legacy roots. Returns
// false (not an error) when the workspace has not been materialized yet — the
// caller materializes first. This probe is read-only; it never writes the
// manifest as a side effect.
func TerraformRootSupportsNodeOps(root string) (bool, error) {
	m, err := readRootManifest(root)
	if err != nil {
		return false, err
	}
	if m != nil {
		return m.Format >= nodeOpsRootFormat, nil
	}
	return rootIsNodeOpsCapable(root)
}

// ExpectedTerraformRootFormat is the root-format generation this binary
// materializes and migrates toward.
func ExpectedTerraformRootFormat() int { return nodeOpsRootFormat }

// TerraformRootFormat reports the stamped format generation of a materialized
// root and whether a manifest was present. A legacy root without a manifest
// reports (0, false, nil); its capability is inferred by TerraformRootSupportsNodeOps.
func TerraformRootFormat(root string) (format int, stamped bool, err error) {
	m, err := readRootManifest(root)
	if err != nil {
		return 0, false, err
	}
	if m == nil {
		return 0, false, nil
	}
	return m.Format, true, nil
}

// RootMigrationPreview classifies the files MigrateTerraformRoot will rewrite,
// so the caller can shape an honest consent prompt.
type RootMigrationPreview struct {
	// OperatorModified lists files whose on-disk content diverges from the hash
	// the manifest recorded — a genuine operator edit the migration backs up.
	OperatorModified []string
	// Refresh lists files the migration will rewrite that are pristine (match
	// the recorded hash) or unverifiable (legacy root without a manifest);
	// okdctl cannot assert the operator touched these.
	Refresh []string
}

// PreviewTerraformRootMigration reports, without mutating anything, which
// managed files a migration would rewrite and whether each is an operator edit.
// Only files whose on-disk content differs from the embedded copy are included,
// mirroring MigrateTerraformRoot's skip-if-equal behavior.
func PreviewTerraformRootMigration(root string) (RootMigrationPreview, error) {
	m, err := readRootManifest(root)
	if err != nil {
		return RootMigrationPreview{}, err
	}
	var prev RootMigrationPreview
	for _, rel := range nodeOpsRootFiles {
		target := filepath.Join(root, "infrastructure", filepath.FromSlash(rel))
		embedded, err := infrastructure.TerraformFS.ReadFile(rel)
		if err != nil {
			return RootMigrationPreview{}, fmt.Errorf("read embedded %s: %w", rel, err)
		}
		existing, err := os.ReadFile(target)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				prev.Refresh = append(prev.Refresh, target)
				continue
			}
			return RootMigrationPreview{}, fmt.Errorf("read %s: %w", target, err)
		}
		if bytes.Equal(existing, embedded) {
			continue
		}
		if manifestFlagsEdit(m, rel, existing) {
			prev.OperatorModified = append(prev.OperatorModified, target)
		} else {
			prev.Refresh = append(prev.Refresh, target)
		}
	}
	return prev, nil
}

// manifestFlagsEdit reports whether the manifest recorded a hash for rel that
// disagrees with existing. Absent a manifest (or an untracked file) okdctl
// cannot prove an operator edit, so it returns false rather than guess.
func manifestFlagsEdit(m *terraformRootManifest, rel string, existing []byte) bool {
	if m == nil {
		return false
	}
	recorded, ok := m.Files[rel]
	if !ok {
		return false
	}
	return recorded != contentHash(existing)
}

// EmbeddedDrift classifies managed *.tf files whose on-disk content differs
// from this binary's embedded copy. MaterializeTerraform is write-once, so
// after an okdctl upgrade an existing workspace keeps deploying the old HCL;
// drift detection is what keeps that divergence from staying silent.
type EmbeddedDrift struct {
	// Stale files match the hash the manifest recorded when okdctl wrote
	// them — the operator never touched them; only the embedded copy moved on.
	Stale []string
	// Unverified files differ from the embedded copy but have no recorded
	// hash (legacy root or untracked file), so okdctl cannot tell an operator
	// edit from staleness.
	Unverified []string
}

// DetectEmbeddedDrift compares every embedded *.tf source against its on-disk
// copy under root. Files matching the embedded copy, files missing on disk
// (materialize will create them), and proven operator edits (recorded hash
// disagrees with disk) are all excluded — write-once means an operator's
// divergence is deliberate and must not be nagged about. Read-only.
func DetectEmbeddedDrift(root string) (EmbeddedDrift, error) {
	m, err := readRootManifest(root)
	if err != nil {
		return EmbeddedDrift{}, err
	}
	var drift EmbeddedDrift
	err = fs.WalkDir(infrastructure.TerraformFS, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || filepath.Ext(path) != ".tf" {
			return nil
		}
		target := filepath.Join(root, "infrastructure", filepath.FromSlash(path))
		existing, readErr := os.ReadFile(target)
		if readErr != nil {
			if errors.Is(readErr, fs.ErrNotExist) {
				return nil
			}
			return readErr
		}
		embedded, readErr := infrastructure.TerraformFS.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if bytes.Equal(existing, embedded) {
			return nil
		}
		var recorded string
		if m != nil {
			recorded = m.Files[path]
		}
		switch {
		case recorded == "":
			drift.Unverified = append(drift.Unverified, target)
		case recorded == contentHash(existing):
			drift.Stale = append(drift.Stale, target)
		}
		return nil
	})
	if err != nil {
		return EmbeddedDrift{}, fmt.Errorf("detect terraform drift: %w", err)
	}
	return drift, nil
}

// MigrateTerraformRoot overwrites the production-root files that
// MaterializeTerraform wrote write-once, replacing them with the current
// embedded copies so node ops can drive worker_count / master sizing. Each
// replaced file is backed up to <file>.<timestamp>.pre-nodeops.bak first, and
// the root manifest is stamped LAST so a crash mid-migration leaves an unstamped
// root that re-offers the idempotent migration. Returns the files it rewrote.
// The caller is responsible for obtaining operator consent before calling —
// this overwrites operator-editable HCL.
func MigrateTerraformRoot(root string) ([]string, error) {
	ts := time.Now().UTC().Format("20060102T150405Z")
	var migrated []string
	for _, rel := range nodeOpsRootFiles {
		target := filepath.Join(root, "infrastructure", filepath.FromSlash(rel))
		embedded, err := infrastructure.TerraformFS.ReadFile(rel)
		if err != nil {
			return migrated, fmt.Errorf("read embedded %s: %w", rel, err)
		}
		existing, err := os.ReadFile(target)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return migrated, fmt.Errorf("read %s: %w", target, err)
			}
			existing = nil
		}
		if existing != nil && bytes.Equal(existing, embedded) {
			continue
		}
		if len(existing) > 0 {
			backup := fmt.Sprintf("%s.%s.pre-nodeops.bak", target, ts)
			if err := system.AtomicWrite(backup, existing, 0o644); err != nil {
				return migrated, fmt.Errorf("back up %s: %w", target, err)
			}
		}
		if err := system.AtomicWrite(target, embedded, 0o644); err != nil {
			return migrated, fmt.Errorf("write %s: %w", target, err)
		}
		migrated = append(migrated, target)
	}
	if err := stampRootManifest(root, nodeOpsRootFormat); err != nil {
		return migrated, fmt.Errorf("stamp terraform root: %w", err)
	}
	if err := system.ChownTreeToInvokingUser(filepath.Join(root, "infrastructure")); err != nil {
		return migrated, fmt.Errorf("chown terraform root: %w", err)
	}
	return migrated, nil
}
