// Package flux provides the FluxCD GitOps addon, which bootstraps Flux
// controllers into an OKD cluster and registers them via the addon system's
// init()-based catalog.
package flux

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/system"
)

const (
	defaultControllerTimeout  = 5 * time.Minute
	defaultGitRepoSyncTimeout = 3 * time.Minute
)

// ProviderID is the canonical addon identity string for FluxCD. Wizard
// defaults and addon DefaultSettings both reference this constant so the
// two cannot silently diverge.
const ProviderID = "flux"

// Settings keys consumed by the Flux addon. Named here so callers (install
// wizard, validators, gitops bootstrap) reference the same string.
const (
	SettingRepository         = "repository"
	SettingBranch             = "branch"
	SettingPath               = "path"
	SettingProvider           = "provider"
	SettingControllerTimeout  = "controller_timeout"
	SettingGitSyncTimeout     = "git_sync_timeout"
	SettingGitHostFingerprint = "git_host_fingerprint"
	// SettingAcceptHostKey opts into TOFU when no git_host_fingerprint pin is
	// set. Set to "true" only after reviewing the observed fingerprints logged
	// at WARN on the first unauthenticated run.
	SettingAcceptHostKey = "accept_host_key"
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
		return &errtypes.ConfigError{Msg: "flux: invalid settings", Err: err}
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

	if err := f.waitForControllers(ctx, env); err != nil {
		return err
	}

	// Wait for GitRepository sync (non-fatal — user may need to fix deploy key or URL)
	if err := f.waitForGitSync(ctx, env); err != nil {
		env.Logger.Warn("flux: git sync not ready", "err", err)
		env.Logger.Info("flux: debug with: oc get gitrepository -n flux-system -o yaml")
		env.Logger.Info("flux: the cluster will auto-reconcile once the git source is reachable")
	}

	env.Logger.Info("flux: gitops installed and syncing with repository")
	return nil
}

// helmUpgradeInstall wraps the shared "helm upgrade --install ... --wait"
// invocation used for both flux-operator and flux-instance. extraArgs is
// appended after the common --namespace flag (so callers can pass -f
// <values-file> or --create-namespace).
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
	valuesYAML, err := buildInstanceValues(fs)
	if err != nil {
		return fmt.Errorf("flux: build instance values: %w", err)
	}
	valuesPath, err := system.WriteTempFile("okdctl-flux-instance-*.yaml", 0o600, func(f *os.File) error {
		_, writeErr := f.Write(valuesYAML)
		return writeErr
	})
	if err != nil {
		return fmt.Errorf("flux: write instance values file: %w", err)
	}
	defer func() { _ = os.Remove(valuesPath) }()
	return f.helmUpgradeInstall(ctx, env, "flux-instance",
		"oci://ghcr.io/controlplaneio-fluxcd/charts/flux-instance",
		"flux instance",
		"-f", valuesPath,
	)
}

