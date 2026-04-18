// Package proxmox implements the Proxmox VE provider for OKD deployment.
//
// Typical lifecycle:
//
//	p := New(opts...)      // create provider with logger, env, project root
//	p.Connect(ctx, cfg)    // validate config and record host/node (no network I/O)
//	p.Provision(ctx, cfg, provOpts)  // init + plan + apply terraform, return VM IPs
//	p.Disconnect(ctx)      // release resources, nil out executor
//
// Connect does not verify Proxmox reachability because authentication is
// handled entirely via environment variables forwarded to terraform
// (PROXMOX_VE_ENDPOINT, PROXMOX_VE_API_TOKEN, etc.). The provider has no
// direct Proxmox HTTP client.
package proxmox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/netutil"
	"github.com/qxtaiba/okdctl/internal/tui"
)

type Provider struct {
	connected     bool
	host          string
	node          string
	terraformExec *terraform.Executor
	projectRoot   string
	tfEnv         string
	logger        *slog.Logger
	env           []string
}

type Option func(*Provider)

func WithProjectRoot(root string) Option {
	return func(p *Provider) { p.projectRoot = root }
}

func WithLogger(l *slog.Logger) Option {
	return func(p *Provider) {
		p.logger = l
	}
}

// WithEnv passes environment variables through to terraform for provider authentication.
func WithEnv(env []string) Option {
	return func(p *Provider) {
		p.env = append(p.env, env...)
	}
}

func New(opts ...Option) *Provider {
	p := &Provider{
		logger: logutil.NopLogger,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Connect validates that the required Proxmox configuration is present and
// records host/node for later use. It does NOT verify network connectivity
// to the Proxmox host because authentication is handled via environment
// variables (PROXMOX_VE_ENDPOINT, PROXMOX_VE_API_TOKEN) passed directly to
// terraform — the Provider has no Proxmox HTTP client. Connectivity issues
// surface during terraform plan/apply with clear provider-level errors.
func (p *Provider) Connect(_ context.Context, cfg *config.Config) error {
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

// Disconnect accepts a context for interface consistency but does not use it.
func (p *Provider) Disconnect(_ context.Context) error {
	p.connected = false
	p.terraformExec = nil
	return nil
}

// setupTerraform initializes (or reinitializes) the terraform executor for the
// given projectRoot/tfEnv. Not safe for concurrent use — the current call
// graph is sequential per Provider instance (one deployment per CLI run). If
// concurrent Provision ever becomes a requirement, add a mutex around
// terraformExec / projectRoot / tfEnv.
func (p *Provider) setupTerraform(projectRoot, tfEnv string) {
	if p.terraformExec != nil && p.projectRoot == projectRoot && p.tfEnv == tfEnv {
		return
	}

	p.projectRoot = projectRoot
	p.tfEnv = tfEnv

	tfDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", tfEnv)

	tfOpts := []terraform.Option{
		terraform.WithLogger(p.logger),
	}
	if len(p.env) > 0 {
		tfOpts = append(tfOpts, terraform.WithEnv(p.env))
	}

	p.terraformExec = terraform.NewWithVarFile(tfDir, filepath.Join(tfDir, "terraform.tfvars"), tfOpts...)
}

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
		return nil, fmt.Errorf("terraform init failed: %w", err)
	}

	p.logger.Info("terraform: creating execution plan")
	planOpts := terraform.PlanOptions{
		OutputPlanFile: terraform.PlanFileName,
	}
	if err := p.terraformExec.Plan(ctx, planOpts); err != nil {
		return nil, fmt.Errorf("terraform plan failed: %w", err)
	}

	totalNodes := 1 + cfg.Topology.ControlPlane.Count + cfg.Topology.Workers.Count
	p.logger.Info(fmt.Sprintf("terraform: plan will create %d virtual machines", totalNodes))

	p.logger.Info("terraform: applying infrastructure changes")
	stopSpinner := tui.StartSpinner(ctx, "applying terraform infrastructure")
	applyOpts := terraform.ApplyOptions{
		PlanFile:    filepath.Join(p.terraformExec.WorkDir, terraform.PlanFileName),
		AutoApprove: opts.AutoApprove,
	}
	applyErr := p.terraformExec.Apply(ctx, applyOpts)
	stopSpinner()
	if applyErr != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, fmt.Errorf("terraform apply interrupted: %w", applyErr)
		}
		p.logger.Warn("terraform: apply failed; partial infrastructure may exist. run 'okdctl destroy' to clean up")
		return nil, fmt.Errorf("terraform apply failed: %w", applyErr)
	}

	result, err := p.retrieveProvisionResult(cfg)
	if err != nil {
		return nil, fmt.Errorf("terraform apply succeeded but IP retrieval failed; run 'okdctl destroy' to clean up: %w", err)
	}

	if len(result.VMs) == 0 {
		return nil, fmt.Errorf("terraform apply succeeded but no VMs were provisioned; check config")
	}

	p.logger.Info(fmt.Sprintf("terraform: provisioned %d vms", len(result.VMs)))
	for _, vm := range result.VMs {
		if vm.IPAddress != "" {
			p.logger.Info(fmt.Sprintf("terraform: %s: %s", vm.Name, vm.IPAddress))
		}
	}

	return result, nil
}

// retrieveProvisionResult derives VM IPs from static config.
// IP scheme: bootstrap = start IP, masters = start+1..N, workers = start+N+1 onwards.
func (p *Provider) retrieveProvisionResult(cfg *config.Config) (*ProvisionResult, error) {
	result := &ProvisionResult{
		VMs:             []VMStatus{},
		ControlPlaneIPs: []string{},
		WorkerIPs:       []string{},
	}

	startIP := cfg.Networking.StaticIP.Start
	if startIP == "" {
		return nil, fmt.Errorf("static IP start address is required for OKD deployments")
	}

	totalNodes := 1 + cfg.Topology.ControlPlane.Count + cfg.Topology.Workers.Count
	if cfg.Networking.MachineCIDR != "" {
		if err := netutil.ValidateIPRangeInCIDR(startIP, totalNodes, cfg.Networking.MachineCIDR); err != nil {
			return nil, fmt.Errorf("IP range validation failed: %w", err)
		}
	}

	bootstrapIP := startIP
	result.BootstrapIP = bootstrapIP
	result.VMs = append(result.VMs, VMStatus{
		Name:      "bootstrap",
		Role:      RoleBootstrap,
		IPAddress: bootstrapIP,
		Status:    StateRunning,
	})

	for i := range cfg.Topology.ControlPlane.Count {
		ip, err := netutil.CalculateVMIP(startIP, 1+i)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate master%d IP: %w", i, err)
		}
		result.ControlPlaneIPs = append(result.ControlPlaneIPs, ip)
		result.VMs = append(result.VMs, VMStatus{
			Name:      fmt.Sprintf("master%d", i),
			Role:      RoleMaster,
			IPAddress: ip,
			Status:    StateRunning,
		})
	}

	workerOffset := 1 + cfg.Topology.ControlPlane.Count
	for i := range cfg.Topology.Workers.Count {
		ip, err := netutil.CalculateVMIP(startIP, workerOffset+i)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate worker%d IP: %w", i, err)
		}
		result.WorkerIPs = append(result.WorkerIPs, ip)
		result.VMs = append(result.VMs, VMStatus{
			Name:      fmt.Sprintf("worker%d", i),
			Role:      RoleWorker,
			IPAddress: ip,
			Status:    StateRunning,
		})
	}

	if cfg.Networking.Gateway != "" {
		result.APIServerIP = cfg.Networking.Gateway
	}

	return result, nil
}
