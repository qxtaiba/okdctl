package cleanup

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/workspace"
)

// Summary is the post-cleanup inventory the CLI renders for the operator.
type Summary struct {
	RemainingWorkFiles      int
	RemainingIgnitionFiles  int
	RemainingTerraformFiles int
	WorkDirSize             string
}

// GenerateSummary returns a post-cleanup inventory for opts; non-existent paths
// count as zero remaining.
func GenerateSummary(opts *Options) Summary {
	summary := Summary{
		WorkDirSize: "0B",
	}

	if _, err := os.Stat(opts.WorkDir); err == nil {
		count, size := collectDirStats(opts.WorkDir)
		summary.RemainingWorkFiles = count
		summary.WorkDirSize = formatSize(size)
	}

	ignitionDir := filepath.Join(opts.HTTPServerRoot, "ignition")
	if files, err := filepath.Glob(filepath.Join(ignitionDir, "*.ign")); err == nil {
		summary.RemainingIgnitionFiles = len(files)
	}

	if opts.Kind == Full || opts.Kind == TerraformOnly {
		terraformBase := workspace.TerraformEnvDir(opts.ProjectRoot, "")
		if entries, err := os.ReadDir(terraformBase); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				summary.RemainingTerraformFiles += countTerraformArtifacts(filepath.Join(terraformBase, entry.Name()))
			}
		}
	}

	return summary
}

// countTerraformArtifacts counts leftover terraform files in envDir; tfstate is
// deliberately excluded.
func countTerraformArtifacts(envDir string) int {
	count := 0
	for _, f := range []string{"terraform.tfvars", "tfplan", "destroy.tfplan", "terraform.tfstate.backup"} {
		if _, err := os.Stat(filepath.Join(envDir, f)); err == nil {
			count++
		}
	}
	return count
}

func printSummary(opts *Options, t *cleanupTracker, logger *slog.Logger) {
	summary := GenerateSummary(opts)

	logger.Info("cleanup: summary")

	if summary.RemainingWorkFiles == 0 {
		logger.Info("cleanup: work directory clean", "count", 0)
	} else {
		logger.Info("cleanup: work directory files remaining", "count", summary.RemainingWorkFiles, "size", summary.WorkDirSize)
	}

	if summary.RemainingIgnitionFiles == 0 {
		logger.Info("cleanup: ignition files clean", "count", 0)
	} else {
		logger.Info("cleanup: ignition files remaining", "count", summary.RemainingIgnitionFiles)
	}

	if opts.Kind == Full || opts.Kind == TerraformOnly {
		if summary.RemainingTerraformFiles == 0 {
			logger.Info("cleanup: terraform clean", "count", 0)
		} else {
			logger.Info("cleanup: terraform files remaining", "count", summary.RemainingTerraformFiles)
		}
	}

	totalRemaining := summary.RemainingWorkFiles + summary.RemainingIgnitionFiles + summary.RemainingTerraformFiles
	switch {
	case len(t.names) > 0:
		logger.Warn("cleanup: partial cleanup; rerun to retry; subsystems still active",
			"failed_steps", t.names)
	case totalRemaining == 0:
		if opts.Kind == Full {
			logger.Info("cleanup: completed")
			logger.Info("cleanup: system ready for fresh deployment")
		} else {
			logger.Info("cleanup: completed for scope", "scope", opts.Kind)
		}
	default:
		logger.Warn("cleanup: partial completion")
		logger.Info("cleanup: files remain (this may be normal)", "count", totalRemaining)
	}
}

func collectDirStats(path string) (count int, totalSize int64) {
	_ = filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		count++
		totalSize += info.Size()
		return nil
	})
	return count, totalSize
}

func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1fG", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1fM", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1fK", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
