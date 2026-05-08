// Package cluster provides a thin Kubernetes client wrapper around kubectl/oc
// for OKD cluster operations such as CSR approval, node readiness checks,
// and resource queries during install and postinstall phases.
package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// K8sClient is a thin kubectl/oc wrapper used by the install and
// postinstall phases for CSR approval, readiness checks, and resource
// queries.
type K8sClient struct {
	CLI string

	Kubeconfig string

	exec   *executor.Executor
	logger *slog.Logger
}

// Option configures a K8sClient at construction time.
type Option func(*K8sClient)

// WithCLI overrides the CLI binary name (defaults to "kubectl", upgraded to
// "oc" when available).
func WithCLI(cli string) Option {
	return func(c *K8sClient) { c.CLI = cli }
}

// WithKubeconfig points the client at a specific kubeconfig path.
func WithKubeconfig(path string) Option {
	return func(c *K8sClient) { c.Kubeconfig = path }
}

// WithLogger injects a structured logger. Nil logger falls back to
// logutil.NopLogger.
func WithLogger(l *slog.Logger) Option {
	return func(c *K8sClient) { c.logger = logutil.OrNop(l) }
}

// WithEnvFallback applies environment-driven defaults when no explicit option
// has set them: reads KUBECONFIG from the process env when Kubeconfig is
// still empty, and upgrades the binary from "kubectl" to "oc" when "oc" is
// on PATH. Pass this option only when env-driven discovery is intentional;
// production callers (install, postinstall) supply WithCLI/WithKubeconfig
// explicitly so they get reproducible construction.
func WithEnvFallback() Option {
	return func(c *K8sClient) {
		if c.Kubeconfig == "" {
			c.Kubeconfig = os.Getenv("KUBECONFIG")
		}
		if c.CLI == "kubectl" && executor.CommandExists("oc") {
			c.CLI = "oc"
		}
	}
}

// NewK8sClient builds a K8sClient applying the supplied options in order.
// It does not read the process environment or probe PATH; callers wanting
// those defaults must pass WithEnvFallback() explicitly.
func NewK8sClient(opts ...Option) *K8sClient {
	c := &K8sClient{
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

func (c *K8sClient) run(ctx context.Context, args ...string) (*executor.Result, error) {
	result, err := c.exec.Run(ctx, c.CLI, args...)
	if err != nil {
		return nil, fmt.Errorf("%s %s failed: %w", c.CLI, subcommand(args), err)
	}
	return result, nil
}

func (c *K8sClient) runCheck(ctx context.Context, args ...string) error {
	result, err := c.run(ctx, args...)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		stderr := strings.TrimSpace(result.Stderr)
		if stderr == "" {
			stderr = strings.TrimSpace(result.Stdout)
		}
		// Return a typed *executor.ExitError so callers can errors.As to
		// inspect ExitCode without re-parsing the message.
		return &executor.ExitError{
			Command:  c.CLI + " " + subcommand(args),
			ExitCode: result.ExitCode,
			Stderr:   stderr,
		}
	}
	return nil
}
