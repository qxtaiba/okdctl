package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/tui"
)

func loadConfig(configFile string) (*config.Config, error) {
	loader := config.NewLoader()
	cfg, err := loader.LoadFile(configFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			tui.Error("configuration file not found", tui.LF("file", configFile))
			if configFile == "okdctl.yaml" {
				tui.Info("run 'okdctl deploy' to create a configuration file")
			} else {
				tui.Info("run 'okdctl deploy --output-file <file>' to create it", tui.LF("file", configFile))
			}
			return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("configuration file not found: %s", configFile), Err: errtypes.ErrConfigMissing}
		}
		return nil, &errtypes.ConfigError{Msg: "load configuration", Err: err}
	}
	return cfg, nil
}

// handleCredentials loads the .env file (non-overwriting) then resolves
// Proxmox credentials from environment and config. Returns an error if the
// .env could not be loaded — callers must not proceed in that case.
func handleCredentials(cfg *config.Config) (*credentials.ProxmoxCredentials, error) {
	envPath := credentials.EnvFilePath(cfgFile)
	if err := credentials.LoadEnvFile(envPath); err != nil {
		return nil, err
	}

	creds := credentials.GetProxmoxCredentials(cfg)
	if !creds.IsValid() {
		tui.Warn("no proxmox credentials found")
		tui.Info("set credentials via environment variables or env file", tui.LF("path", envPath))
		tui.Info("  PROXMOX_VE_USERNAME + PROXMOX_VE_PASSWORD")
		tui.Info("  or PROXMOX_VE_API_TOKEN")
	} else {
		reportCredentialProvenance(creds)
	}
	return creds, nil
}

// reportCredentialProvenance logs the resolved credential source plus any
// mixed-provenance warnings the operator should see (env-overrides-config,
// endpoint falling back to config). Callers must check creds.IsValid first.
func reportCredentialProvenance(creds *credentials.ProxmoxCredentials) {
	tui.Info("using credentials", tui.LF("source", creds.Source))
	if creds.ConfigCredentialsOverridden {
		tui.Warn("environment credentials override proxmox credentials in config file")
	}
	if creds.EndpointFromConfig {
		tui.Warn("PROXMOX_VE_ENDPOINT not set; endpoint falling back to config file (mixed source)")
	}
	if creds.Insecure {
		tui.Warn("proxmox: TLS verification disabled (insecure=true in config)")
	}
}

// planPreviewOptions configures runTerraformPlanPreview.
type planPreviewOptions struct {
	// ConfigPath derives the credentials .env file path via credentials.EnvFilePath.
	ConfigPath string
	// ProjectRoot is the workspace root containing the terraform environments dir.
	ProjectRoot string
	// Caller is the runlock verb recorded for concurrent-run diagnostics
	// (e.g. "deploy --dry-run", "plan").
	Caller string
}

// runTerraformPlanPreview loads credentials, acquires the project run lock,
// connects the proxmox provider, and runs a read-only terraform plan
// preview, returning the parsed non-no-op resource changes. It is the sole
// path to a preview plan — deploy --dry-run and okdctl plan both call it so
// the two commands cannot drift on how a preview is produced. Errors are
// returned as-is (already typed via errtypes); callers decide whether to
// wrap or let them surface.
func runTerraformPlanPreview(ctx context.Context, cfg *config.Config, opts planPreviewOptions) ([]terraform.ResourceChange, error) {
	envPath := credentials.EnvFilePath(opts.ConfigPath)
	if err := credentials.LoadEnvFile(envPath); err != nil {
		return nil, err
	}

	creds := credentials.GetProxmoxCredentials(cfg)
	defer creds.Zeroize()

	lock, err := runlock.Acquire(opts.ProjectRoot, opts.Caller)
	if err != nil {
		return nil, err
	}
	defer lock.Release()

	prov := proxmox.New(
		proxmox.WithProjectRoot(opts.ProjectRoot),
		proxmox.WithLogger(tui.SimpleLogger()),
		proxmox.WithEnv(creds.Env()),
	)
	defer prov.ZeroizeEnv()
	if err := prov.Connect(ctx, cfg); err != nil {
		return nil, err
	}

	tui.Info("plan: running terraform plan (no changes will be made)")

	return prov.PlanPreview(ctx, cfg, proxmox.ProvisionOptions{
		ProjectRoot:  opts.ProjectRoot,
		TerraformEnv: phase.GetTerraformEnv(cfg),
	})
}

