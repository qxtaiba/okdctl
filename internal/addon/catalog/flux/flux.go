// Package flux provides the FluxCD GitOps addon, which bootstraps Flux
// controllers into an OKD cluster and registers them via the addon system's
// init()-based catalog.
package flux

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/system"
)

const (
	defaultControllerTimeout  = 5 * time.Minute
	defaultGitRepoSyncTimeout = 3 * time.Minute
)

// Settings keys consumed by the Flux addon. Named here so callers (install
// wizard, validators, gitops bootstrap) reference the same string.
const (
	SettingRepository        = "repository"
	SettingBranch            = "branch"
	SettingPath              = "path"
	SettingProvider          = "provider"
	SettingControllerTimeout = "controller_timeout"
	SettingGitSyncTimeout    = "git_sync_timeout"
)

// k8sBoolTrue is the literal Kubernetes returns for boolean-valued status
// fields (e.g. status.conditions[?(@.type=="Ready")].status). External API
// contract — do not change.
const k8sBoolTrue = "True"

// validSyncPath matches safe sync paths: alphanumeric, slashes, underscores, dots, and hyphens.
var validSyncPath = regexp.MustCompile(`^[a-zA-Z0-9/_.\-]+$`)

func init() {
	if err := addon.Register(&Flux{}); err != nil {
		panic(err)
	}
}

// Flux is the addon.Addon implementation for Flux GitOps.
type Flux struct{}

// Info returns the addon metadata.
func (f *Flux) Info() addon.Metadata {
	return addon.Metadata{
		Name:           "flux",
		DisplayName:    "Flux GitOps",
		Description:    "Continuous delivery using Flux GitOps controller",
		Category:       "gitops",
		Dependencies:   nil,
		Priority:       80,
		DefaultEnabled: false,
	}
}

// Install provisions flux-operator and flux-instance via Helm and waits for
// controllers and the initial git sync.
func (f *Flux) Install(ctx context.Context, env *addon.Environment) error {
	if !executor.CommandExists("helm") {
		return &errtypes.ConfigError{Msg: "helm is required to install Flux"}
	}

	decoded, err := f.DecodeSettings(env.AddonConfig.Settings)
	if err != nil {
		return fmt.Errorf("flux: invalid settings: %w", err)
	}
	fs := decoded.(Settings)

	if err := addon.EnsureNamespace(ctx, env, "flux-system"); err != nil {
		return err
	}

	if err := addon.RetryDefault(ctx, func() error {
		return f.createDeployKeySecret(ctx, env, fs)
	}); err != nil {
		return fmt.Errorf("deploy key secret required: %w", err)
	}

	if err := f.installOperator(ctx, env); err != nil {
		return err
	}

	if err := f.installInstance(ctx, env, fs); err != nil {
		return err
	}

	// Wait for Flux controllers to become available (fatal if they don't start)
	if err := f.waitForControllers(ctx, env); err != nil {
		return err
	}

	// Wait for GitRepository sync (non-fatal — user may need to fix deploy key or URL)
	if err := f.waitForGitSync(ctx, env); err != nil {
		env.Logger.Warn("flux: git sync not ready", "err", err)
		env.Logger.Warn("flux: debug with: oc get gitrepository -n flux-system -o yaml")
		env.Logger.Warn("flux: the cluster will auto-reconcile once the git source is reachable")
	}

	env.Logger.Info("flux: gitops installed and syncing with repository")
	return nil
}

// helmUpgradeInstall wraps the shared "helm upgrade --install ... --wait"
// invocation used for both flux-operator and flux-instance. extraArgs is
// appended after the common --namespace flag (so callers can pass --set
// pairs or --create-namespace).
func (f *Flux) helmUpgradeInstall(ctx context.Context, env *addon.Environment, release, chart, errLabel string, extraArgs ...string) error {
	return addon.RetryDefault(ctx, func() error {
		args := []string{"upgrade", "--install", release, chart, "--namespace", "flux-system"}
		args = append(args, extraArgs...)
		args = append(args, "--wait")
		if _, err := env.Exec.RunChecked(ctx, "helm", args...); err != nil {
			return fmt.Errorf("failed to install %s: %w", errLabel, err)
		}
		return nil
	})
}

