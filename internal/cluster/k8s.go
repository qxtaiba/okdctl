package cluster

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

// K8sClient provides a high-level interface for Kubernetes operations.
// It uses kubectl or oc as the underlying CLI tool.
type K8sClient struct {
	// CLI is the command to use (kubectl, oc)
	CLI string

	// Kubeconfig is the path to the kubeconfig file
	Kubeconfig string

	exec *executor.Executor
}

// Option configures a K8sClient.
type Option func(*K8sClient)

// WithCLI sets the CLI tool to use (kubectl or oc).
func WithCLI(cli string) Option {
	return func(c *K8sClient) { c.CLI = cli }
}

// WithKubeconfig sets the path to the kubeconfig file.
func WithKubeconfig(path string) Option {
	return func(c *K8sClient) { c.Kubeconfig = path }
}

// NewK8sClient creates a new Kubernetes client with optional configuration.
func NewK8sClient(opts ...Option) *K8sClient {
	c := &K8sClient{
		CLI: "kubectl", // Default CLI
	}

	// Check for KUBECONFIG environment variable as default
	if envKubeconfig := os.Getenv("KUBECONFIG"); envKubeconfig != "" {
		c.Kubeconfig = envKubeconfig
	}

	// Apply options
	for _, opt := range opts {
		opt(c)
	}

	// Auto-detect CLI if not explicitly set or set to default
	if c.CLI == "kubectl" {
		if executor.CommandExists("oc") {
			c.CLI = "oc"
		}
	}

	// Create executor with kubeconfig environment if not already set
	if c.exec == nil {
		cmdRunner := executor.New()
		if c.Kubeconfig != "" {
			cmdRunner.Env = []string{fmt.Sprintf("KUBECONFIG=%s", c.Kubeconfig)}
		}
		c.exec = cmdRunner
	}

	return c
}

// run executes a kubectl/oc command and checks the result.
func (c *K8sClient) run(ctx context.Context, args ...string) (*executor.Result, error) {
	result, err := c.exec.Run(ctx, c.CLI, args...)
	if err != nil {
		return nil, utils.WrapErrorf(err, "%s %s failed", c.CLI, strings.Join(args, " "))
	}
	return result, nil
}

// runCheck executes a command and returns an error if it fails.
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
		return fmt.Errorf("%s %s failed (exit %d): %s",
			c.CLI, strings.Join(args, " "), result.ExitCode, stderr)
	}
	return nil
}
