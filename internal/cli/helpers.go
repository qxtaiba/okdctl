package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

func loadConfig(configFile string) (*config.Config, error) {
	loader := config.NewLoader()
	cfg, err := loader.LoadFile(configFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if configFile == "okdctl.yaml" {
				logutil.Info("run 'okdctl deploy' to create a configuration file")
			} else {
				logutil.Info("run 'okdctl deploy --output-file <file>' to create it", logutil.LF("file", configFile))
			}
			return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("configuration file not found: %s", configFile), Err: errtypes.ErrConfigMissing}
		}
		return nil, &errtypes.ConfigError{Msg: "load configuration", Err: err}
	}
	return cfg, nil
}

// handleCredentials loads the .env file then resolves credentials; callers must
// not proceed if it returns an error.
func handleCredentials(cfg *config.Config) (*credentials.ProxmoxCredentials, error) {
	envPath := credentials.EnvFilePath(cfgFile)
	if err := credentials.LoadEnvFile(envPath); err != nil {
		return nil, err
	}

	creds := credentials.GetProxmoxCredentials(cfg)
	if !creds.IsValid() {
		logutil.Warn("no proxmox credentials found")
		logutil.Info("set credentials via environment variables or env file",
			logutil.LF("path", envPath),
			logutil.LF("vars", "PROXMOX_VE_USERNAME+PROXMOX_VE_PASSWORD or PROXMOX_VE_API_TOKEN"))
	} else {
		reportCredentialProvenance(creds)
	}
	return creds, nil
}

// reportCredentialProvenance logs credential source and mixed-provenance
// warnings; callers must check creds.IsValid first.
func reportCredentialProvenance(creds *credentials.ProxmoxCredentials) {
	logutil.Info("using credentials", logutil.LF("source", creds.Source))
	if creds.ConfigCredentialsOverridden {
		logutil.Warn("environment credentials override proxmox credentials in config file")
	}
	if creds.EndpointFromConfig {
		logutil.Warn("PROXMOX_VE_ENDPOINT not set; endpoint falling back to config file (mixed source)")
	}
	if creds.Insecure {
		logutil.Warn("proxmox: TLS verification disabled (insecure=true in config)")
	}
}

type planPreviewOptions struct {
	// derives the .env file path via credentials.EnvFilePath
	ConfigPath string
	// contains the terraform environments dir
	ProjectRoot string
	// runlock verb recorded for concurrent-run diagnostics (e.g. "plan")
	Caller string
}

// runTerraformPlanPreview is the sole preview-plan path; deploy --dry-run and
// okdctl plan both call it so they cannot drift on how a preview is produced.
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
		proxmox.WithLogger(logutil.SimpleLogger()),
		proxmox.WithEnv(creds.Env()),
	)
	defer prov.ZeroizeEnv()
	if err := prov.Connect(ctx, cfg); err != nil {
		return nil, err
	}

	return prov.PlanPreview(ctx, cfg, proxmox.ProvisionOptions{
		ProjectRoot:  opts.ProjectRoot,
		TerraformEnv: cfg.TerraformEnvName(),
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
		// ErrNotExist here is benign (path not created yet); any other error
		// must not be swallowed, since a still-symlinked abs handed to
		// sudo-elevated helpers could redirect writes.
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("resolve project root symlinks: %w", err)
		}
		return abs, nil
	}
	return resolved, nil
}

// resolveWorkspaceRoot resolves cwd without requiring project markers; deploy
// uses it directly since deploy is what creates the markers.
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
	// tfstate is a secondary recovery hint that can outlive destroy+cleanup; a
	// lone tfstate warns (warnIfTfStateOnly) since it may belong to a different
	// cluster.
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

// hasPrimaryMarker reports whether root has the config file or okdctl.env; both
// are written exclusively by okdctl.
func hasPrimaryMarker(root string) bool {
	return slices.ContainsFunc([]string{filepath.Base(cfgFile), "okdctl.env"}, func(name string) bool {
		_, err := os.Stat(filepath.Join(root, name))
		return err == nil
	})
}

func terraformStateMatches(root string) []string {
	matches, _ := filepath.Glob(filepath.Join(workspace.TerraformEnvDir(root, "*"), "terraform.tfstate"))
	return matches
}

// hasProjectMarker reports whether root has a primary marker (see
// hasPrimaryMarker) or a terraform.tfstate.
func hasProjectMarker(root string) bool {
	return hasPrimaryMarker(root) || len(terraformStateMatches(root)) > 0
}

// warnIfTfStateOnly warns when tfstate is the only marker present, since it may
// belong to a different cluster.
func warnIfTfStateOnly(root string) {
	if hasPrimaryMarker(root) {
		return
	}
	matches := terraformStateMatches(root)
	if len(matches) == 0 {
		return
	}
	logutil.Warn(
		"okdctl.yaml and okdctl.env not found; accepting terraform.tfstate as a recovery hint",
		logutil.LF("path", matches[0]),
		logutil.LF("dir", root),
	)
	logutil.Info("if this directory belongs to a different cluster, stop and run 'okdctl deploy' in the correct directory")
}

// refuseInFlightNodeOp refuses when an interrupted node op would undercount
// nodes in a full reconcile; ack mirrors --acknowledge-interrupted-op.
func refuseInFlightNodeOp(projectRoot string, cfg *config.Config, ack bool) error {
	m, err := node.ReadOpMarker(workspace.WorkDir(projectRoot), cfg.Cluster.Name)
	if err != nil {
		return err
	}
	if m == nil || m.CompletedAddResidue(cfg.Topology.Workers.Count) {
		return nil
	}
	if ack {
		logutil.Warn("deploy: overriding in-flight node op marker",
			logutil.LF("op", string(m.Op)),
			logutil.LF("target", m.Target),
			logutil.LF("step", string(m.Step)))
		return nil
	}
	return &errtypes.ConfigError{Msg: fmt.Sprintf(
		"an interrupted node %s for %q is in flight (stopped before step %q); deploy would reconcile a topology that undercounts it and destroy the in-flight node(s) — finish the %s first, or re-run deploy with --acknowledge-interrupted-op to override",
		m.Op, m.Target, m.Step, m.Op)}
}

// announceInFlightNodeOp surfaces an in-flight node-op marker before
// destroy/plan confirms; best-effort, the command proceeds regardless.
func announceInFlightNodeOp(projectRoot string, cfg *config.Config) {
	m, err := node.ReadOpMarker(workspace.WorkDir(projectRoot), cfg.Cluster.Name)
	if err != nil {
		logutil.Warn("could not read the node op marker", logutil.LF("err", err))
		return
	}
	if m == nil || m.CompletedAddResidue(cfg.Topology.Workers.Count) {
		return
	}
	logutil.Warn("an interrupted node op is in flight; terraform state includes its partial work",
		logutil.LF("op", string(m.Op)),
		logutil.LF("target", m.Target),
		logutil.LF("step", string(m.Step)))
}
