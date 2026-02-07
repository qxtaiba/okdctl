// Package main is the entry point for the openshitctl CLI.
package main

import (
	"os"
	"strings"

	_ "github.com/qxtaiba/okd-proxmox-cli/internal/addon/catalog" // Register all built-in addons
	"github.com/qxtaiba/okd-proxmox-cli/internal/cli"
)

func init() {
	// Block running as root/sudo - the tool uses sudo internally when needed
	if os.Geteuid() == 0 {
		_, _ = os.Stderr.WriteString("\nerror: do not run as root/sudo. this tool uses sudo internally when needed.\n\n")
		os.Exit(1)
	}

	// Ensure /usr/local/bin is in PATH (may be missing when running as sudo)
	path := os.Getenv("PATH")
	if !strings.Contains(path, "/usr/local/bin") {
		_ = os.Setenv("PATH", "/usr/local/bin:"+path)
	}
}

func main() {
	cli.Execute()
}
