package deployment

import (
	"context"
	"os"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd"
	"github.com/qxtaiba/okd-proxmox-cli/internal/logging"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

// Result contains information gathered during deployment.
type Result struct {
	// RouterLBIP is the LoadBalancer IP assigned to the default ingress router.
	RouterLBIP string

	// GrappleberryRouterIP is the LoadBalancer IP for the grappleberry ingress (if configured).
	GrappleberryRouterIP string
}

// Options configures deployment execution behavior.
type Options struct {
	// ShowStartMessage displays "starting deployment..." before beginning.
	ShowStartMessage bool

	// Logger is used for output during deployment.
	Logger logging.Logger

	// CredentialsEnv contains environment variables for credentials.
	// These are passed to subprocesses without modifying global environment.
	// Use credentials.ProxmoxCredentials.Env() to generate this.
	CredentialsEnv []string

	// OnStart is called when deployment begins.
	OnStart func(clusterName string)

	// OnComplete is called when deployment finishes successfully.
	// Duration is the total deployment time, Result contains deployment info.
	OnComplete func(duration time.Duration, result *Result)

	// OnError is called when deployment fails.
	OnError func(err error)
}

// Run executes the complete deployment workflow.
//
// The function performs:
//  1. Creates the OKD provisioner and validates configuration
//  2. Creates and runs the Executor
//  3. Returns error if deployment fails
//
// Callers should handle interrupt signals separately and pass a cancellable context.
func Run(ctx context.Context, cfg *config.Config, opts Options) error {
	if opts.Logger == nil {
		opts.Logger = logging.NoopLogger()
	}

	clusterFQDN := cfg.Cluster.Name + "." + cfg.Cluster.Domain

	if opts.ShowStartMessage && opts.OnStart != nil {
		opts.OnStart(clusterFQDN)
	}

	projectRoot, err := os.Getwd()
	if err != nil {
		return utils.WrapError("failed to get working directory", err)
	}

	// Build provisioner options
	provisionerOpts := []okd.ProvisionerOption{
		okd.WithProjectRoot(projectRoot),
		okd.WithLogger(opts.Logger),
	}

	// Add credentials environment if provided
	if len(opts.CredentialsEnv) > 0 {
		provisionerOpts = append(provisionerOpts, okd.WithEnv(opts.CredentialsEnv))
	}

	// Create provisioner
	p := okd.New(cfg.Distribution.Version, provisionerOpts...)

	// Validate with provisioner
	if err := p.Validate(cfg); err != nil {
		return utils.WrapError("provisioner validation failed", err)
	}

	// Create Executor
	exec := NewExecutor(
		cfg,
		p,
		projectRoot,
		WithLogger(opts.Logger),
	)

	startTime := time.Now()

	// Run deployment phases
	result, err := exec.Execute(ctx)
	if err != nil {
		if opts.OnError != nil {
			opts.OnError(err)
		}
		return utils.WrapError("deployment failed", err)
	}

	// Success
	duration := time.Since(startTime).Round(time.Second)
	if opts.OnComplete != nil {
		opts.OnComplete(duration, result)
	}

	return nil
}
