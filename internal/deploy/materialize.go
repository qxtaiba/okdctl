package deploy

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okdctl/infrastructure"
	"github.com/qxtaiba/okdctl/internal/system"
)

// MaterializeTerraform writes the embedded Terraform sources (the
// proxmox-okd module and production environment, provider lock file
// included) under root so a packaged binary can deploy from an empty
// directory. Policy is write-once per file: anything already present — a
// source checkout, a prior run, or user-modified HCL — is never
// overwritten; only missing files are created. Returns the paths it
// created. Under sudo the tree is chown'd back to the invoking user so it
// stays inspectable and removable without root.
func MaterializeTerraform(root string) ([]string, error) {
	var created []string
	err := fs.WalkDir(infrastructure.TerraformFS, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		target := filepath.Join(root, "infrastructure", filepath.FromSlash(path))
		// Lstat, not Stat: a symlink (even dangling) counts as existing so
		// the write can never chase or replace an operator-planted link.
		if _, statErr := os.Lstat(target); statErr == nil {
			return nil
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return statErr
		}
		data, readErr := infrastructure.TerraformFS.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if writeErr := system.AtomicWrite(target, data, 0o644); writeErr != nil {
			return writeErr
		}
		created = append(created, target)
		return nil
	})
	if err != nil {
		return created, fmt.Errorf("materialize terraform sources: %w", err)
	}
	// Only touch the manifest and re-chown when this run actually created
	// files: a repeat deploy — or `deploy --dry-run` — against a settled root
	// writes nothing and leaves any existing manifest alone. A run that did
	// create files re-stamps, replacing recorded hashes for files it did not
	// write this run.
	if len(created) > 0 {
		if err := stampRootManifest(root, nodeOpsRootFormat); err != nil {
			return created, fmt.Errorf("stamp terraform root: %w", err)
		}
		if err := system.ChownTreeToInvokingUser(filepath.Join(root, "infrastructure")); err != nil {
			return created, fmt.Errorf("chown terraform sources: %w", err)
		}
	}
	return created, nil
}
