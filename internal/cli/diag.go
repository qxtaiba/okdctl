package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/platform"
	"github.com/qxtaiba/okd-proxmox-cli/pkg/version"
)

var (
	diagOutput string
	diagStdout bool
)

var diagCmd = &cobra.Command{
	Use:   "diag",
	Short: "Collect a diagnostic bundle for bug reports",
	Long: `Collect a sanitized diagnostic bundle.

The bundle captures: openshitctl version info, host OS and tool
versions, a sanitized environment variable dump, and your effective
config file with secrets masked. It is safe to attach to a public
issue or paste into a bug report.

Sanitization is unconditional. Environment variables matching common
secret name patterns (PASSWORD, TOKEN, SECRET, CREDENTIAL, API_KEY,
PRIVATE_KEY, ACCESS_KEY) are masked as '***', and Proxmox credential
fields in the config are zeroed before marshalling.

By default writes a timestamped tar.gz in the current directory.
Pass --stdout to dump the bundle as plain text for pasting.`,
	RunE: runDiag,
}

func init() {
	diagCmd.Flags().StringVarP(&diagOutput, "output", "o", "", "output file (default: diag-<timestamp>.tar.gz in current directory)")
	diagCmd.Flags().BoolVar(&diagStdout, "stdout", false, "write the bundle as plain text to stdout instead of a tar.gz")
	rootCmd.AddCommand(diagCmd)
}

// diagSection is one logical slice of a diagnostic bundle, named and
// rendered as a single text file in the output tarball.
type diagSection struct {
	name    string // becomes <name>.txt in the tarball
	content string
}

func runDiag(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	sections := []diagSection{
		{"version", collectVersion()},
		{"system", collectSystem()},
		{"tool_versions", collectToolVersions(ctx)},
		{"env_sanitized", collectEnvSanitized()},
		{"config_sanitized", collectConfigSanitized(cfgFile)},
	}

	if diagStdout {
		for _, s := range sections {
			fmt.Printf("=== %s ===\n%s\n\n", s.name, s.content)
		}
		return nil
	}

	outPath := diagOutput
	if outPath == "" {
		outPath = fmt.Sprintf("diag-%s.tar.gz", time.Now().UTC().Format("20060102-150405"))
	}
	if err := writeTarball(outPath, sections); err != nil {
		return fmt.Errorf("failed to write bundle: %w", err)
	}

	tui.Info(fmt.Sprintf("diag: bundle written to %s", outPath))
	tui.Info("diag: attach this file to your issue (or paste the output of --stdout)")
	return nil
}

func collectVersion() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "version:    %s\n", version.Version)
	fmt.Fprintf(&sb, "git_commit: %s\n", version.GitCommit)
	fmt.Fprintf(&sb, "build_date: %s\n", version.BuildDate)
	fmt.Fprintf(&sb, "go_version: %s\n", version.GoVersion)
	fmt.Fprintf(&sb, "platform:   %s\n", version.Platform)
	return sb.String()
}

func collectSystem() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "goos:         %s\n", runtime.GOOS)
	fmt.Fprintf(&sb, "goarch:       %s\n", runtime.GOARCH)
	fmt.Fprintf(&sb, "num_cpu:      %d\n", runtime.NumCPU())

	if runtime.GOOS == goosDarwin {
		fmt.Fprintln(&sb, "host_os:      macOS (operator mode)")
		return sb.String()
	}

	host, err := platform.Detect()
	if err != nil {
		fmt.Fprintf(&sb, "host_os:      unknown (%s)\n", err)
		return sb.String()
	}
	fmt.Fprintf(&sb, "host_os:      %s %s\n", host.ID, host.Version)
	fmt.Fprintf(&sb, "host_family:  %s\n", host.Family)
	return sb.String()
}

