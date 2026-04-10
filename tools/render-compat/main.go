// Command render-compat regenerates the compatibility matrix section of
// README.md from docs/compatibility.yaml.
//
// Usage:
//
//	go run ./tools/render-compat           # regenerate README.md in place
//	go run ./tools/render-compat -check    # fail if README.md is out of sync
//
// This is invoked by the Makefile targets `compat` and `compat-check`, and
// by CI to verify the rendered matrix has not drifted from the YAML source.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	yamlPath   = "docs/compatibility.yaml"
	readmePath = "README.md"
	startMark  = "<!-- COMPAT:START -->"
	endMark    = "<!-- COMPAT:END -->"
)

type knownBroken struct {
	Version string `yaml:"version"`
	Reason  string `yaml:"reason"`
}

type component struct {
	Name        string        `yaml:"name"`
	Tested      []string      `yaml:"tested"`
	KnownBroken []knownBroken `yaml:"known_broken"`
}

type compatDoc struct {
	Components []component `yaml:"components"`
}

func main() {
	check := flag.Bool("check", false, "verify README matches the rendered compat section; exit 1 on drift")
	flag.Parse()

	raw, err := os.ReadFile(yamlPath)
	if err != nil {
		die("read %s: %v", yamlPath, err)
	}

	var doc compatDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		die("parse %s: %v", yamlPath, err)
	}

	rendered := render(&doc)

	readme, err := os.ReadFile(readmePath)
	if err != nil {
		die("read %s: %v", readmePath, err)
	}

	updated, err := replaceSection(readme, rendered)
	if err != nil {
		die("%v", err)
	}

	if *check {
		if !bytes.Equal(readme, updated) {
			fmt.Fprintln(os.Stderr, "README.md is out of sync with docs/compatibility.yaml — run `make compat`")
			os.Exit(1)
		}
		return
	}

	if bytes.Equal(readme, updated) {
		fmt.Println("compat: README.md is already up to date")
		return
	}

	// README.md must be world-readable; 0644 is intentional.
	if err := os.WriteFile(readmePath, updated, 0o644); err != nil { //nolint:gosec // public doc file
		die("write %s: %v", readmePath, err)
	}
	fmt.Println("compat: README.md updated")
}

func render(doc *compatDoc) string {
	var sb strings.Builder

	sb.WriteString(startMark + "\n")
	sb.WriteString("<!-- This section is generated from docs/compatibility.yaml. Run `make compat` to regenerate. -->\n\n")
	sb.WriteString("| Component | Tested | Known broken / unsupported |\n")
	sb.WriteString("|-----------|--------|----------------------------|\n")

	for _, c := range doc.Components {
		tested := strings.Join(c.Tested, ", ")
		if tested == "" {
			tested = "—"
		}

		var brokenParts []string
		for _, kb := range c.KnownBroken {
			brokenParts = append(brokenParts, fmt.Sprintf("`%s` (%s)", kb.Version, kb.Reason))
		}
		broken := strings.Join(brokenParts, "; ")
		if broken == "" {
			broken = "—"
		}

		fmt.Fprintf(&sb, "| %s | %s | %s |\n", c.Name, tested, broken)
	}

	sb.WriteString("\n" + endMark)
	return sb.String()
}

func replaceSection(readme []byte, replacement string) ([]byte, error) {
	startIdx := bytes.Index(readme, []byte(startMark))
	if startIdx < 0 {
		return nil, fmt.Errorf("%s marker not found in README.md", startMark)
	}
	endIdx := bytes.Index(readme, []byte(endMark))
	if endIdx < 0 {
		return nil, fmt.Errorf("%s marker not found in README.md", endMark)
	}
	if endIdx < startIdx {
		return nil, fmt.Errorf("%s appears before %s in README.md", endMark, startMark)
	}

	var out bytes.Buffer
	out.Write(readme[:startIdx])
	out.WriteString(replacement)
	out.Write(readme[endIdx+len(endMark):])
	return out.Bytes(), nil
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "render-compat: "+format+"\n", args...)
	os.Exit(1)
}