func (f *Flux) installOperator(ctx context.Context, env *addon.Environment) error {
	env.Logger.Info("flux: installing operator via helm")
	if err := f.helmUpgradeInstall(ctx, env, "flux-operator",
		"oci://ghcr.io/controlplaneio-fluxcd/charts/flux-operator",
		"flux operator", "--create-namespace"); err != nil {
		return err
	}
	if _, err := env.Exec.RunChecked(ctx, "oc", "wait", "--for=condition=available", "deployment/flux-operator",
		"--namespace", "flux-system", "--timeout=120s"); err != nil {
		return fmt.Errorf("flux: operator not ready within 120s timeout: %w", err)
	}
	return nil
}

func (f *Flux) installInstance(ctx context.Context, env *addon.Environment, fs Settings) error {
	env.Logger.Info("flux: installing instance for gitops sync")
	if fs.Repository == "" {
		return &errtypes.ConfigError{Msg: "flux repository not configured - set addons.flux.settings.repository in config"}
	}
	return f.helmUpgradeInstall(ctx, env, "flux-instance",
		"oci://ghcr.io/controlplaneio-fluxcd/charts/flux-instance",
		"flux instance",
		"--set", "instance.cluster.type=openshift",
		"--set", fmt.Sprintf("instance.sync.url=%s", fs.Repository),
		"--set", fmt.Sprintf("instance.sync.ref=refs/heads/%s", fs.Branch),
		"--set", fmt.Sprintf("instance.sync.path=%s", fs.Path),
		"--set", "instance.sync.pullSecret=flux-system",
	)
}

// Verify reports whether the flux-operator and source-controller deployments
// have ready replicas. GitRepository sync status is logged but non-fatal.
func (f *Flux) Verify(ctx context.Context, env *addon.Environment) error {
	result, err := env.Exec.RunChecked(ctx, "oc", "get", "deployment", "flux-operator",
		"-n", "flux-system", "-o", "jsonpath={.status.readyReplicas}")
	if err != nil {
		return fmt.Errorf("flux-operator deployment not found: %w", err)
	}
	ready := strings.TrimSpace(result.Stdout)
	if ready == "" || ready == "0" {
		return fmt.Errorf("flux-operator has no ready replicas")
	}
	env.Logger.Info(fmt.Sprintf("flux: operator ready (%s replicas)", ready))

	result, err = env.Exec.Run(ctx, "oc", "get", "deployment", "source-controller",
		"-n", "flux-system", "-o", "jsonpath={.status.readyReplicas}")
	if err != nil || result.ExitCode != 0 {
		env.Logger.Warn("flux: cannot query source-controller deployment; verification skipped")
	} else {
		scReady := strings.TrimSpace(result.Stdout)
		if scReady == "" || scReady == "0" {
			return fmt.Errorf("source-controller has no ready replicas")
		}
		env.Logger.Info(fmt.Sprintf("flux: source-controller ready (%s replicas)", scReady))
	}

	// Check GitRepository sync status (informational, not fatal for verify)
	result, err = env.Exec.Run(ctx, "oc", "get", "gitrepository", "-n", "flux-system",
		"-o", "jsonpath={.items[0].status.conditions[?(@.type==\"Ready\")].status}")
	if err == nil && result.ExitCode == 0 {
		syncStatus := strings.TrimSpace(result.Stdout)
		if syncStatus == k8sBoolTrue {
			env.Logger.Info("flux: git repository synced")
		} else {
			env.Logger.Warn(fmt.Sprintf("flux: git repository not yet synced (status: %s)", syncStatus))
		}
	}

	return nil
}

// Uninstall removes the flux-operator and flux-instance Helm releases and
// deletes the flux-system namespace. Individual failures are logged but do
// not abort the sequence.
func (f *Flux) Uninstall(ctx context.Context, env *addon.Environment) error {
	env.Logger.Info("flux: removing flux components")
	warnOnErr := func(err error, desc string) {
		if err != nil {
			env.Logger.Warn("flux: "+desc, "err", err)
		}
	}
	_, err := env.Exec.Run(ctx, "helm", "uninstall", "flux-instance", "--namespace", "flux-system")
	warnOnErr(err, "uninstall flux-instance")
	_, err = env.Exec.Run(ctx, "helm", "uninstall", "flux-operator", "--namespace", "flux-system")
	warnOnErr(err, "uninstall flux-operator")
	_, err = env.Exec.Run(ctx, "oc", "delete", "ns", "flux-system")
	warnOnErr(err, "delete flux-system namespace")
	return nil
}

