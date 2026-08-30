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

// MaterializeTerraform writes the embedded Terraform sources under root so a
// packaged binary can deploy from an empty directory; existing files are
// never overwritten, only missing ones are created. Under sudo, the tree is
// chown'd back to the invoking user.
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
		// Lstat, not Stat: a symlink (even dangling) counts as existing, so
		// the write never chases or replaces an operator-planted link.
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
	// Only touch the manifest/chown when this run created files — a repeat
	// deploy or dry-run against a settled root leaves any existing manifest alone.
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
