// Package main is the entry point for the openshitctl binary, which deploys
// OKD clusters on Proxmox. It refuses to run as root and delegates all CLI
// handling to internal/cli.
package main

import (
	"os"
	"strings"

	_ "github.com/qxtaiba/okd-proxmox-cli/internal/addon/catalog" // Register all built-in addons
	"github.com/qxtaiba/okd-proxmox-cli/internal/cli"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
)

func main() {
	preflight()
	cli.Execute()
}

// preflight runs startup checks that must happen before the cobra command
// tree runs. It intentionally lives in main() rather than init() so tui
// output is available when reporting errors.
func preflight() {
	if os.Geteuid() == 0 {
		tui.Error("do not run as root/sudo. this tool uses sudo internally when needed.")
		os.Exit(1)
	}

	// /usr/local/bin may be missing from PATH when invoked via sudo
	path := os.Getenv("PATH")
	if !strings.Contains(path, "/usr/local/bin") {
		if err := os.Setenv("PATH", "/usr/local/bin:"+path); err != nil {
			tui.Warn("failed to prepend /usr/local/bin to PATH: " + err.Error())
		}
	}
}