func validateConfig(cfg *config.Config, w io.Writer) *config.ValidationResult {
	result := cfg.Validate()
	if !result.IsValid() {
		fmt.Fprintln(w, render.ValidationSummary(result))
	}
	return result
}

func resolveProjectRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(wd)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// EvalSymlinks fails with ErrNotExist when a path component does not
		// exist yet (e.g., macOS temp dirs on startup). That case is benign:
		// fall back to the absolute path. Any other error — EPERM, EIO, etc.
		// — is a real filesystem problem that must not be silently swallowed,
		// because abs may still be a symlink and handing it to sudo-elevated
		// helpers would let an attacker redirect writes to arbitrary paths.
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("resolve project root symlinks: %w", err)
		}
		return abs, nil
	}
	return resolved, nil
}

// resolveWorkspaceRoot resolves the current directory as the workspace root
// without requiring project markers. Deploy uses it directly: deploy is the
// command that creates the markers, so gating it on their presence would be
// circular.
func resolveWorkspaceRoot() (string, error) {
	root, err := resolveProjectRoot()
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	if root == "" {
		return "", fmt.Errorf("project root resolved to empty path")
	}
	return root, nil
}

func resolveProjectRootOrDie() (string, error) {
	root, err := resolveWorkspaceRoot()
	if err != nil {
		return "", err
	}
	// Project marker check: okdctl.yaml and okdctl.env are the primary
	// markers. terraform.tfstate is a secondary recovery hint — it can
	// outlive a successful destroy+cleanup, which preserves it for
	// resumability. At least one of the three must be present; a lone
	// tfstate triggers an explicit operator warning (warnIfTfStateOnly)
	// because it may belong to a different cluster.
	if !hasProjectMarker(root) {
		return "", &errtypes.ConfigError{
			Msg: fmt.Sprintf(
				"no cluster in %s "+
					"(checked okdctl.yaml, okdctl.env, "+
					"infrastructure/terraform/environments/*/terraform.tfstate); "+
					"run 'okdctl deploy' first",
				root,
			),
			Err: errtypes.ErrConfigMissing,
		}
	}
	warnIfTfStateOnly(root)
	return root, nil
}

// hasPrimaryMarker reports whether root contains okdctl's primary config
// markers: the configured config-file name or okdctl.env. Both are
// exclusively written by okdctl inside a project root.
func hasPrimaryMarker(root string) bool {
	for _, name := range []string{filepath.Base(cfgFile), "okdctl.env"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	return false
}

func terraformStateMatches(root string) []string {
	matches, _ := filepath.Glob(
		filepath.Join(root, "infrastructure", "terraform", "environments", "*", "terraform.tfstate"),
	)
	return matches
}

// hasProjectMarker reports whether root contains at least one okdctl project
// marker: a primary marker (see hasPrimaryMarker) or a terraform.tfstate
// under infrastructure/terraform/environments/.
func hasProjectMarker(root string) bool {
	return hasPrimaryMarker(root) || len(terraformStateMatches(root)) > 0
}

// warnIfTfStateOnly emits a structured warning when the only project marker
// is terraform.tfstate (okdctl.yaml and okdctl.env both absent). A lone
// tfstate after destroy+cleanup may belong to a different cluster if the
// operator removed the primary config files manually.
func warnIfTfStateOnly(root string) {
	if hasPrimaryMarker(root) {
		return
	}
	matches := terraformStateMatches(root)
	if len(matches) == 0 {
		return
	}
	tui.Warn("okdctl.yaml and okdctl.env not found; accepting terraform.tfstate as a recovery hint",
		tui.LF("tfstate", matches[0]),
		tui.LF("root", root),
	)
	tui.Info("if this directory belongs to a different cluster, stop and run 'okdctl deploy' in the correct directory")
}
