// Package cluster provides a thin Kubernetes client wrapper around kubectl/oc
// for OKD cluster operations such as CSR approval, node readiness checks,
// and resource queries during install and postinstall phases.
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

// Client is a thin kubectl/oc wrapper used by the install and
// postinstall phases for CSR approval, readiness checks, and resource
// queries.
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

// WithEnvFallback applies environment-driven defaults when no explicit option
// has set them: reads KUBECONFIG from the process env when Kubeconfig is
// still empty, and upgrades the binary from "kubectl" to "oc" when "oc" is
// on PATH. Pass this option only when env-driven discovery is intentional;
// production callers (install, postinstall) supply WithCLI/WithKubeconfig
// explicitly so they get reproducible construction.
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

func (c *Client) run(ctx context.Context, args ...string) (*executor.Result, error) {
	result, err := c.exec.Run(ctx, c.CLI, args...)
	if err != nil {
		return nil, fmt.Errorf("%s %s failed: %w", c.CLI, subcommand(args), err)
	}
	return result, nil
}

func (c *Client) runCheck(ctx context.Context, args ...string) error {
	result, err := c.run(ctx, args...)
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