// RequiredTools lists the external binaries flux needs on the host (helm).
func (f *Flux) RequiredTools() []addon.ToolSpec {
	return []addon.ToolSpec{
		{Name: "helm", Description: "Helm package manager for installing Flux charts"},
	}
}

// DefaultSettings returns the built-in defaults for flux's settings map.
func (f *Flux) DefaultSettings() map[string]string {
	return map[string]string{
		SettingProvider:          "flux",
		SettingBranch:            "main",
		SettingPath:              "kubernetes/clusters/production",
		SettingControllerTimeout: "300",
		SettingGitSyncTimeout:    "180",
	}
}

// ValidateSettings checks the flux addon settings map. It requires a Git URL
// and rejects malformed branch or path values.
func (f *Flux) ValidateSettings(settings map[string]string) []string {
	decoded, err := f.DecodeSettings(settings)
	if err != nil {
		return []string{err.Error()}
	}
	fs := decoded.(Settings)
	var errs []string
	if fs.Repository == "" {
		errs = append(errs, "repository is required (set addons.flux.settings.repository)")
	} else if !strings.HasPrefix(fs.Repository, "ssh://") && !strings.HasPrefix(fs.Repository, "https://") &&
		!strings.HasPrefix(fs.Repository, "git://") && !strings.HasPrefix(fs.Repository, "git@") {
		errs = append(errs, "repository must be a valid Git URL (ssh://, https://, git://, or git@)")
	}
	if fs.Branch != "" && strings.ContainsAny(fs.Branch, " \t") {
		errs = append(errs, "branch name cannot contain spaces")
	}
	if fs.Path != "" && !validSyncPath.MatchString(fs.Path) {
		errs = append(errs, "path contains invalid characters (allowed: alphanumeric, /, _, ., -)")
	}
	return errs
}

// WizardFields returns the wizard input fields the flux addon contributes.
func (f *Flux) WizardFields() []addon.WizardField {
	return []addon.WizardField{
		{Key: SettingRepository, Label: "Repository URL", Help: "ssh://git@github.com/org/repo.git", Required: true},
		{Key: SettingBranch, Label: "Branch", Default: "main", Help: "Branch to sync"},
		{Key: SettingPath, Label: "Path", Default: "kubernetes/clusters/production", Help: "Path within repo"},
	}
}

func (f *Flux) waitForControllers(ctx context.Context, env *addon.Environment) error {
	env.Logger.Info("flux: waiting for controllers to become ready")

	timeout := getTimeout(env.AddonConfig.Settings, SettingControllerTimeout, defaultControllerTimeout)

	if err := system.WaitForWithTimeout(ctx, "flux", "controllers", func() bool {
		result, _ := env.Exec.Run(ctx, "oc", "get", "deployments",
			"-n", "flux-system",
			"-l", "app.kubernetes.io/part-of=flux",
			"-o", "jsonpath={range .items[*]}{.metadata.name}={.status.availableReplicas}{\"\\n\"}{end}")
		if result.ExitCode != 0 {
			return false
		}
		lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
		if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
			return false
		}
		for _, line := range lines {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			replicas := strings.TrimSpace(parts[1])
			if replicas == "" || replicas == "0" {
				return false
			}
		}
		return true
	}, timeout, env.Logger); err != nil {
		return fmt.Errorf("flux controllers did not become ready within %v: %w", timeout, err)
	}

	env.Logger.Info("flux: all controllers are running")
	return nil
}

func (f *Flux) waitForGitSync(ctx context.Context, env *addon.Environment) error {
	env.Logger.Info("flux: waiting for git repository sync")

	timeout := getTimeout(env.AddonConfig.Settings, SettingGitSyncTimeout, defaultGitRepoSyncTimeout)

	if err := system.WaitForWithTimeout(ctx, "flux", "git sync", func() bool {
		result, _ := env.Exec.Run(ctx, "oc", "get", "gitrepository",
			"-n", "flux-system",
			"-o", "jsonpath={.items[0].status.conditions[?(@.type==\"Ready\")].status}")
		if result.ExitCode != 0 {
			return false
		}
		return strings.TrimSpace(result.Stdout) == k8sBoolTrue
	}, timeout, env.Logger); err != nil {
		return fmt.Errorf("git repository sync not ready within %v: %w", timeout, err)
	}

	env.Logger.Info("flux: git repository synced successfully")
	return nil
}