// collectToolVersions probes the CLIs openshitctl shells out to and
// records their self-reported versions. A missing tool yields "(not
// installed)" — not an error. Cold-cache invocations on macOS can take
// 5-8s due to Gatekeeper/Rosetta, hence the 10s timeout.
func collectToolVersions(ctx context.Context) string {
	tools := []struct {
		name string
		args []string
	}{
		{"oc", []string{"version", "--client=true", "--output=yaml"}},
		{"openshift-install", []string{"version"}},
		{"terraform", []string{"version"}},
		{"cosign", []string{"version"}},
		{"syft", []string{"version"}},
		{"git", []string{"--version"}},
		{"curl", []string{"--version"}},
		{"ssh", []string{"-V"}},
	}

	var sb strings.Builder
	for _, t := range tools {
		fmt.Fprintf(&sb, "=== %s ===\n", t.name)
		if _, err := exec.LookPath(t.name); err != nil {
			fmt.Fprintln(&sb, "(not installed)")
			fmt.Fprintln(&sb)
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		// Tool names are hardcoded constants from the loop above, not user input.
		out, err := exec.CommandContext(cctx, t.name, t.args...).CombinedOutput() //nolint:gosec // G204: hardcoded tool names
		cancel()
		if err != nil {
			fmt.Fprintf(&sb, "(error running %s: %v)\n", t.name, err)
			fmt.Fprintln(&sb)
			continue
		}
		// First 20 lines max, to keep the bundle terse.
		lines := strings.SplitN(string(out), "\n", 21)
		if len(lines) > 20 {
			lines = lines[:20]
			lines = append(lines, "(truncated)")
		}
		fmt.Fprintln(&sb, strings.Join(lines, "\n"))
		fmt.Fprintln(&sb)
	}
	return sb.String()
}

// sensitiveKeyParts is the set of substrings (matched case-insensitively
// against environment variable names) that trigger value masking in the
// env dump. Sourced from envchain / dotenv-linter convention. Keep narrow
// enough to avoid false positives like AUTHOR matching AUTH; missing a
// real secret is a security bug.
var sensitiveKeyParts = []string{
	"PASSWORD",
	"PASSWD",
	"TOKEN",
	"SECRET",
	"CREDENTIAL",
	"API_KEY",
	"APIKEY",
	"PRIVATE_KEY",
	"ACCESS_KEY",
}

func isSensitiveKey(k string) bool {
	up := strings.ToUpper(k)
	for _, part := range sensitiveKeyParts {
		if strings.Contains(up, part) {
			return true
		}
	}
	return false
}

func collectEnvSanitized() string {
	env := os.Environ()
	sort.Strings(env)

	var sb strings.Builder
	for _, kv := range env {
		key, value, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if isSensitiveKey(key) {
			fmt.Fprintf(&sb, "%s=***\n", key)
			continue
		}
		fmt.Fprintf(&sb, "%s=%s\n", key, value)
	}
	return sb.String()
}

// collectConfigSanitized loads the named config file, zeroes out the
// Proxmox credential fields on the in-memory config, and marshals the
// result. If no config file exists, reports that instead of failing.
// Takes path as a parameter (rather than reading the cli package global)
// so it is unit-testable.
func collectConfigSanitized(path string) string {
	if path == "" {
		path = "openshitctl.yaml"
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("(no config file at %s; skipping)\n", path)
		}
		return fmt.Sprintf("(error stating %s: %v)\n", path, err)
	}

	loader := config.NewLoader()
	cfg, err := loader.LoadFile(path)
	if err != nil {
		return fmt.Sprintf("(error loading %s: %v)\n", path, err)
	}

	// Zero credential fields before re-marshalling so the saved copy
	// carries none. This is belt-and-braces — the YAML format already
	// omits empty credential fields — but we want explicit guarantees
	// when producing a diagnostic bundle.
	if cfg.Provider.Proxmox != nil {
		cfg.Provider.Proxmox.Password = ""
		cfg.Provider.Proxmox.APIToken = ""
	}

	// Write to a temp file via the loader's Save path, then read back.
	// This keeps the on-disk format identical to what Save produces.
	tmp, err := os.CreateTemp("", "diag-config-*.yaml")
	if err != nil {
		return fmt.Sprintf("(error creating temp file: %v)\n", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := loader.Save(cfg, tmpPath); err != nil {
		return fmt.Sprintf("(error saving sanitized config: %v)\n", err)
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Sprintf("(error reading sanitized config: %v)\n", err)
	}
	return string(data)
}

func writeTarball(path string, sections []diagSection) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	// Safety net: deferred close runs after any explicit close below.
	defer func() { _ = f.Close() }()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	now := time.Now()
	writeEntry := func(name, content string) error {
		body := []byte(content)
		hdr := &tar.Header{
			Name:    name + ".txt",
			Mode:    0o644,
			Size:    int64(len(body)),
			ModTime: now,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("tar header for %s: %w", name, err)
		}
		if _, err := tw.Write(body); err != nil {
			return fmt.Errorf("tar body for %s: %w", name, err)
		}
		return nil
	}

	for _, s := range sections {
		if err := writeEntry(s.name, s.content); err != nil {
			return err
		}
	}
	// Top-level README describes the bundle format and sanitization rules
	// so whoever opens it — possibly years later — understands what's in
	// it without reading source.
	if err := writeEntry("README", generateDiagReadme(sections)); err != nil {
		return err
	}

	// Explicit close order: tar → gzip → file. Errors from these closes
	// (gzip flush, disk full, quota) must propagate so a truncated bundle
	// is never silently shipped.
	if err := tw.Close(); err != nil {
		return fmt.Errorf("close tar writer: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("close gzip writer: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close output file: %w", err)
	}
	return nil
}

func generateDiagReadme(sections []diagSection) string {
	var names bytes.Buffer
	for _, s := range sections {
		fmt.Fprintf(&names, "  - %s.txt\n", s.name)
	}
	return fmt.Sprintf(`openshitctl diagnostic bundle
==============================

Generated by 'openshitctl diag' on %s.

This bundle was automatically sanitized: environment variable names
containing any of PASSWORD, PASSWD, TOKEN, SECRET, CREDENTIAL, API_KEY,
APIKEY, PRIVATE_KEY, or ACCESS_KEY have their values masked as '***',
and Proxmox credential fields in the config file are zeroed before
marshalling. It is safe to attach to a public issue or paste into a
bug report.

Contents:

%s
Version: v%s (%s)
`, time.Now().UTC().Format(time.RFC3339), names.String(), strings.TrimPrefix(version.Version, "v"), version.GitCommit)
}
