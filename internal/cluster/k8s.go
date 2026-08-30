// Package cluster provides a thin kubectl/oc client wrapper for OKD cluster
// operations, shared by phase code and non-phase CLI commands.
package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// Client is a kubectl/oc wrapper for the install and postinstall phases.
// Must be constructed via New — the zero value panics on first use.
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

// WithLogger injects a structured logger; nil falls back to logutil.NopLogger.
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) { c.logger = logutil.OrNop(l) }
}

// WithExecutor injects an already-configured executor instead of letting New
// build a private one. WithKubeconfig and WithEnvFallback's KUBECONFIG
// injection are silently skipped when this option is used — the caller owns
// env wiring.
func WithExecutor(exec *executor.Executor) Option {
	return func(c *Client) { c.exec = exec }
}

// WithEnvFallback sets Kubeconfig from $KUBECONFIG and upgrades the CLI to
// "oc" when available. It MUST be the last option passed to New — any
// WithKubeconfig or WithCLI passed afterward silently overwrites the
// env-derived values.
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

// validateKubeconfigEnv rejects symlinks (avoids a TOCTOU race) and paths
// outside the $HOME/etc allowlist.
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
		return fmt.Errorf("kubeconfig %q has insecure permissions %#o; run 'chmod 600 <path>' to fix", clean, perm)
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

// New builds a Client applying the supplied options in order. It does not
// read the environment or probe PATH; pass WithEnvFallback explicitly for that.
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

// subcommand returns args[0] (or "(no args)") — full argv could leak secrets into logs.
func subcommand(args []string) string {
	if len(args) == 0 {
		return "(no args)"
	}
	return args[0]
}

// Run executes `<cli> <args...>` via the shared executor.
// A non-nil error means a transport failure — check Result.ExitCode for
// command failures; the error omits full argv, which could carry secrets.
func (c *Client) Run(ctx context.Context, args ...string) (*executor.Result, error) {
	result, err := c.exec.Run(ctx, c.CLI, args...)
	if err != nil {
		return nil, fmt.Errorf("run %s %s: %w", c.CLI, subcommand(args), err)
	}
	return result, nil
}

func (c *Client) runOutput(ctx context.Context, args ...string) (*executor.Result, error) {
	result, err := c.exec.RunOutput(ctx, 0, c.CLI, args...)
	if err != nil {
		return nil, fmt.Errorf("run %s %s: %w", c.CLI, subcommand(args), err)
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
		// Cancelled ctx yields context.Canceled; otherwise *executor.ExitError.
		return executor.NewExitError(ctx, c.CLI+" "+subcommand(args), result.ExitCode, stderr)
	}
	return nil
}
