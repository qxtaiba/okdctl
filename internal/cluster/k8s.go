// Package cluster provides a thin Kubernetes client wrapper around kubectl/oc
// for OKD cluster operations such as CSR approval, node readiness checks,
// and resource queries during install and postinstall phases.
package cluster

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

type K8sClient struct {
	CLI string

	Kubeconfig string

	exec *executor.Executor
}

type Option func(*K8sClient)

func WithCLI(cli string) Option {
	return func(c *K8sClient) { c.CLI = cli }
}

func WithKubeconfig(path string) Option {
	return func(c *K8sClient) { c.Kubeconfig = path }
}

func NewK8sClient(opts ...Option) *K8sClient {
	c := &K8sClient{
		CLI: "kubectl",
	}

	if envKubeconfig := os.Getenv("KUBECONFIG"); envKubeconfig != "" {
		c.Kubeconfig = envKubeconfig
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.CLI == "kubectl" {
		if executor.CommandExists("oc") {
			c.CLI = "oc"
		}
	}

	if c.exec == nil {
		cmdRunner := executor.New()
		if c.Kubeconfig != "" {
			cmdRunner.Env = []string{fmt.Sprintf("KUBECONFIG=%s", c.Kubeconfig)}
		}
		c.exec = cmdRunner
	}

	return c
}

func (c *K8sClient) run(ctx context.Context, args ...string) (*executor.Result, error) {
	result, err := c.exec.Run(ctx, c.CLI, args...)
	if err != nil {
		return nil, utils.WrapErrorf(err, "%s %s failed", c.CLI, strings.Join(args, " "))
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
		return fmt.Errorf("%s %s failed (exit %d): %s",
			c.CLI, strings.Join(args, " "), result.ExitCode, stderr)
	}
	return nil
}
