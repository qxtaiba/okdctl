package cleanup

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// Summary is the post-cleanup accounting the CLI renders for the operator —
// how many files survived each removal pass, and the work-dir footprint.
type Summary struct {
	RemainingWorkFiles      int
	RemainingIgnitionFiles  int
	RemainingTerraformFiles int
	WorkDirSize             string
}

// GenerateSummary walks the known cleanup targets under opts.WorkDir,
// opts.HTTPServerRoot, and the terraform environments directory and returns
// a Summary. Non-existent paths are treated as "zero remaining" rather than
// an error.
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
		terraformBase := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments")
		if entries, err := os.ReadDir(terraformBase); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					envDir := filepath.Join(terraformBase, entry.Name())
					// Don't count terraform.tfstate - it's intentionally preserved for destroy
					for _, f := range []string{"terraform.tfvars", "tfplan", "destroy.tfplan", "terraform.tfstate.backup"} {
						if _, err := os.Stat(filepath.Join(envDir, f)); err == nil {
							summary.RemainingTerraformFiles++
						}
					}
				}
			}
		}
	}

	return summary
}

func printSummary(opts *Options, logger *slog.Logger) {
	summary := GenerateSummary(opts)

	logger.Info("cleanup: summary")

	if summary.RemainingWorkFiles == 0 {
		logger.Info("  work directory: clean (0 files)")
	} else {
		logger.Info("  work directory: files remaining", "count", summary.RemainingWorkFiles, "size", summary.WorkDirSize)
	}

	if summary.RemainingIgnitionFiles == 0 {
		logger.Info("  ignition files: clean (0 files)")
	} else {
		logger.Info(fmt.Sprintf("  ignition files: %d files remaining", summary.RemainingIgnitionFiles))
	}

	if opts.Kind == Full || opts.Kind == TerraformOnly {
		if summary.RemainingTerraformFiles == 0 {
			logger.Info("  terraform: clean (0 files)")
		} else {
			logger.Info(fmt.Sprintf("  terraform: %d files remaining", summary.RemainingTerraformFiles))
		}
	}

	totalRemaining := summary.RemainingWorkFiles + summary.RemainingIgnitionFiles + summary.RemainingTerraformFiles
	if totalRemaining == 0 {
		if opts.Kind == Full {
			logger.Info("cleanup: completed successfully")
			logger.Info("cleanup: system ready for fresh deployment")
		} else {
			logger.Info(fmt.Sprintf("cleanup: completed for scope %s", opts.Kind))
		}
	} else {
		logger.Warn("cleanup: partial completion")
		logger.Info(fmt.Sprintf("cleanup: %d files remain (this may be normal)", totalRemaining))
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
