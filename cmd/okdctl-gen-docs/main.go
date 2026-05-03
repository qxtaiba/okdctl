//go:build docs

// Package main generates Markdown reference pages for every okdctl
// subcommand and writes them to an output directory (default: docs/cli).
// Run via: go run -tags docs ./cmd/okdctl-gen-docs [-o <dir>]
package main

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
	// DisableAutoGenTag suppresses cobra's date-stamped footer; without it
	// git diff --exit-code would fire on every regeneration regardless of
	// whether any command actually changed.
	root.DisableAutoGenTag = true

	if err := doc.GenMarkdownTree(root, *outDir); err != nil {
		log.Fatalf("generate docs: %v", err)
	}
}
