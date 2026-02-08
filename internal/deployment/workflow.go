package deployment

import (
	"context"
	"os"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

type Result struct {
	// KubeVipIP is the kube-vip virtual IP for the API server.
	KubeVipIP string

	// BastionIP is the bastion IP currently handling *.apps ingress via HAProxy.
	BastionIP string

	// BootstrapCleaned indicates the bootstrap VM was destroyed after install.
	BootstrapCleaned bool

	// DNSDeployed indicates production DNS was deployed (api → VIP, apps → bastion).
	DNSDeployed bool
}

type Options struct {
	ShowStartMessage bool

	Logger utils.Logger

	// Passed to subprocesses without modifying global environment.
	// Use credentials.ProxmoxCredentials.Env() to generate this.
	CredentialsEnv []string

	OnStart func(clusterName string)

	OnComplete func(duration time.Duration, result *Result)

	OnError func(err error)
}

// Run expects a cancellable context; callers should handle interrupt signals separately.
func Run(ctx context.Context, cfg *config.Config, opts Options) error {
	if opts.Logger == nil {
		opts.Logger = utils.NoopLogger()
	}

	clusterFQDN := cfg.Cluster.Name + "." + cfg.Cluster.Domain

	if opts.ShowStartMessage && opts.OnStart != nil {
		opts.OnStart(clusterFQDN)
	}

	projectRoot, err := os.Getwd()
	if err != nil {
		return utils.WrapError("failed to get working directory", err)
	}

	provisionerOpts := []okd.ProvisionerOption{
		okd.WithProjectRoot(projectRoot),
		okd.WithLogger(opts.Logger),
	}

	if len(opts.CredentialsEnv) > 0 {
		provisionerOpts = append(provisionerOpts, okd.WithEnv(opts.CredentialsEnv))
	}

	p := okd.New(cfg.Distribution.Version, provisionerOpts...)

	if err := p.Validate(cfg); err != nil {
		return utils.WrapError("provisioner validation failed", err)
	}

	exec := NewExecutor(
		cfg,
		p,
		projectRoot,
		WithLogger(opts.Logger),
	)

	startTime := time.Now()

	result, err := exec.Execute(ctx)
	if err != nil {
		if opts.OnError != nil {
			opts.OnError(err)
		}
		return utils.WrapError("deployment failed", err)
	}

	duration := time.Since(startTime).Round(time.Second)
	if opts.OnComplete != nil {
		opts.OnComplete(duration, result)
	}

	return nil
}
