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
	// terraformRootManifestName is the materialization stamp file under
	// <root>/infrastructure/terraform/.
	terraformRootManifestName = ".okdctl-terraform-root.json"

	// rootManifestSchema versions the manifest JSON layout; bump on breaking struct changes.
	rootManifestSchema = 1

	// nodeOpsRootFormat is the root-format generation; written for forward
	// compat, unused today.
	nodeOpsRootFormat = 1
)

// terraformRootManifest records the root's format and per-file sha256,
// written last so its presence certifies every managed file landed.
type terraformRootManifest struct {
	SchemaVersion int               `json:"schema_version"`
	Format        int               `json:"format"`
	Files         map[string]string `json:"files"`
}

func rootManifestPath(root string) string {
	return filepath.Join(root, "infrastructure", "terraform", terraformRootManifestName)
}

// readRootManifest returns (nil, nil) when no manifest exists or its
// SchemaVersion is unrecognized (logged at Warn); it is read-only.
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

// stampRootManifest hashes the EMBEDDED bytes (not on-disk content), so a
// later refresh can tell operator edits from pristine files. Callers MUST
// call it only after every managed file has landed — the manifest is the
// completion marker detection trusts.
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
// drift can silently persist across upgrades unless flagged.
type EmbeddedDrift struct {
	// Stale files match the manifest's recorded hash — only the embedded copy moved on.
	Stale []string
	// Unverified files differ from embedded but have no recorded hash — an
	// operator edit can't be distinguished from staleness.
	Unverified []string
}

// DetectEmbeddedDrift compares every embedded *.tf source against its
// on-disk copy under root, excluding matches, missing files, and proven
// operator edits; read-only.
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