// buildInstanceValues marshals the flux-instance helm chart values from
// Settings into YAML bytes. Keeping values in a file rather than --set argv
// prevents repository URLs from appearing in /proc/<pid>/cmdline and in helm
// release Secrets.
func buildInstanceValues(fs Settings) ([]byte, error) {
	v := map[string]any{
		"instance": map[string]any{
			"cluster": map[string]any{
				"type": "openshift",
			},
			"sync": map[string]any{
				"url":        fs.Repository,
				"ref":        "refs/heads/" + fs.Branch,
				"path":       fs.Path,
				"pullSecret": "flux-system",
			},
		},
	}
	return yaml.Marshal(v)
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
	env.Logger.Info("flux: operator ready", "replicas", ready)

	result, err = env.Exec.Run(ctx, "oc", "get", "deployment", "source-controller",
		"-n", "flux-system", "-o", "jsonpath={.status.readyReplicas}")
	if err != nil || result.ExitCode != 0 {
		env.Logger.Warn("flux: cannot query source-controller deployment; verification skipped")
	} else {
		scReady := strings.TrimSpace(result.Stdout)
		if scReady == "" || scReady == "0" {
			return fmt.Errorf("source-controller has no ready replicas")
		}
		env.Logger.Info("flux: source-controller ready", "replicas", scReady)
	}

	// Check GitRepository sync status (informational, not fatal for verify)
	result, err = env.Exec.Run(ctx, "oc", "get", "gitrepository", "-n", "flux-system",
		"-o", "jsonpath={.items[0].status.conditions[?(@.type==\"Ready\")].status}")
	if err == nil && result.ExitCode == 0 {
		syncStatus := strings.TrimSpace(result.Stdout)
		if syncStatus == k8sBoolTrue {
			env.Logger.Info("flux: git repository synced")
		} else {
			env.Logger.Warn("flux: git repository not yet synced", "status", syncStatus)
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
		SettingProvider:          ProviderID,
		SettingBranch:            "main",
		SettingPath:              "kubernetes/clusters/production",
		SettingControllerTimeout: "300",
		SettingGitSyncTimeout:    "180",
	}
}

// ValidateSettings checks the flux addon settings map. It requires a Git URL,
// rejects malformed branch or path values, and rejects URLs with embedded
// userinfo (https://user:token@host) — helm --set arguments end up in
// /proc/<pid>/cmdline, so SSH-key auth via a deploy-key Secret is the only
// supported credential channel.
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
	} else if u, parseErr := url.Parse(fs.Repository); parseErr == nil &&
		(u.Scheme == "http" || u.Scheme == "https") && u.User != nil {
		errs = append(errs, "repository URL must not contain credentials; use SSH deploy-key auth instead")
	} else if _, hostErr := gitHost(fs.Repository); hostErr != nil {
		errs = append(errs, fmt.Sprintf("repository URL has an invalid host: %v", hostErr))
	}
	if fs.Branch != "" && strings.ContainsAny(fs.Branch, " \t") {
		errs = append(errs, "branch name cannot contain spaces")
	}
	if fs.Path != "" && !validSyncPath.MatchString(fs.Path) {
		errs = append(errs, "path contains invalid characters (allowed: alphanumeric, /, _, ., -)")
	}
	if fp := fs.GitHostFingerprint; fp != "" && (!strings.HasPrefix(fp, "SHA256:") || len(fp) <= len("SHA256:")) {
		errs = append(errs, "git_host_fingerprint must be in SHA256:<base64> format (from ssh-keygen -lf)")
	}
	return errs
}

// WizardFields returns the wizard input fields the flux addon contributes.
func (f *Flux) WizardFields() []addon.WizardField {
	return []addon.WizardField{
		{Key: SettingRepository, Label: "Repository URL", Help: "ssh://git@github.com/org/repo.git (SSH deploy-key auth only; no https://user:token@ URLs)", Required: true},
		{Key: SettingBranch, Label: "Branch", Default: "main", Help: "Branch to sync"},
		{Key: SettingPath, Label: "Path", Default: "kubernetes/clusters/production", Help: "Path within repo"},
	}
}

