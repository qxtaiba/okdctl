// Package main is the okdctl binary entry point; root-rejection policy lives in
// internal/cli (ensureRoot).
package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	_ "github.com/qxtaiba/okdctl/internal/addon/catalog" // Register all built-in addons
	"github.com/qxtaiba/okdctl/internal/cli"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

func main() {
	preflight()
	cli.Execute()
}

// preflight runs pre-cobra startup checks; lives in main() (not init()) so tui
// output is available when reporting errors.
func preflight() {
	// Warn before OKDCTL_BIN_DIR is silently dropped downstream.
	if v := os.Getenv("OKDCTL_BIN_DIR"); v != "" {
		expanded := system.ExpandPath(v)
		if err := config.ValidateBinDir(expanded); err != nil {
			fields := []logutil.LogField{logutil.LF("value", v), logutil.LF("err", err)}
			if strings.HasPrefix(v, "~") && expanded == v {
				fields = append(fields, logutil.LF("note", "tilde expansion failed (home dir unresolved)"))
			}
			cli.DeferWarn(func() { logutil.Warn("OKDCTL_BIN_DIR ignored", fields...) })
		}
	}
	binDir := config.PreflightBinDir()
	path := os.Getenv("PATH")
	if !slices.Contains(filepath.SplitList(path), binDir) {
		if err := os.Setenv("PATH", binDir+":"+path); err != nil {
			cli.DeferWarn(func() {
				logutil.Warn("failed to prepend bin dir to PATH", logutil.LF("bin_dir", binDir), logutil.LF("err", err))
			})
		}
	}
}
