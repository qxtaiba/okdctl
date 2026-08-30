// Package flux implements the FluxCD GitOps addon, bootstrapping Flux
// controllers into an OKD cluster.
package flux

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

const (
	defaultControllerTimeout  = 5 * time.Minute
	defaultGitRepoSyncTimeout = 3 * time.Minute
)

const (
	defaultBranch = "main"
	defaultPath   = "kubernetes/clusters/production"
)

// ProviderID is the addon identity string for FluxCD.
const ProviderID = "flux"

// Settings keys for the flux addon.
const (
	SettingRepository         = "repository"
	SettingBranch             = "branch"
	SettingPath               = "path"
	SettingProvider           = "provider"
	SettingControllerTimeout  = "controller_timeout"
	SettingGitSyncTimeout     = "git_sync_timeout"
	SettingGitHostFingerprint = "git_host_fingerprint"
	// SettingAcceptHostKey opts into TOFU when git_host_fingerprint is unset.
	SettingAcceptHostKey = "accept_host_key"
)

const k8sBoolTrue = string(nodetypes.ConditionStatusTrue)

var validSyncPath = regexp.MustCompile(`^[a-zA-Z0-9/_.\-]+$`)

func init() {
	if err := addon.Register(&fluxAddon{}); err != nil {
		panic(err)
	}
}

type fluxAddon struct{}

