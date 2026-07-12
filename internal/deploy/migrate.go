package deploy

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okdctl/infrastructure"
	"github.com/qxtaiba/okdctl/internal/system"
)

// nodeOpsRootFiles are the production-root files whose embedded copies gained
// the node-lifecycle variables (worker_count, master_memory_mb,
// master_cpu_cores). Their paths are relative to the embedded FS root and
// mirror the on-disk workspace layout under <root>/infrastructure/.
var nodeOpsRootFiles = []string{
	"terraform/environments/production/variables.tf",
	"terraform/environments/production/main.tf",
}

// nodeOpsMarker is a string present only in the widened root variables.tf.
// MaterializeTerraform is write-once, so an existing workspace keeps its older
// root that lacks this variable; detecting its absence is how a node op knows
// the on-disk root predates the widening.
const nodeOpsMarker = `variable "worker_count"`

// TerraformRootSupportsNodeOps reports whether the on-disk production root
// declares the node-lifecycle variables. Returns false (not an error) when the
// workspace has not been materialized yet — the caller materializes first.
func TerraformRootSupportsNodeOps(root string) (bool, error) {
	path := filepath.Join(root, "infrastructure", nodeOpsRootFiles[0])
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read terraform root: %w", err)
	}
	return bytes.Contains(data, []byte(nodeOpsMarker)), nil
}

// MigrateTerraformRoot overwrites the production-root files that
// MaterializeTerraform wrote write-once, replacing them with the current
// embedded copies so node
// ops can drive worker_count / master sizing. Each replaced file is backed up
// to <file>.<timestamp>.pre-nodeops.bak first. Returns the files it rewrote.
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
			return migrated, fmt.Errorf("read %s: %w", target, err)
		}
		if bytes.Equal(existing, embedded) {
			continue
		}
		backup := fmt.Sprintf("%s.%s.pre-nodeops.bak", target, ts)
		if err := system.AtomicWrite(backup, existing, 0o644); err != nil {
			return migrated, fmt.Errorf("back up %s: %w", target, err)
		}
		if err := system.AtomicWrite(target, embedded, 0o644); err != nil {
			return migrated, fmt.Errorf("write %s: %w", target, err)
		}
		migrated = append(migrated, target)
	}
	if len(migrated) > 0 {
		if err := system.ChownTreeToInvokingUser(filepath.Join(root, "infrastructure")); err != nil {
			return migrated, fmt.Errorf("chown terraform root: %w", err)
		}
	}
	return migrated, nil
}
