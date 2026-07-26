// Package cluster provides a thin Kubernetes client wrapper around kubectl/oc
// for OKD cluster operations such as CSR approval, node readiness checks, and
// resource queries, shared by phase code (via BasePhase.Oc*, which delegates
// internally) and non-phase CLI commands. Consumers define their own narrow
// interfaces to test against (see install.csrApprover); Client returns
// concrete types, never interfaces.
package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// Client is a kubectl/oc wrapper for the install and postinstall phases.
//
// Must be constructed via New — the zero value panics on first use (the
// backing executor and logger are set only in New).
type Client struct {
	CLI string

	Kubeconfig string

	exec   *executor.Executor
	logger *slog.Logger
}

// Option configures a Client at construction time.
type Option func(*Client)

// WithCLI overrides the CLI binary name (defaults to "kubectl", upgraded to
// "oc" when available).
func WithCLI(cli string) Option {
	return func(c *Client) { c.CLI = cli }
}

// WithKubeconfig points the client at a specific kubeconfig path.
func WithKubeconfig(path string) Option {
	return func(c *Client) { c.Kubeconfig = path }
}

// WithLogger injects a structured logger. Nil logger falls back to
// logutil.NopLogger.
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) { c.logger = logutil.OrNop(l) }
}

// WithExecutor injects an already-configured executor instead of letting
// New build a private one. The caller owns env wiring on this executor —
// WithKubeconfig and WithEnvFallback's KUBECONFIG injection only apply to a
// Client-owned executor and are silently skipped when this option is used.
// BasePhase.oc() uses this to share the phase's own executor (whose env
// already carries KUBECONFIG via Exec.AppendEnv) instead of duplicating the
// oc/kubectl invocation and error-formatting logic in phase code.
func WithExecutor(exec *executor.Executor) Option {
	return func(c *Client) { c.exec = exec }
}

// WithEnvFallback reads the process environment and PATH at application time:
// it sets Kubeconfig from $KUBECONFIG when still empty, and upgrades the
// binary from "kubectl" to "oc" when "oc" is on PATH. It MUST be the last
// option passed to New(); any WithKubeconfig or WithCLI that follows will
// silently overwrite the env-derived values, negating the fallback.
//
// Deliberately unwired today: every okdctl command targets the
// workspace-managed kubeconfig (<projectRoot>/okd-install/auth/kubeconfig),
// and honoring $KUBECONFIG would silently retarget a command at whatever
// unrelated cluster the user's env points to. Wire it only for a command
// that must operate without a workspace — e.g. inspecting or adopting a
// cluster okdctl did not deploy.
func WithEnvFallback() Option {
	return func(c *Client) {
		if c.Kubeconfig == "" {
			if kc := os.Getenv("KUBECONFIG"); kc != "" {
				if err := validateKubeconfigEnv(kc); err != nil {
					c.logger.Debug("ignoring $KUBECONFIG env value", "err", err)
				} else {
					c.Kubeconfig = kc
				}
			}
		}
		if c.CLI == "kubectl" && executor.CommandExists("oc") {
			c.CLI = "oc"
		}
	}
}

// validateKubeconfigEnv checks a KUBECONFIG path read from the process
// environment. It rejects symlinks (Lstat does not follow the link,
// eliminating the TOCTOU window that stat+open would leave) and paths
// outside the $HOME or /etc prefix allowlist, preventing a hostile env var
// from pointing the client at /dev/zero, /proc/self/environ, or similar.
func validateKubeconfigEnv(path string) error {
	clean := filepath.Clean(path)

	fi, err := os.Lstat(clean)
	if err != nil {
		return fmt.Errorf("kubeconfig path inaccessible: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("kubeconfig path is a symlink")
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		return &errtypes.AuthError{
			Msg:  fmt.Sprintf("kubeconfig has insecure permissions %#o; run 'chmod 600 <path>' to fix", perm),
			Path: clean,
			Err:  os.ErrPermission,
		}
	}

	home, _ := os.UserHomeDir()
	sep := string(filepath.Separator)
	for _, prefix := range []string{home, "/etc"} {
		if prefix != "" && strings.HasPrefix(clean, prefix+sep) {
			return nil
		}
	}
	return fmt.Errorf("kubeconfig path outside allowed prefixes ($HOME, /etc)")
}

// New builds a Client applying the supplied options in order.
// It does not read the process environment or probe PATH; callers wanting
// those defaults must pass WithEnvFallback() explicitly.
func New(opts ...Option) *Client {
	c := &Client{
		CLI:    "kubectl",
		logger: logutil.NopLogger,
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.exec == nil {
		opts := []executor.Option{executor.WithLogger(c.logger)}
		if c.Kubeconfig != "" {
			opts = append(opts, executor.WithEnv([]string{fmt.Sprintf("KUBECONFIG=%s", c.Kubeconfig)}))
		}
		c.exec = executor.New(opts...)
	}

	return c
}

// subcommand returns args[0] when present, or "(no args)". Used for error
// formatting so we never embed arbitrary arg values (which could carry
// --from-literal=... style secrets) in wrapped errors or logs.
func subcommand(args []string) string {
	if len(args) == 0 {
		return "(no args)"
	}
	return args[0]
}

// Run executes `<cli> <args...>` once via the shared executor and returns
// the raw *executor.Result; callers inspect Result.ExitCode for command
// failures. A non-nil error means a transport failure (binary missing, ctx
// cancellation), wrapped with the CLI name and subcommand only — never the
// full arg list — so a caller passing e.g. --from-literal=token=... never
// leaks it into a wrapped error or log.
func (c *Client) Run(ctx context.Context, args ...string) (*executor.Result, error) {
	result, err := c.exec.Run(ctx, c.CLI, args...)
	if err != nil {
		return nil, fmt.Errorf("%s %s failed: %w", c.CLI, subcommand(args), err)
	}
	return result, nil
}

func (c *Client) runOutput(ctx context.Context, args ...string) (*executor.Result, error) {
	result, err := c.exec.RunOutput(ctx, 0, c.CLI, args...)
	if err != nil {
		return nil, fmt.Errorf("%s %s failed: %w", c.CLI, subcommand(args), err)
	}
	return result, nil
}

func (c *Client) runCheck(ctx context.Context, args ...string) error {
	result, err := c.Run(ctx, args...)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		stderr := strings.TrimSpace(result.Stderr)
		if stderr == "" {
			stderr = strings.TrimSpace(result.Stdout)
		}
		// On a cancelled ctx this yields context.Canceled; otherwise a
		// typed *executor.ExitError callers can errors.As for ExitCode.
		return executor.NewExitError(ctx, c.CLI+" "+subcommand(args), result.ExitCode, stderr)
	}
	return nil
}
