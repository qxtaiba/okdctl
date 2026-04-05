package flux

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/addon"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/retry"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

const (
	defaultControllerTimeout  = 5 * time.Minute
	defaultGitRepoSyncTimeout = 3 * time.Minute
)

func init() {
	addon.Register(&Flux{})
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

	// Create namespace with retry (transient API errors during cluster bootstrap)
	if err := retry.Do(ctx, 3, 5*time.Second, func() error {
		return addon.EnsureNamespace(ctx, env, "flux-system")
	}); err != nil {
		return err
	}

	if err := retry.Do(ctx, 3, 5*time.Second, func() error {
		return f.createDeployKeySecret(ctx, env)
	}); err != nil {
		return utils.WrapError("deploy key secret required", err)
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

func (f *Flux) installOperator(ctx context.Context, env *addon.Environment) error {
	env.Logger.Info("flux: installing operator via helm")
	if err := retry.Do(ctx, 3, 5*time.Second, func() error {
		result, err := env.Exec.Run(ctx, "helm", "upgrade", "--install", "flux-operator",
			"oci://ghcr.io/controlplaneio-fluxcd/charts/flux-operator",
			"--namespace", "flux-system",
			"--create-namespace",
			"--wait")
		if err != nil {
			return utils.WrapError("failed to install flux operator", err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("failed to install flux operator: %s", result.Stderr)
		}
		return nil
	}); err != nil {
		return err
	}

	result, err := env.Exec.Run(ctx, "oc", "wait", "--for=condition=available", "deployment/flux-operator",
		"--namespace", "flux-system", "--timeout=120s")
	if err != nil || result == nil || result.ExitCode != 0 {
		return fmt.Errorf("flux: operator not ready within 120s timeout")
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
	syncRef := "refs/heads/" + branch
	syncPath := settings["path"]
	if syncPath == "" {
		syncPath = "kubernetes/clusters/production"
	}

	if err := retry.Do(ctx, 3, 5*time.Second, func() error {
		r, err := env.Exec.Run(ctx, "helm", "upgrade", "--install", "flux-instance",
			"oci://ghcr.io/controlplaneio-fluxcd/charts/flux-instance",
			"--namespace", "flux-system",
			"--set", "instance.cluster.type=openshift",
			"--set", fmt.Sprintf("instance.sync.url=%s", syncURL),
			"--set", fmt.Sprintf("instance.sync.ref=%s", syncRef),
			"--set", fmt.Sprintf("instance.sync.path=%s", syncPath),
			"--set", "instance.sync.pullSecret=flux-system",
			"--wait")
		if err != nil {
			return utils.WrapError("failed to install flux instance", err)
		}
		if r.ExitCode != 0 {
			return fmt.Errorf("failed to install flux instance: %s", r.Stderr)
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (f *Flux) Verify(ctx context.Context, env *addon.Environment) error {
	result, err := env.Exec.Run(ctx, "oc", "get", "deployment", "flux-operator",
		"-n", "flux-system", "-o", "jsonpath={.status.readyReplicas}")
	if err != nil || result == nil || result.ExitCode != 0 {
		return fmt.Errorf("flux-operator deployment not found")
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
			env.Logger.Warn("flux: git repository not yet synced (status: " + syncStatus + ")")
		}
	}

	return nil
}

func (f *Flux) Uninstall(ctx context.Context, env *addon.Environment) error {
	env.Logger.Info("flux: removing flux components")
	_, _ = env.Exec.Run(ctx, "helm", "uninstall", "flux-instance", "--namespace", "flux-system")
	_, _ = env.Exec.Run(ctx, "helm", "uninstall", "flux-operator", "--namespace", "flux-system")
	_, _ = env.Exec.Run(ctx, "oc", "delete", "ns", "flux-system")
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
	if repo := settings["repository"]; repo != "" {
		if !strings.HasPrefix(repo, "ssh://") && !strings.HasPrefix(repo, "https://") &&
			!strings.HasPrefix(repo, "git://") && !strings.HasPrefix(repo, "git@") {
			errs = append(errs, "repository must be a valid Git URL (ssh://, https://, git://, or git@)")
		}
		// Reject mixed ssh://...host:org/repo format (must use slashes after scheme)
		if strings.HasPrefix(repo, "ssh://") {
			// Strip scheme, then check if host portion contains a colon (SCP-style path)
			afterScheme := strings.TrimPrefix(repo, "ssh://")
			if slashIdx := strings.Index(afterScheme, "/"); slashIdx > 0 {
				host := afterScheme[:slashIdx]
				// A colon in the host part (e.g., "git@github.com:org") means SCP-style mixed with scheme
				if strings.Contains(host, ":") && !strings.Contains(host, "]:") {
					errs = append(errs, "ssh:// URLs must use slashes for path (ssh://git@github.com/org/repo.git), not colons (ssh://git@github.com:org/repo.git)")
				}
			} else if strings.Contains(afterScheme, ":") {
				// No slash at all but has colon — e.g., ssh://git@github.com:org/repo
				errs = append(errs, "ssh:// URLs must use slashes for path (ssh://git@github.com/org/repo.git), not colons (ssh://git@github.com:org/repo.git)")
			}
		}
	}
	if branch := settings["branch"]; branch != "" && strings.ContainsAny(branch, " \t") {
		errs = append(errs, "branch name cannot contain spaces")
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
		if ctx.Err() != nil {
			return false
		}
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
		if ctx.Err() != nil {
			return false
		}
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
		return utils.WrapError("failed to resolve git host for ssh-keyscan", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return utils.WrapError("failed to get home directory", err)
	}
	deployKeyFile := filepath.Join(homeDir, ".ssh", "flux-deploy-key")

	if !system.FileExists(deployKeyFile) {
		return fmt.Errorf("deploy key not found at %s - generate with: ssh-keygen -t ed25519 -f ~/.ssh/flux-deploy-key -N ''", deployKeyFile)
	}

	privateKey, err := os.ReadFile(deployKeyFile)
	if err != nil {
		return utils.WrapError("failed to read deploy key", err)
	}

	// The public half is optional: flux/source-controller only requires identity
	// and known_hosts. Users who only installed the private key should not fail.
	publicKeyFile := deployKeyFile + ".pub"
	var publicKey []byte
	if b, err := os.ReadFile(publicKeyFile); err == nil {
		publicKey = b
	} else if !os.IsNotExist(err) {
		return utils.WrapError("failed to read deploy key public half", err)
	}

	knownHostsResult, err := env.Exec.Run(ctx, "ssh-keyscan", host)
	if err != nil || knownHostsResult.ExitCode != 0 {
		return fmt.Errorf("failed to get host key for %s", host)
	}

	manifest := buildFluxDeployKeySecret("flux-system", "flux-system",
		string(privateKey), string(publicKey), knownHostsResult.Stdout)
	result, err := env.Exec.RunWithStdin(ctx, manifest, "oc", "apply", "-f", "-")
	if err != nil {
		return utils.WrapError("failed to apply deploy key secret", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to apply deploy key secret: %s", result.Stderr)
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
		at := strings.Index(repoURL, "@")
		if at < 0 {
			return "", fmt.Errorf("cannot parse host from %q", repoURL)
		}
		rest := repoURL[at+1:]
		colon := strings.Index(rest, ":")
		if colon < 0 {
			return "", fmt.Errorf("cannot parse host from %q", repoURL)
		}
		return rest[:colon], nil
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
// key material. Values are base64-encoded into the data map so oc apply can
// pipe the manifest via stdin without exposing key bytes on argv. publicKey is
// optional — flux only requires identity and known_hosts, so the identity.pub
// field is omitted when empty.
func buildFluxDeployKeySecret(namespace, name, privateKey, publicKey, knownHosts string) string {
	enc := base64.StdEncoding.EncodeToString
	dataLines := []string{
		fmt.Sprintf("  identity: %s", enc([]byte(privateKey))),
	}
	if publicKey != "" {
		dataLines = append(dataLines, fmt.Sprintf("  identity.pub: %s", enc([]byte(publicKey))))
	}
	dataLines = append(dataLines, fmt.Sprintf("  known_hosts: %s", enc([]byte(knownHosts))))
	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
data:
%s
`, name, namespace, strings.Join(dataLines, "\n"))
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