func (f *Flux) waitForControllers(ctx context.Context, env *addon.Environment) error {
	env.Logger.Info("flux: waiting for controllers to become ready")

	timeout := getTimeout(env.AddonConfig.Settings, SettingControllerTimeout, defaultControllerTimeout)

	if err := system.WaitForWithTimeout(ctx, "flux", "controllers", func(context.Context) bool {
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

	if err := system.WaitForWithTimeout(ctx, "flux", "git sync", func(context.Context) bool {
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

	switch info, err := os.Lstat(deployKeyFile); {
	case err != nil && os.IsNotExist(err):
		return fmt.Errorf("deploy key not found at %s - generate with: ssh-keygen -t ed25519 -f ~/.ssh/flux-deploy-key -N ''", deployKeyFile)
	case err != nil:
		return fmt.Errorf("failed to stat deploy key: %w", err)
	case info.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("deploy key %s is a symlink; refusing to follow", deployKeyFile)
	}

	privateKey, err := readKeyFile(deployKeyFile)
	if err != nil {
		return fmt.Errorf("failed to read deploy key: %w", err)
	}

	// The public half is optional: flux/source-controller only requires identity
	// and known_hosts. Users who only installed the private key should not fail.
	publicKeyFile := deployKeyFile + ".pub"
	var publicKey []byte
	if b, err := readKeyFile(publicKeyFile); err == nil {
		publicKey = b
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read deploy key public half: %w", err)
	}

	knownHostsResult, err := env.Exec.RunChecked(ctx, "ssh-keyscan", host)
	if err != nil {
		return fmt.Errorf("failed to get host key for %s: %w", host, err)
	}

	if err := verifyKeyscanFingerprint(knownHostsResult.Stdout, host, fs.GitHostFingerprint, fs.AcceptHostKey, env.Logger); err != nil {
		return fmt.Errorf("flux: host key verification failed: %w", err)
	}

	manifest, err := buildFluxDeployKeySecret("flux-system", "flux-system",
		privateKey, publicKey, filterKeyscanLines(knownHostsResult.Stdout))
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

// verifyKeyscanFingerprint validates the output of ssh-keyscan against an
// operator-configured SHA256 pin.
//
// When expected is non-empty: each parsed key's fingerprint is compared; a
// match returns nil; no match returns an error naming expected and observed.
//
// When expected is empty and acceptHostKey is true: observed fingerprints
// log at WARN and nil is returned (TOFU).
//
// When expected is empty and acceptHostKey is false: returns an error listing
// observed fingerprints — fail closed without TOFU.
func verifyKeyscanFingerprint(keyscanOut, host, expected string, acceptHostKey bool, log *slog.Logger) error {
	var observed []string
	sc := bufio.NewScanner(strings.NewReader(keyscanOut))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
		if err != nil {
			continue
		}
		fp := ssh.FingerprintSHA256(key)
		if expected != "" && fp == expected {
			return nil
		}
		observed = append(observed, fp)
	}
	if expected == "" {
		if !acceptHostKey {
			return fmt.Errorf("git host fingerprint not pinned for %s — set addons.flux.settings.git_host_fingerprint to one of the observed values [%s], or set accept_host_key=true to opt into TOFU",
				host, strings.Join(observed, ", "))
		}
		log.Warn("flux: git host fingerprint not pinned — set addons.flux.settings.git_host_fingerprint to pin",
			"host", host, "observed_fingerprints", strings.Join(observed, ", "))
		return nil
	}
	return fmt.Errorf("git host key mismatch for %s: expected %s, observed [%s]",
		host, expected, strings.Join(observed, ", "))
}

// filterKeyscanLines strips comment and blank lines from ssh-keyscan output,
// returning only the host-key lines. The Flux Secret stays byte-stable across
// keyscan runs whose banner-line ordering or comment content may vary.
func filterKeyscanLines(keyscanOut string) []byte {
	var b strings.Builder
	sc := bufio.NewScanner(strings.NewReader(keyscanOut))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return []byte(b.String())
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
		if host == "" || strings.HasPrefix(host, "-") {
			return "", fmt.Errorf("repository host %q is invalid", host)
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
	if strings.HasPrefix(host, "-") {
		return "", fmt.Errorf("repository host %q is invalid", host)
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
// readKeyFile reads path while refusing to follow a symlink at the final
// component. Mirrors the lstat-then-O_NOFOLLOW pattern in runlock.Acquire.
func readKeyFile(path string) ([]byte, error) {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s is a symlink; refusing to follow", path)
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func getTimeout(settings map[string]string, key string, defaultTimeout time.Duration) time.Duration {
	if v, ok := settings[key]; ok && v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return defaultTimeout
}
