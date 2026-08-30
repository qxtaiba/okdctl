//go:build docs

// Package main generates Markdown reference pages for every okdctl
// subcommand. Run via: go run -tags docs ./cmd/okdctl-gen-docs [-o <dir>]
package main

// Uses stdlib log, not slog/tui: this never links into the shipped binary.
import (
	"flag"
	"log"
	"os"

	_ "github.com/qxtaiba/okdctl/internal/addon/catalog"
	"github.com/qxtaiba/okdctl/internal/cli"
	"github.com/spf13/cobra/doc"
)

func main() {
	outDir := flag.String("o", "docs/cli", "output directory for generated Markdown")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	root := cli.RootCmd()
	// Drops cobra's date-stamped footer so regeneration diffs are stable.
	root.DisableAutoGenTag = true

	if err := doc.GenMarkdownTree(root, *outDir); err != nil {
		log.Fatalf("generate docs: %v", err)
	}
}