func (f *fluxAddon) Info() addon.Metadata {
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

func (f *fluxAddon) Install(ctx context.Context, env *addon.Environment) error {
	if !executor.CommandExists("helm") {
		return &errtypes.ConfigError{Msg: "helm is required to install Flux"}
	}

	fs, err := f.decodeSettings(env.AddonConfig.Settings)
	if err != nil {
		return &errtypes.ConfigError{Msg: "flux: invalid settings", Err: err}
	}

	if err := addon.EnsureNamespace(ctx, env, "flux-system"); err != nil {
		return err
	}

	if err := addon.RetryDefault(ctx, func() error {
		return f.createDeployKeySecret(ctx, env, &fs)
	}); err != nil {
		return fmt.Errorf("deploy key secret required: %w", err)
	}

	if err := f.installOperator(ctx, env); err != nil {
		return err
	}

	if err := f.installInstance(ctx, env, &fs); err != nil {
		return err
	}

	if err := f.waitForControllers(ctx, env, &fs); err != nil {
		return err
	}

	// Non-fatal: git sync failures are usually a bad deploy key or repo URL.
	if err := f.waitForGitSync(ctx, env, &fs); err != nil {
		env.Logger.Warn("flux: git sync not ready", "err", err)
		env.Logger.Info("flux: debug with: oc get gitrepository -n flux-system -o yaml")
		env.Logger.Info("flux: the cluster will auto-reconcile once the git source is reachable")
	}

	env.Logger.Info("flux: gitops installed and syncing with repository")
	return nil
}

// helmUpgradeInstall runs "helm upgrade --install ... --wait"; extraArgs land
// between --namespace and --wait.
func (f *fluxAddon) helmUpgradeInstall(ctx context.Context, env *addon.Environment, release, chart, errLabel string, extraArgs ...string) error {
	return addon.RetryDefault(ctx, func() error {
		args := []string{"upgrade", "--install", release, chart, "--namespace", "flux-system"}
		args = append(args, extraArgs...)
		args = append(args, "--wait")
		if _, err := env.Exec.RunChecked(ctx, "helm", args...); err != nil {
			return fmt.Errorf("install %s: %w", errLabel, err)
		}
		return nil
	})
}

func (f *fluxAddon) installOperator(ctx context.Context, env *addon.Environment) error {
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

func (f *fluxAddon) installInstance(ctx context.Context, env *addon.Environment, fs *Settings) error {
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
	return f.helmUpgradeInstall(
		ctx, env, "flux-instance",
		"oci://ghcr.io/controlplaneio-fluxcd/charts/flux-instance",
		"flux instance",
		"-f", valuesPath,
	)
}

// buildInstanceValues marshals flux-instance's helm chart values to YAML; a
// file (not --set argv) keeps repo URLs out of /proc/<pid>/cmdline.
func buildInstanceValues(fs *Settings) ([]byte, error) {
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

// Verify checks flux-operator/source-controller readiness; GitRepository sync status is non-fatal.
func (f *fluxAddon) Verify(ctx context.Context, env *addon.Environment) error {
	result, err := env.Exec.RunChecked(ctx, "oc", "get", "deployment", "flux-operator",
		"-n", "flux-system", "-o", "jsonpath={.status.readyReplicas}")
	if err != nil {
		return fmt.Errorf("flux-operator deployment not found: %w", err)
	}
	ready := strings.TrimSpace(result.Stdout)
	if ready == "" || ready == "0" {
		return errors.New("flux-operator has no ready replicas")
	}
	env.Logger.Info("flux: operator ready", "replicas", ready)

	result, err = env.Exec.Run(ctx, "oc", "get", "deployment", "source-controller",
		"-n", "flux-system", "-o", "jsonpath={.status.readyReplicas}")
	if err != nil || result.ExitCode != 0 {
		env.Logger.Warn("flux: cannot query source-controller deployment; verification skipped")
	} else {
		scReady := strings.TrimSpace(result.Stdout)
		if scReady == "" || scReady == "0" {
			return errors.New("source-controller has no ready replicas")
		}
		env.Logger.Info("flux: source-controller ready", "replicas", scReady)
	}

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

// Uninstall removes flux's Helm releases and namespace; step failures are logged, not aborted.
func (f *fluxAddon) Uninstall(ctx context.Context, env *addon.Environment) error {
	env.Logger.Info("flux: removing flux components")
	// Run returns nil on a non-zero exit; the exit code must be checked explicitly.
	warnOnErr := func(res *executor.Result, err error, desc string) {
		if err != nil || res.ExitCode != 0 {
			env.Logger.Warn("flux: uninstall step failed", "step", desc, "exit", res.ExitCode, "err", err)
		}
	}
	res, err := env.Exec.Run(ctx, "helm", "uninstall", "flux-instance", "--namespace", "flux-system")
	warnOnErr(res, err, "uninstall flux-instance")
	res, err = env.Exec.Run(ctx, "helm", "uninstall", "flux-operator", "--namespace", "flux-system")
	warnOnErr(res, err, "uninstall flux-operator")
	res, err = env.Exec.Run(ctx, "oc", "delete", "ns", "flux-system")
	warnOnErr(res, err, "delete flux-system namespace")
	return nil
}

func (f *fluxAddon) RequiredTools() []addon.ToolSpec {
	return []addon.ToolSpec{
		{Name: "helm", Description: "Helm package manager for installing Flux charts"},
	}
}

func (f *fluxAddon) DefaultSettings() map[string]string {
	return map[string]string{
		SettingProvider:          ProviderID,
		SettingBranch:            defaultBranch,
		SettingPath:              defaultPath,
		SettingControllerTimeout: "300",
		SettingGitSyncTimeout:    "180",
	}
}

// ValidateSettings requires a Git URL and rejects embedded userinfo (https://user:token@host).
// SSH deploy-key auth is the only supported credential channel.
func (f *fluxAddon) ValidateSettings(settings map[string]string) []string {
	fs, err := f.decodeSettings(settings)
	if err != nil {
		return []string{err.Error()}
	}
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

func (f *fluxAddon) waitForControllers(ctx context.Context, env *addon.Environment, fs *Settings) error {
	env.Logger.Info("flux: waiting for controllers to become ready")

	timeout := fs.ControllerTimeout

	if err := system.WaitForWithTimeout(ctx, "flux", "controllers", func(pctx context.Context) bool {
		// pctx is WaitFor's poll-deadline ctx, bounding a hung oc probe instead of the outer ctx.
		result, err := env.Exec.RunOutput(pctx, 0, "oc", "get", "deployments",
			"-n", "flux-system",
			"-l", "app.kubernetes.io/part-of=flux",
			"-o", "jsonpath={range .items[*]}{.metadata.name}={.status.availableReplicas}{\"\\n\"}{end}")
		if err != nil || result.ExitCode != 0 || result.Truncated {
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

func (f *fluxAddon) waitForGitSync(ctx context.Context, env *addon.Environment, fs *Settings) error {
	env.Logger.Info("flux: waiting for git repository sync")

	timeout := fs.GitSyncTimeout

	var lastProbeErr string
	if err := system.WaitForWithTimeout(ctx, "flux", "git sync", func(pctx context.Context) bool {
		result, err := env.Exec.Run(pctx, "oc", "get", "gitrepository",
			"-n", "flux-system",
			"-o", "jsonpath={.items[0].status.conditions[?(@.type==\"Ready\")].status}")
		if err != nil {
			// A transport failure yields err with ExitCode 0; log once, then treat as not-ready.
			if msg := err.Error(); msg != lastProbeErr {
				env.Logger.Warn("flux: git sync probe failed", "err", err)
				lastProbeErr = msg
			} else {
				env.Logger.Debug("flux: git sync probe failed", "err", err)
			}
			return false
		}
		lastProbeErr = ""
		if result.ExitCode != 0 {
			return false
		}
		return strings.TrimSpace(result.Stdout) == k8sBoolTrue
	}, timeout, env.Logger); err != nil {
		return fmt.Errorf("git repository sync not ready within %v: %w", timeout, err)
	}

	env.Logger.Info("flux: git repository synced")
	return nil
}

func (f *fluxAddon) createDeployKeySecret(ctx context.Context, env *addon.Environment, fs *Settings) error {
	repoURL := fs.Repository
	if repoURL == "" {
		return &errtypes.ConfigError{Msg: "flux repository not configured - set addons.flux.settings.repository in config"}
	}
	host, err := gitHost(repoURL)
	if err != nil {
		return fmt.Errorf("resolve git host for ssh-keyscan: %w", err)
	}

	// Matches the invoking user's `ssh-keygen -f ~/.ssh/...` even after the
	// deploy re-execs under sudo.
	homeDir, err := system.InvokingUserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}
	deployKeyFile := filepath.Join(homeDir, ".ssh", "flux-deploy-key")

	switch info, err := os.Lstat(deployKeyFile); {
	case err != nil && errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("deploy key not found at %s - generate with: ssh-keygen -t ed25519 -f ~/.ssh/flux-deploy-key -N ''", deployKeyFile)
	case err != nil:
		return fmt.Errorf("stat deploy key: %w", err)
	case info.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("deploy key %s is a symlink; refusing to follow", deployKeyFile)
	}

	privateKey, err := readKeyFile(deployKeyFile)
	if err != nil {
		return fmt.Errorf("read deploy key: %w", err)
	}

	// Public half is optional — flux only requires identity and known_hosts.
	publicKeyFile := deployKeyFile + ".pub"
	var publicKey []byte
	if b, err := readKeyFile(publicKeyFile); err == nil {
		publicKey = b
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read deploy key public half: %w", err)
	}

	// RunOutputChecked (not ring-truncated RunChecked): a truncated tail must fail closed.
	knownHostsResult, err := env.Exec.RunOutputChecked(ctx, 0, "ssh-keyscan", host)
	if err != nil {
		return fmt.Errorf("get host key for %s: %w", host, err)
	}
	if knownHostsResult.Truncated {
		return fmt.Errorf("ssh-keyscan output for %s exceeded the capture limit; refusing to verify a partial host-key list", host)
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
		return fmt.Errorf("apply deploy key secret: %w", applyErr)
	}

	env.Logger.Info("flux: deploy key secret applied")
	return nil
}

// verifyKeyscanFingerprint fails closed on an unpinned host unless acceptHostKey opts into TOFU.
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

// filterKeyscanLines drops comment/blank lines so the Secret is byte-stable across keyscan runs.
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

// gitHost extracts the host from a git URL: ssh://, https://, or scp-style user@host:path.
func gitHost(repoURL string) (string, error) {
	repoURL = strings.TrimSpace(repoURL)
	if repoURL == "" {
		return "", &errtypes.ConfigError{Msg: "empty repository URL"}
	}
	// scp-style user@host:path; error text uses only the substring after the
	// last @ to avoid leaking a token.
	if !strings.Contains(repoURL, "://") {
		at := strings.LastIndex(repoURL, "@")
		if at < 0 {
			return "", errors.New("cannot parse host from repository URL")
		}
		rest := repoURL[at+1:]
		host, _, ok := strings.Cut(rest, ":")
		if !ok {
			return "", fmt.Errorf("cannot parse host from %q", rest)
		}
		if host == "" || strings.HasPrefix(host, "-") {
			return "", fmt.Errorf("repository host %q is invalid", host)
		}
		return host, nil
	}
	u, err := url.Parse(repoURL)
	if err != nil {
		// *url.Error carries the raw URL (password included) verbatim; never wrap it.
		return "", errors.New("cannot parse repository URL")
	}
	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("%q has no host component", u.Redacted())
	}
	if strings.HasPrefix(host, "-") {
		return "", fmt.Errorf("repository host %q is invalid", host)
	}
	return host, nil
}

// buildFluxDeployKeySecret renders the deploy-key Secret manifest; inputs are
// []byte so callers can clear the private key after use.
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

// readKeyFile refuses to follow a symlink at the final path component (mirrors runlock.Acquire).
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
