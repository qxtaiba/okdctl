// Package main is the okdctl binary entry point; root-rejection policy lives in
// internal/cli (ensureRoot).
package main

import "github.com/qxtaiba/okdctl/internal/cli"

func main() {
	cli.Execute()
}