func (f *Flux) createDeployKeySecret(ctx context.Context, env *addon.Environment, fs Settings) error {
	repoURL := fs.Repository
	if repoURL == "" {
		return &errtypes.ConfigError{Msg: "flux repository not configured - set addons.flux.settings.repository in config"}
	}
	host, err := gitHost(repoURL)
	if err != nil {
		return fmt.Errorf("failed to resolve git host for ssh-keyscan: %w", err)
	}

	// Resolve the invoking user's home so `ssh-keygen -f ~/.ssh/...` from
	// their shell and the path we read here resolve to the same file, even
	// after the deploy re-execs under sudo.
	homeDir, err := system.InvokingUserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	deployKeyFile := filepath.Join(homeDir, ".ssh", "flux-deploy-key")

	if !system.FileExists(deployKeyFile) {
		return fmt.Errorf("deploy key not found at %s - generate with: ssh-keygen -t ed25519 -f ~/.ssh/flux-deploy-key -N ''", deployKeyFile)
	}

	privateKey, err := os.ReadFile(deployKeyFile)
	if err != nil {
		return fmt.Errorf("failed to read deploy key: %w", err)
	}

	// The public half is optional: flux/source-controller only requires identity
	// and known_hosts. Users who only installed the private key should not fail.
	publicKeyFile := deployKeyFile + ".pub"
	var publicKey []byte
	if b, err := os.ReadFile(publicKeyFile); err == nil {
		publicKey = b
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read deploy key public half: %w", err)
	}

	knownHostsResult, err := env.Exec.RunChecked(ctx, "ssh-keyscan", host)
	if err != nil {
		return fmt.Errorf("failed to get host key for %s: %w", host, err)
	}

	manifest, err := buildFluxDeployKeySecret("flux-system", "flux-system",
		privateKey, publicKey, []byte(knownHostsResult.Stdout))
	if err != nil {
		return fmt.Errorf("build deploy key secret: %w", err)
	}
	_, applyErr := env.Exec.RunWithStdinChecked(ctx, manifest, "oc", "apply", "-f", "-")
	clear(privateKey)
	if applyErr != nil {
		return fmt.Errorf("failed to apply deploy key secret: %w", applyErr)
	}

	env.Logger.Info("flux: deploy key secret applied")
	return nil
}

// gitHost extracts the host portion of a git repository URL. It supports
// ssh://, https://, and the scp-style user@host:path form used by most git
// servers (github, gitlab, gitea, self-hosted).
func gitHost(repoURL string) (string, error) {
	repoURL = strings.TrimSpace(repoURL)
	if repoURL == "" {
		return "", &errtypes.ConfigError{Msg: "empty repository URL"}
	}
	// scp-style: user@host:path (no scheme, has @ and : before any /)
	if !strings.Contains(repoURL, "://") {
		_, rest, ok := strings.Cut(repoURL, "@")
		if !ok {
			return "", fmt.Errorf("cannot parse host from %q", repoURL)
		}
		host, _, ok := strings.Cut(rest, ":")
		if !ok {
			return "", fmt.Errorf("cannot parse host from %q", repoURL)
		}
		return host, nil
	}
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", fmt.Errorf("cannot parse %q: %w", repoURL, err)
	}
	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("%q has no host component", repoURL)
	}
	return host, nil
}

// buildFluxDeployKeySecret renders a Secret manifest containing the SSH deploy
// key material. publicKey is optional — flux only requires identity and
// known_hosts, so the identity.pub field is omitted when empty. Inputs are
// []byte so callers can clear the private key buffer after use.
func buildFluxDeployKeySecret(namespace, name string, privateKey, publicKey, knownHosts []byte) (string, error) {
	data := map[string][]byte{
		"identity":    privateKey,
		"known_hosts": knownHosts,
	}
	if len(publicKey) != 0 {
		data["identity.pub"] = publicKey
	}
	return addon.BuildOpaqueSecret(namespace, name, data)
}

// getTimeout reads a timeout setting (in seconds) from the settings map,
// falling back to the given default.
func getTimeout(settings map[string]string, key string, defaultTimeout time.Duration) time.Duration {
	if v, ok := settings[key]; ok && v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return defaultTimeout
}
