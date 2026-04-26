// Package proxmox implements the Proxmox VE provider for OKD deployment.
// The Provider drives Connect/Provision/Disconnect over a Terraform
// executor; authentication is handled entirely via environment variables
// forwarded to terraform (PROXMOX_VE_ENDPOINT, PROXMOX_VE_API_TOKEN).
package proxmox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/netutil"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// Provider drives the Proxmox VE infrastructure lifecycle (connect, provision,
// disconnect) via a Terraform executor.
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

// Option configures a Provider at construction time.
type Option func(*Provider)

// WithProjectRoot sets the project root used to locate the Terraform tree.
func WithProjectRoot(root string) Option {
	return func(p *Provider) { p.projectRoot = root }
}

// WithLogger sets the slog logger used by the Provider. Nil resolves to
// logutil.NopLogger to match the other infrastructure constructors.
func WithLogger(l *slog.Logger) Option {
	return func(p *Provider) { p.logger = logutil.OrNop(l) }
}

// WithEnv passes environment variables through to terraform for provider authentication.
func WithEnv(env []string) Option {
	return func(p *Provider) {
		p.env = append(p.env, env...)
	}
}

// New constructs a Provider with the given options. The logger defaults to
// a no-op logger if WithLogger is not supplied.
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
		return &errtypes.ConfigError{Msg: "configuration is required"}
	}
	if cfg.Provider.Proxmox == nil {
		return &errtypes.ConfigError{Msg: "proxmox configuration is required"}
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

	p.terraformExec = terraform.New(tfDir, tfOpts...)
}

// Provision runs terraform init/plan/apply for the configured environment
// and returns the VM IPs. Connect must have run first; otherwise this
// returns ErrNotConnected.
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
	defer func() { _ = p.terraformExec.CleanupPlans() }()

	totalNodes := 1 + cfg.Topology.ControlPlane.Count + cfg.Topology.Workers.Count
	p.logger.Info("terraform: plan will create virtual machines", "count", totalNodes)

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
		p.logger.Warn("terraform: apply failed; partial infrastructure may exist. run 'okdctl destroy' to clean up", "err", applyErr)
		return nil, fmt.Errorf("terraform apply failed: %w", applyErr)
	}

	result, err := p.retrieveProvisionResult(cfg)
	if err != nil {
		return nil, fmt.Errorf("terraform apply succeeded but IP retrieval failed; run 'okdctl destroy' to clean up: %w", err)
	}

	if len(result.VMs) == 0 {
		return nil, &errtypes.ClusterError{Msg: "terraform apply succeeded but no VMs were provisioned; check config"}
	}

	p.logger.Info("terraform: vms provisioned", "count", len(result.VMs))
	for _, vm := range result.VMs {
		if vm.IPAddress != "" {
			p.logger.Info("terraform: vm provisioned", "vm", vm.Name, "ip", vm.IPAddress)
		}
	}

	return result, nil
}

// PlanOnly runs terraform init and plan for the configured environment without
// applying changes. opts.ProjectRoot and opts.TerraformEnv must both be set.
// Plan output streams to the terminal via PlanStreamed. Used by --dry-run deploy.
func (p *Provider) PlanOnly(ctx context.Context, cfg *config.Config, opts ProvisionOptions) error {
	if !p.connected {
		return ErrNotConnected
	}

	if opts.ProjectRoot != "" && opts.TerraformEnv != "" {
		p.setupTerraform(opts.ProjectRoot, opts.TerraformEnv)
	}

	if p.terraformExec == nil {
		return ErrTerraformNotConfigured
	}

	p.logger.Info("terraform: initializing backend and providers")
	if err := p.terraformExec.Init(ctx); err != nil {
		return fmt.Errorf("terraform init failed: %w", err)
	}

	totalNodes := 1 + cfg.Topology.ControlPlane.Count + cfg.Topology.Workers.Count
	p.logger.Info("terraform: plan preview", "vm_count", totalNodes)

	if err := p.terraformExec.PlanStreamed(ctx, terraform.PlanOptions{}); err != nil {
		return fmt.Errorf("terraform plan failed: %w", err)
	}

	return nil
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
		return nil, &errtypes.ConfigError{Msg: "static IP start address is required for OKD deployments"}
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
		Name:      string(RoleBootstrap),
		Role:      RoleBootstrap,
		IPAddress: bootstrapIP,
		Status:    StateRunning,
	})

	for i := range cfg.Topology.ControlPlane.Count {
		ip, err := netutil.CalculateVMIP(startIP, 1+i)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate %s%d IP: %w", RoleMaster, i, err)
		}
		result.ControlPlaneIPs = append(result.ControlPlaneIPs, ip)
		result.VMs = append(result.VMs, VMStatus{
			Name:      fmt.Sprintf("%s%d", RoleMaster, i),
			Role:      RoleMaster,
			IPAddress: ip,
			Status:    StateRunning,
		})
	}

	workerOffset := 1 + cfg.Topology.ControlPlane.Count
	for i := range cfg.Topology.Workers.Count {
		ip, err := netutil.CalculateVMIP(startIP, workerOffset+i)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate %s%d IP: %w", RoleWorker, i, err)
		}
		result.WorkerIPs = append(result.WorkerIPs, ip)
		result.VMs = append(result.VMs, VMStatus{
			Name:      fmt.Sprintf("%s%d", RoleWorker, i),
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
