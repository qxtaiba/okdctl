package main

import (
	"os"
	"strings"

	_ "github.com/qxtaiba/okd-proxmox-cli/internal/addon/catalog" // Register all built-in addons
	"github.com/qxtaiba/okd-proxmox-cli/internal/cli"
)

func init() {
	if os.Geteuid() == 0 {
		_, _ = os.Stderr.WriteString("\nerror: do not run as root/sudo. this tool uses sudo internally when needed.\n\n")
		os.Exit(1)
	}

	// /usr/local/bin may be missing from PATH when invoked via sudo
	path := os.Getenv("PATH")
	if !strings.Contains(path, "/usr/local/bin") {
		_ = os.Setenv("PATH", "/usr/local/bin:"+path)
	}
}

func main() {
	cli.Execute()
}
