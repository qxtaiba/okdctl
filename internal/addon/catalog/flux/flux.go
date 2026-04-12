// Package flux provides the FluxCD GitOps addon, which bootstraps Flux
// controllers into an OKD cluster and registers them via the addon system's
// init()-based catalog.
package flux

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/utils/system"
)

const (
	defaultControllerTimeout  = 5 * time.Minute
	defaultGitRepoSyncTimeout = 3 * time.Minute
)

// validSyncPath matches safe sync paths: alphanumeric, slashes, underscores, dots, and hyphens.
var validSyncPath = regexp.MustCompile(`^[a-zA-Z0-9/_.\-]+$`)

func init() {
	if err := addon.Register(&Flux{}); err != nil {
		panic(err)
	}
}

type Flux struct{}

func (f *Flux) Info() addon.AddonInfo {
	return addon.AddonInfo{
		Name:           "flux",
		DisplayName:    "Flux GitOps",
		Description:    "Continuous delivery using Flux GitOps controller",
		Category:       "gitops",
		Dependencies:   nil,
		Priority:       80,
		DefaultEnabled: false,
	}
}

func (f *Flux) Install(ctx context.Context, env *addon.Environment) error {
	if !executor.CommandExists("helm") {
		return fmt.Errorf("helm is required to install Flux")
	}

	if err := addon.EnsureNamespace(ctx, env, "flux-system"); err != nil {
		return err
	}

	if err := addon.RetryDefault(ctx, func() error {
		return f.createDeployKeySecret(ctx, env)
	}); err != nil {
		return fmt.Errorf("deploy key secret required: %w", err)
	}

	if err := f.installOperator(ctx, env); err != nil {
		return err
	}

	if err := f.installInstance(ctx, env); err != nil {
		return err
	}

	// Wait for Flux controllers to become available (fatal if they don't start)
	if err := f.waitForControllers(ctx, env); err != nil {
		return err
	}

	// Wait for GitRepository sync (non-fatal — user may need to fix deploy key or URL)
	if err := f.waitForGitSync(ctx, env); err != nil {
		env.Logger.Warn(fmt.Sprintf("flux: git sync not ready: %v", err))
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

func (f *Flux) installInstance(ctx context.Context, env *addon.Environment) error {
	env.Logger.Info("flux: installing instance for gitops sync")
	settings := env.AddonConfig.Settings
	syncURL := settings["repository"]
	if syncURL == "" {
		return fmt.Errorf("flux repository not configured - set addons.flux.settings.repository in config")
	}
	branch := settings["branch"]
	if branch == "" {
		branch = "main"
	}
	syncPath := settings["path"]
	if syncPath == "" {
		syncPath = "kubernetes/clusters/production"
	}

	return f.helmUpgradeInstall(ctx, env, "flux-instance",
		"oci://ghcr.io/controlplaneio-fluxcd/charts/flux-instance",
		"flux instance",
		"--set", "instance.cluster.type=openshift",
		"--set", fmt.Sprintf("instance.sync.url=%s", syncURL),
		"--set", fmt.Sprintf("instance.sync.ref=refs/heads/%s", branch),
		"--set", fmt.Sprintf("instance.sync.path=%s", syncPath),
		"--set", "instance.sync.pullSecret=flux-system",
	)
}

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
	if err != nil || result == nil || result.ExitCode != 0 {
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
	if err == nil && result != nil && result.ExitCode == 0 {
		syncStatus := strings.TrimSpace(result.Stdout)
		if syncStatus == "True" {
			env.Logger.Info("flux: git repository synced")
		} else {
			env.Logger.Warn(fmt.Sprintf("flux: git repository not yet synced (status: %s)", syncStatus))
		}
	}

	return nil
}

func (f *Flux) Uninstall(ctx context.Context, env *addon.Environment) error {
	env.Logger.Info("flux: removing flux components")
	warnOnErr := func(err error, desc string) {
		if err != nil {
			env.Logger.Warn(fmt.Sprintf("flux: %s: %v", desc, err))
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

func (f *Flux) RequiredTools() []addon.ToolSpec {
	return []addon.ToolSpec{
		{Name: "helm", Description: "Helm package manager for installing Flux charts"},
	}
}

func (f *Flux) DefaultSettings() map[string]string {
	return map[string]string{
		"provider":           "flux",
		"branch":             "main",
		"path":               "kubernetes/clusters/production",
		"controller_timeout": "300",
		"git_sync_timeout":   "180",
	}
}

func (f *Flux) ValidateSettings(settings map[string]string) []string {
	var errs []string
	repo := settings["repository"]
	if repo == "" {
		errs = append(errs, "repository is required (set addons.flux.settings.repository)")
	} else {
		if !strings.HasPrefix(repo, "ssh://") && !strings.HasPrefix(repo, "https://") &&
			!strings.HasPrefix(repo, "git://") && !strings.HasPrefix(repo, "git@") {
			errs = append(errs, "repository must be a valid Git URL (ssh://, https://, git://, or git@)")
		}
		errs = append(errs, validateSSHRepoURL(repo)...)
	}
	if branch := settings["branch"]; branch != "" && strings.ContainsAny(branch, " \t") {
		errs = append(errs, "branch name cannot contain spaces")
	}
	if p := settings["path"]; p != "" && !validSyncPath.MatchString(p) {
		errs = append(errs, "path contains invalid characters (allowed: alphanumeric, /, _, ., -)")
	}
	return errs
}

func (f *Flux) WizardFields() []addon.WizardField {
	return []addon.WizardField{
		{Key: "repository", Label: "Repository URL", Help: "ssh://git@github.com/org/repo.git", Required: true},
		{Key: "branch", Label: "Branch", Default: "main", Help: "Branch to sync"},
		{Key: "path", Label: "Path", Default: "kubernetes/clusters/production", Help: "Path within repo"},
	}
}

func (f *Flux) waitForControllers(ctx context.Context, env *addon.Environment) error {
	env.Logger.Info("flux: waiting for controllers to become ready")

	timeout := getTimeout(env.AddonConfig.Settings, "controller_timeout", defaultControllerTimeout)

	if err := system.WaitForWithTimeout(ctx, "flux", "controllers", func() bool {
		result, _ := env.Exec.Run(ctx, "oc", "get", "deployments",
			"-n", "flux-system",
			"-l", "app.kubernetes.io/part-of=flux",
			"-o", "jsonpath={range .items[*]}{.metadata.name}={.status.availableReplicas}{\"\\n\"}{end}")
		if result == nil || result.ExitCode != 0 {
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

	timeout := getTimeout(env.AddonConfig.Settings, "git_sync_timeout", defaultGitRepoSyncTimeout)

	if err := system.WaitForWithTimeout(ctx, "flux", "git sync", func() bool {
		result, _ := env.Exec.Run(ctx, "oc", "get", "gitrepository",
			"-n", "flux-system",
			"-o", "jsonpath={.items[0].status.conditions[?(@.type==\"Ready\")].status}")
		if result == nil || result.ExitCode != 0 {
			return false
		}
		return strings.TrimSpace(result.Stdout) == "True"
	}, timeout, env.Logger); err != nil {
		return fmt.Errorf("git repository sync not ready within %v", timeout)
	}

	env.Logger.Info("flux: git repository synced successfully")
	return nil
}

func (f *Flux) createDeployKeySecret(ctx context.Context, env *addon.Environment) error {
	repoURL := env.AddonConfig.Settings["repository"]
	if repoURL == "" {
		return fmt.Errorf("flux repository not configured - set addons.flux.settings.repository in config")
	}
	host, err := gitHost(repoURL)
	if err != nil {
		return fmt.Errorf("failed to resolve git host for ssh-keyscan: %w", err)
	}

	homeDir, err := os.UserHomeDir()
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

	manifest := buildFluxDeployKeySecret("flux-system", "flux-system",
		string(privateKey), string(publicKey), knownHostsResult.Stdout)
	if _, err := env.Exec.RunWithStdinChecked(ctx, manifest, "oc", "apply", "-f", "-"); err != nil {
		return fmt.Errorf("failed to apply deploy key secret: %w", err)
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
		return "", fmt.Errorf("empty repository URL")
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
// known_hosts, so the identity.pub field is omitted when empty.
func buildFluxDeployKeySecret(namespace, name, privateKey, publicKey, knownHosts string) string {
	enc := base64.StdEncoding.EncodeToString
	data := map[string]string{
		"identity":    enc([]byte(privateKey)),
		"known_hosts": enc([]byte(knownHosts)),
	}
	if publicKey != "" {
		data["identity.pub"] = enc([]byte(publicKey))
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

func validateSSHRepoURL(repo string) []string {
	if !strings.HasPrefix(repo, "ssh://") {
		return nil
	}
	const scpErr = "ssh:// URLs must use slashes for path (ssh://git@github.com/org/repo.git), not colons (ssh://git@github.com:org/repo.git)"
	afterScheme := strings.TrimPrefix(repo, "ssh://")
	slashIdx := strings.Index(afterScheme, "/")
	if slashIdx <= 0 {
		if strings.Contains(afterScheme, ":") {
			return []string{scpErr}
		}
		return nil
	}
	host := afterScheme[:slashIdx]
	if strings.Contains(host, ":") && !strings.Contains(host, "]:") {
		colonIdx := strings.LastIndex(host, ":")
		if !isNumeric(host[colonIdx+1:]) {
			return []string{scpErr}
		}
	}
	return nil
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
