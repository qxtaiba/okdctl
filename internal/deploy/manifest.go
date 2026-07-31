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

	"github.com/qxtaiba/okdctl/infrastructure"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

const (
	// terraformRootManifestName is the stamp materialization writes under
	// <root>/infrastructure/terraform/ recording the root's format and
	// per-file hashes.
	terraformRootManifestName = ".okdctl-terraform-root.json"

	// rootManifestSchema versions the manifest JSON layout itself; bump it when
	// the struct shape changes so an older binary can reject a newer manifest.
	rootManifestSchema = 1

	// nodeOpsRootFormat is the root-format generation this binary
	// materializes. Format is written for forward versioning only; nothing
	// reads it back today.
	nodeOpsRootFormat = 1
)

// terraformRootManifest records the format generation of a materialized
// terraform root plus a sha256 of every managed file, written LAST so its
// presence certifies that all managed files already landed. Drift detection
// diffs the recorded hashes against on-disk content to tell operator edits
// from pristine files.
type terraformRootManifest struct {
	SchemaVersion int               `json:"schema_version"`
	Format        int               `json:"format"`
	Files         map[string]string `json:"files"`
}

func rootManifestPath(root string) string {
	return filepath.Join(root, "infrastructure", "terraform", terraformRootManifestName)
}

// readRootManifest returns the root's manifest, or (nil, nil) when none is
// usable: no file on disk (a crash between materializing files and stamping
// leaves an unstamped root), or a manifest whose SchemaVersion this build does
// not recognise (a newer binary wrote it) — the latter is logged at Warn
// rather than trusted. It is read-only: detection must never write.
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
		logutil.Warn("terraform root manifest has unknown schema; ignoring it",
			logutil.LF("path", rootManifestPath(root)),
			logutil.LF("schema_version", m.SchemaVersion),
			logutil.LF("expected", rootManifestSchema))
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
// just the env files. Callers MUST invoke it only after every managed file
// has landed: the manifest is the completion marker detection trusts, so
// stamping before the last file lands would itself open a crash window.
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

// EmbeddedDrift classifies managed *.tf files whose on-disk content differs
// from this binary's embedded copy. MaterializeTerraform is write-once, so
// after an okdctl upgrade an existing workspace keeps deploying the old HCL;
// drift detection is what keeps that divergence from staying silent.
type EmbeddedDrift struct {
	// Stale files match the hash the manifest recorded when okdctl wrote
	// them — the operator never touched them; only the embedded copy moved on.
	Stale []string
	// Unverified files differ from the embedded copy but have no recorded
	// hash (unstamped root or untracked file), so okdctl cannot tell an
	// operator edit from staleness.
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
