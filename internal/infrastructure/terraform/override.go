package terraform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/system"
)

// DestroyOverrideFileName is the transient Terraform override okdctl writes
// into the materialized proxmox-okd module directory for the duration of a
// fully-confirmed destroy, disabling the master resource's
// prevent_destroy=true backstop. The *_override.tf suffix is what makes
// Terraform merge it into the module (override files only merge within
// their own module, which is why an environments/-level override cannot
// work). It is runtime-generated, never embedded, and removed when the
// destroy finishes.
const DestroyOverrideFileName = "prevent_destroy_override.tf"

const destroyOverrideHCL = `# Written by okdctl destroy after full confirmation; deleted when the
# destroy finishes. If you are reading this outside a running destroy it is
# stale — delete it. okdctl refuses to plan or apply while it exists.
resource "proxmox_virtual_environment_vm" "master" {
  lifecycle {
    prevent_destroy = false
  }
}
`

// DestroyOverridePath returns the transient override file path inside
// moduleDir.
func DestroyOverridePath(moduleDir string) string {
	return filepath.Join(moduleDir, DestroyOverrideFileName)
}

// WriteDestroyOverride writes (or overwrites, reclaiming a stale copy from a
// crashed run) the transient prevent_destroy override into moduleDir and
// returns its path. Callers must remove it via RemoveDestroyOverride when
// the destroy finishes, success or failure.
func WriteDestroyOverride(moduleDir string) (string, error) {
	path := DestroyOverridePath(moduleDir)
	if !system.DirExists(moduleDir) {
		return "", fmt.Errorf("write destroy override: module directory not found: %s", moduleDir)
	}
	if err := system.AtomicWrite(path, []byte(destroyOverrideHCL), 0o644); err != nil {
		return "", fmt.Errorf("write destroy override: %w", err)
	}
	return path, nil
}

// RemoveDestroyOverride removes the transient override from moduleDir; a
// missing file is success.
func RemoveDestroyOverride(moduleDir string) error {
	if err := os.Remove(DestroyOverridePath(moduleDir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove destroy override: %w", err)
	}
	return nil
}

// staleDestroyOverridePath resolves the override path from the Executor's
// working directory using the pinned workspace layout
// (environments/<env> and modules/proxmox-okd share the terraform root; see
// workspace.TerraformEnvDir / workspace.TerraformModuleDir). Returns "" when
// no override file exists.
func (t *Executor) staleDestroyOverridePath() string {
	path := DestroyOverridePath(filepath.Join(t.workDir, "..", "..", "modules", "proxmox-okd"))
	if !system.FileExists(path) {
		return ""
	}
	return filepath.Clean(path)
}

// refuseStaleDestroyOverride fails closed when the transient destroy
// override is present: outside a destroy session it can only be debris from
// a crashed destroy, and letting a plan or apply run with the master guard
// disabled would silently defeat prevent_destroy.
func (t *Executor) refuseStaleDestroyOverride() error {
	path := t.staleDestroyOverridePath()
	if path == "" {
		return nil
	}
	return &errtypes.ConfigError{Msg: fmt.Sprintf(
		"stale prevent_destroy override found at %s (left by an interrupted okdctl destroy); it disables the master VM guard, so refusing to plan/apply — delete the file and re-run",
		path)}
}

// WithPreventDestroyHint appends an actionable hint (same HintAppender
// mechanics as WithLockHint) when err carries Terraform's
// "Instance cannot be destroyed" / lifecycle.prevent_destroy diagnostic:
// okdctl's transient override covers only the master resource, so a hit here
// means another resource carries its own guard.
func WithPreventDestroyHint(err error, moduleDir string) error {
	if err == nil {
		return nil
	}
	var exitErr *executor.ExitError
	stderr := err.Error()
	if errors.As(err, &exitErr) {
		stderr = exitErr.Stderr
	}
	if !strings.Contains(stderr, "prevent_destroy") {
		return err
	}
	hint := fmt.Sprintf(
		"terraform refused to destroy a resource protected by prevent_destroy; okdctl's transient override (%s) covers only the master resource — add a matching *_override.tf beside it for the refusing resource, re-run okdctl destroy, then delete it",
		DestroyOverridePath(moduleDir))
	var appender errtypes.HintAppender
	if !errors.As(err, &appender) {
		return errors.Join(&errtypes.ClusterError{Msg: hint}, err)
	}
	return appender.WithHint(hint)
}
