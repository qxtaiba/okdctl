// Package proxmox implements the Proxmox VE provider for OKD deployment.
package proxmox

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/infrastructure/terraform"
	"github.com/qxtaiba/okd-proxmox-cli/internal/logging"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/netutil"
)

// Provider implements the Proxmox VE infrastructure provider.
type Provider struct {
	connected     bool
	host          string
	node          string
	terraformExec *terraform.Executor
	projectRoot   string
	tfEnv         string
	logger        logging.Logger
	env           []string
}

// Option configures a Provider.
type Option func(*Provider)

// WithProjectRoot sets the project root directory.
func WithProjectRoot(root string) Option {
	return func(p *Provider) { p.projectRoot = root }
}

// WithLogger sets a custom logger for the provider.
func WithLogger(l logging.Logger) Option {
	return func(p *Provider) {
		p.logger = l
	}
}

// WithEnv sets environment variables for subprocess execution.
// These are passed to terraform for provider authentication.
func WithEnv(env []string) Option {
	return func(p *Provider) {
		p.env = append(p.env, env...)
	}
}

// New creates a new Proxmox provider with optional configuration.
func New(opts ...Option) *Provider {
	p := &Provider{
		logger: logging.NoopLogger(),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Connect establishes a connection to the Proxmox API.
func (p *Provider) Connect(ctx context.Context, cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("configuration is required")
	}
	if cfg.Provider.Proxmox == nil {
		return fmt.Errorf("proxmox configuration is required")
	}
	p.host = cfg.Provider.Proxmox.Host
	p.node = cfg.Provider.Proxmox.Node
	p.connected = true
	return nil
}

// Disconnect closes the connection to Proxmox.
// The context parameter is accepted for interface consistency but not used
// as disconnect is a synchronous, non-blocking operation.
func (p *Provider) Disconnect(_ context.Context) error {
	p.connected = false
	p.terraformExec = nil
	return nil
}

// setupTerraform configures the Terraform executor for provisioning.
func (p *Provider) setupTerraform(projectRoot, tfEnv string) {
	p.projectRoot = projectRoot
	p.tfEnv = tfEnv

	if p.terraformExec != nil {
		return
	}

	tfDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", tfEnv)

	var tfOpts []terraform.Option
	if len(p.env) > 0 {
		tfOpts = append(tfOpts, terraform.WithEnv(p.env))
	}

	p.terraformExec = terraform.NewWithVarFile(tfDir, filepath.Join(tfDir, "terraform.tfvars"), tfOpts...)
}

// Provision creates VMs on Proxmox using Terraform.
func (p *Provider) Provision(ctx context.Context, cfg *config.Config, opts ProvisionOptions) (*ProvisionResult, error) {
	if !p.connected {
		return nil, ErrNotConnected
	}

	if opts.ProjectRoot != "" && opts.TerraformEnv != "" {
		p.setupTerraform(opts.ProjectRoot, opts.TerraformEnv)
	}

	if p.terraformExec == nil {
		return nil, ErrTerraformNotConfigured
	}

	p.logger.Info("terraform: initializing backend and providers")
	if err := p.terraformExec.Init(ctx); err != nil {
		return nil, utils.WrapError("terraform init failed", err)
	}

	p.logger.Info("terraform: creating execution plan")
	planOpts := terraform.PlanOptions{
		OutputPlanFile: "tfplan",
	}
	if err := p.terraformExec.Plan(ctx, planOpts); err != nil {
		return nil, utils.WrapError("terraform plan failed", err)
	}

	totalNodes := 1 + cfg.Topology.ControlPlane.Count + cfg.Topology.Workers.Count // bootstrap + masters + workers
	p.logger.Info(fmt.Sprintf("terraform: plan will create %d virtual machines", totalNodes))

	p.logger.Info("terraform: applying infrastructure changes")
	applyOpts := terraform.ApplyOptions{
		PlanFile:    filepath.Join(p.terraformExec.GetWorkDir(), "tfplan"),
		AutoApprove: opts.AutoApprove,
	}
	if err := p.terraformExec.Apply(ctx, applyOpts); err != nil {
		if ctx.Err() == context.Canceled {
			return nil, utils.WrapError("terraform apply interrupted", context.Canceled)
		}
		return nil, utils.WrapError("terraform apply failed", err)
	}

	result, err := p.retrieveProvisionResult(ctx, cfg)
	if err != nil {
		p.logger.Warn(fmt.Sprintf("could not retrieve vm ips from terraform outputs: %v", err))
		return &ProvisionResult{
			VMs:             []VMStatus{},
			ControlPlaneIPs: []string{},
			WorkerIPs:       []string{},
		}, nil
	}

	if len(result.VMs) > 0 {
		p.logger.Info(fmt.Sprintf("terraform: provisioned %d vms", len(result.VMs)))
		for _, vm := range result.VMs {
			if vm.IPAddress != "" {
				p.logger.Info(fmt.Sprintf("terraform: %s: %s", vm.Name, vm.IPAddress))
			}
		}
	}

	return result, nil
}

// retrieveProvisionResult builds VM information from the config's static IP assignments.
// OKD uses static IPs configured via ignition files, not dynamic assignment from Proxmox.
// The IP scheme is: bootstrap = start IP, masters = start+1 to start+N, workers = start+N+1 onwards.
func (p *Provider) retrieveProvisionResult(_ context.Context, cfg *config.Config) (*ProvisionResult, error) {
	result := &ProvisionResult{
		VMs:             []VMStatus{},
		ControlPlaneIPs: []string{},
		WorkerIPs:       []string{},
	}

	startIP := cfg.Networking.StaticIP.Start
	if startIP == "" {
		return result, nil // No static IP config, return empty result
	}

	baseIP, lastOctet, err := netutil.SplitIPv4(startIP)
	if err != nil {
		return nil, utils.WrapErrorf(err, "invalid start IP %q", startIP)
	}

	totalNodes := 1 + cfg.Topology.ControlPlane.Count + cfg.Topology.Workers.Count
	maxOctet := lastOctet + totalNodes - 1
	if maxOctet > 255 {
		return nil, fmt.Errorf("IP range overflow: starting IP %s with %d nodes would exceed .255 (needs up to .%d)", startIP, totalNodes, maxOctet)
	}

	bootstrapIP := fmt.Sprintf("%s.%d", baseIP, lastOctet)
	result.BootstrapIP = bootstrapIP
	result.VMs = append(result.VMs, VMStatus{
		Name:      "bootstrap",
		Role:      string(RoleBootstrap),
		IPAddress: bootstrapIP,
		Status:    string(StateRunning),
	})

	for i := 0; i < cfg.Topology.ControlPlane.Count; i++ {
		ip := fmt.Sprintf("%s.%d", baseIP, lastOctet+1+i)
		result.ControlPlaneIPs = append(result.ControlPlaneIPs, ip)
		result.VMs = append(result.VMs, VMStatus{
			Name:      fmt.Sprintf("master%d", i),
			Role:      string(RoleMaster),
			IPAddress: ip,
			Status:    string(StateRunning),
		})
	}

	workerOffset := 1 + cfg.Topology.ControlPlane.Count
	for i := 0; i < cfg.Topology.Workers.Count; i++ {
		ip := fmt.Sprintf("%s.%d", baseIP, lastOctet+workerOffset+i)
		result.WorkerIPs = append(result.WorkerIPs, ip)
		result.VMs = append(result.VMs, VMStatus{
			Name:      fmt.Sprintf("worker%d", i),
			Role:      string(RoleWorker),
			IPAddress: ip,
			Status:    string(StateRunning),
		})
	}

	if cfg.Networking.Gateway != "" {
		result.APIServerIP = cfg.Networking.Gateway
	}

	return result, nil
}
