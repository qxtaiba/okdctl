// Package proxmox implements the Proxmox VE provider for OKD deployment.
// The Provider drives Connect/Provision/Disconnect over a Terraform
// executor; authentication is handled entirely via environment variables
// forwarded to terraform (PROXMOX_VE_ENDPOINT, PROXMOX_VE_API_TOKEN).
package proxmox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/netutil"
	"github.com/qxtaiba/okdctl/internal/sshpin"
)

// Provider drives the Proxmox VE infrastructure lifecycle (connect, provision,
// disconnect) via a Terraform executor.
//
// Mutation invariant (state:48688e63): all Proxmox mutations MUST flow through
// terraform.Executor. Direct Proxmox HTTP calls are forbidden in deploy/destroy
// paths — the bpg/proxmox terraform provider owns 5xx/408/429 retry/backoff.
// If status reads are added later, route them through internal/download's
// retryDownload/isRetryable helpers (exponential backoff, 4xx fail-fast).
type Provider struct {
	connected      bool
	host           string
	node           string
	knownHostsPath string
	terraformExec  *terraform.Executor
	projectRoot    string
	tfEnv          string
	logger         *slog.Logger
	env            []string
	reporter       logutil.ProgressReporter
	sshExec        *executor.Executor
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

// WithProgressReporter sets the callback used to signal long-running
// operations. Defaults to logutil.NopProgressReporter when omitted, so
// headless callers run silent.
func WithProgressReporter(r logutil.ProgressReporter) Option {
	return func(p *Provider) { p.reporter = r }
}

// WithSSHExec sets the executor used for post-apply pvesh enumeration probes.
// When omitted the probe is skipped and Provision relies on the terraform
// output cross-check alone.
func WithSSHExec(exec *executor.Executor) Option {
	return func(p *Provider) { p.sshExec = exec }
}

// ZeroizeEnv overwrites and clears the credential strings stored in the
// Provider's env slice. Entries whose key matches logutil.KeyIsSecret are
// blanked first; the full slice is then cleared so all string headers are
// zeroed. Call via defer immediately after construction so even
// Connect/Provision failures still wipe plaintext credentials.
func (p *Provider) ZeroizeEnv() {
	for i, kv := range p.env {
		key, _, _ := strings.Cut(kv, "=")
		if logutil.KeyIsSecret(key) {
			p.env[i] = ""
		}
	}
	clear(p.env)
	p.env = nil
}

// New constructs a Provider with the given options. The logger defaults to
// a no-op logger if WithLogger is not supplied.
func New(opts ...Option) *Provider {
	p := &Provider{
		logger:   logutil.NopLogger,
		reporter: logutil.NopProgressReporter,
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
// ctx is accepted for symmetry with future network-bound providers; this
// implementation is local-only.
func (p *Provider) Connect(ctx context.Context, cfg *config.Config) error {
	if cfg == nil {
		return &errtypes.ConfigError{Msg: "configuration is required"}
	}
	if cfg.Provider.Proxmox == nil {
		return &errtypes.ConfigError{Msg: "proxmox configuration is required"}
	}
	p.host = cfg.Provider.Proxmox.Host
	if err := config.ValidateProxmoxHost(p.host); err != nil {
		return &errtypes.ConfigError{Msg: "proxmox host is invalid", Err: err}
	}
	p.node = cfg.Provider.Proxmox.Node
	if p.sshExec != nil && (cfg.Provider.Proxmox.SSHHostFingerprint != "" || cfg.Provider.Proxmox.RequirePinnedFingerprint) {
		path, err := sshpin.Verify(ctx, phase.ProxmoxBareHost(p.host), cfg.Provider.Proxmox.SSHHostFingerprint, cfg.Provider.Proxmox.RequirePinnedFingerprint, p.logger)
		if err != nil {
			return &errtypes.NetworkError{Msg: "proxmox host key verification failed", Err: err}
		}
		p.knownHostsPath = path
	}
	p.connected = true
	return nil
}

// Disconnect resets connection state. ctx is accepted for symmetry with future
// network-bound providers; this implementation is local-only.
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
		return nil, &errtypes.ConfigError{Msg: "proxmox provider not connected — call Connect() first", Err: ErrNotConnected}
	}

	if opts.ProjectRoot != "" && opts.TerraformEnv != "" {
		p.setupTerraform(opts.ProjectRoot, opts.TerraformEnv)
	}

	if p.terraformExec == nil {
		return nil, &errtypes.ConfigError{Msg: "terraform executor not configured — set ProjectRoot and TerraformEnv", Err: ErrTerraformNotConfigured}
	}

	p.logger.Info("terraform: initializing backend and providers")
	if err := p.initWithRetry(ctx); err != nil {
		return nil, &errtypes.ClusterError{Msg: "terraform init failed", Err: err}
	}

	p.logger.Info("terraform: creating execution plan")
	planOpts := terraform.PlanOptions{
		OutputPlanFile: terraform.PlanFileName,
	}
	if err := p.terraformExec.Plan(ctx, planOpts); err != nil {
		return nil, &errtypes.ClusterError{Msg: "terraform plan failed", Err: err}
	}
	defer func() { _ = p.terraformExec.CleanupPlans() }()

	totalNodes := 1 + cfg.Topology.ControlPlane.Count + cfg.Topology.Workers.Count
	p.logger.Info("terraform: plan will create virtual machines", "count", totalNodes)

	p.logger.Info("terraform: applying infrastructure changes")
	stopSpinner := p.reporter("applying terraform infrastructure")
	applyOpts := terraform.ApplyOptions{
		PlanFile:    filepath.Join(p.terraformExec.WorkDir, terraform.PlanFileName),
		AutoApprove: opts.AutoApprove,
	}
	snapPath, snapErr := p.terraformExec.SnapshotState(ctx)
	if snapErr != nil {
		return nil, &errtypes.ClusterError{Msg: "provision: state snapshot failed", Err: snapErr}
	}

	applyErr := p.terraformExec.Apply(ctx, applyOpts)
	stopSpinner()
	if applyErr != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, fmt.Errorf("terraform apply interrupted: %w", errors.Join(ctx.Err(), applyErr))
		}
		p.logger.Warn("terraform: apply failed; partial infrastructure may exist. run 'okdctl destroy' to clean up", "err", applyErr)
		msg := "terraform apply failed"
		if snapPath != "" {
			msg = fmt.Sprintf("terraform apply failed (state backup: %s)", snapPath)
		}
		return nil, &errtypes.ClusterError{Msg: msg, Err: applyErr}
	}

	result, err := p.retrieveProvisionResult(cfg)
	if err != nil {
		return nil, &errtypes.ClusterError{Msg: "terraform apply succeeded but IP retrieval failed; run 'okdctl destroy' to clean up", Err: err}
	}

	if len(result.VMs) == 0 {
		return nil, &errtypes.ClusterError{Msg: "terraform apply succeeded but no VMs were provisioned; check config"}
	}

	p.checkTerraformOutputs(ctx, cfg)
	vmidEnumerable := p.probeVMEnumeration(ctx, cfg)

	p.logger.Info("terraform: vms provisioned", "count", len(result.VMs))
	if vmidEnumerable {
		for _, vm := range result.VMs {
			if vm.IPAddress != "" {
				p.logger.Info("terraform: vm provisioned", "vm", vm.Name, "ip", vm.IPAddress)
			}
		}
	}

	return result, nil
}

// PlanOnly runs terraform init and plan for the configured environment without
// applying changes. opts.ProjectRoot and opts.TerraformEnv must both be set.
// Plan output streams to the terminal via PlanStreamed. Used by --dry-run deploy.
func (p *Provider) PlanOnly(ctx context.Context, cfg *config.Config, opts ProvisionOptions) error {
	if !p.connected {
		return &errtypes.ConfigError{Msg: "proxmox provider not connected — call Connect() first", Err: ErrNotConnected}
	}

	if opts.ProjectRoot != "" && opts.TerraformEnv != "" {
		p.setupTerraform(opts.ProjectRoot, opts.TerraformEnv)
	}

	if p.terraformExec == nil {
		return &errtypes.ConfigError{Msg: "terraform executor not configured — set ProjectRoot and TerraformEnv", Err: ErrTerraformNotConfigured}
	}

	p.logger.Info("terraform: initializing backend and providers")
	if err := p.initWithRetry(ctx); err != nil {
		return &errtypes.ClusterError{Msg: "terraform init failed", Err: err}
	}

	totalNodes := 1 + cfg.Topology.ControlPlane.Count + cfg.Topology.Workers.Count
	p.logger.Info("terraform: plan preview", "vm_count", totalNodes)

	if err := p.terraformExec.PlanStreamed(ctx, terraform.PlanOptions{}); err != nil {
		return &errtypes.ClusterError{Msg: "terraform plan failed", Err: err}
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
			return nil, &errtypes.ConfigError{Msg: "IP range validation failed", Err: err}
		}
	}

	bootstrapIP := startIP
	result.BootstrapIP = bootstrapIP
	result.VMs = append(result.VMs, VMStatus{
		Name:      string(RoleBootstrap),
		Role:      RoleBootstrap,
		IPAddress: bootstrapIP,
		Status:    phase.StateRunning,
	})

	for i := range cfg.Topology.ControlPlane.Count {
		ip, err := netutil.CalculateVMIP(startIP, 1+i)
		if err != nil {
			return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("failed to calculate %s%d IP", RoleMaster, i), Err: err}
		}
		result.ControlPlaneIPs = append(result.ControlPlaneIPs, ip)
		result.VMs = append(result.VMs, VMStatus{
			Name:      fmt.Sprintf("%s%d", RoleMaster, i),
			Role:      RoleMaster,
			IPAddress: ip,
			Status:    phase.StateRunning,
		})
	}

	workerOffset := 1 + cfg.Topology.ControlPlane.Count
	for i := range cfg.Topology.Workers.Count {
		ip, err := netutil.CalculateVMIP(startIP, workerOffset+i)
		if err != nil {
			return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("failed to calculate %s%d IP", RoleWorker, i), Err: err}
		}
		result.WorkerIPs = append(result.WorkerIPs, ip)
		result.VMs = append(result.VMs, VMStatus{
			Name:      fmt.Sprintf("%s%d", RoleWorker, i),
			Role:      RoleWorker,
			IPAddress: ip,
			Status:    phase.StateRunning,
		})
	}

	if cfg.Networking.Gateway != "" {
		result.APIServerIP = cfg.Networking.Gateway
	}

	return result, nil
}

// checkTerraformOutputs cross-checks the vm_ids counts from terraform output
// against the topology config. IP-address comparison is deferred: outputs.tf
// today exposes only vm_ids; adding control_plane_ips/worker_ips requires
// HCL changes not in scope of the current fix.
func (p *Provider) checkTerraformOutputs(ctx context.Context, cfg *config.Config) {
	outputs, err := p.terraformExec.Output(ctx)
	if err != nil {
		p.logger.Warn("terraform: output readback failed; ip arithmetic may address phantom nodes", "err", err)
		return
	}
	raw, ok := outputs["vm_ids"]
	if !ok {
		p.logger.Warn("terraform: vm_ids output missing; outputs.tf may have drifted from applied HCL")
		return
	}
	var wrapper struct {
		Value struct {
			Bootstrap *int  `json:"bootstrap"`
			Masters   []int `json:"masters"`
			Workers   []int `json:"workers"`
		} `json:"value"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		p.logger.Warn("terraform: vm_ids output unparseable; schema may have drifted", "err", err)
		return
	}
	v := wrapper.Value
	wantMasters := cfg.Topology.ControlPlane.Count
	wantWorkers := cfg.Topology.Workers.Count
	if v.Bootstrap == nil || len(v.Masters) != wantMasters || len(v.Workers) != wantWorkers {
		p.logger.Warn("terraform: vm_ids count mismatch; ip arithmetic may address phantom nodes",
			"want_masters", wantMasters, "got_masters", len(v.Masters),
			"want_workers", wantWorkers, "got_workers", len(v.Workers),
			"bootstrap_present", v.Bootstrap != nil)
	}
}

// initWithRetry wraps terraform init in a bounded retry for transient
// failures (network blips during provider-plugin download, brief Proxmox
// API unavailability). 3 attempts, exponential backoff starting at 5 s,
// factor 2, jitter 0.5, 5-minute cap. Permanent failures (config/auth
// errors, context cancellation) abort on the first attempt.
func (p *Provider) initWithRetry(ctx context.Context) error {
	backoff := wait.Backoff{
		Duration: 5 * time.Second,
		Factor:   2,
		Jitter:   0.5,
		Steps:    3,
		Cap:      5 * time.Minute,
	}
	var lastWarnMsg string
	var lastErr error
	var attempt int
	err := wait.ExponentialBackoffWithContext(ctx, backoff, func(_ context.Context) (bool, error) {
		attempt++
		if initErr := p.terraformExec.Init(ctx); initErr != nil {
			if !initIsRetryable(initErr) {
				return false, initErr
			}
			lastErr = initErr
			msg := initErr.Error()
			if msg != lastWarnMsg {
				p.logger.Warn("terraform: init failed, retrying", "attempt", attempt, "err", initErr)
				lastWarnMsg = msg
			} else {
				p.logger.Debug("terraform: init failed (repeated), retrying", "attempt", attempt, "err", initErr)
			}
			return false, nil
		}
		return true, nil
	})
	if err == nil {
		return nil
	}
	if lastErr != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return lastErr
	}
	return err
}

// initIsRetryable reports whether an error from terraform init should
// trigger another attempt. Config/auth errors and context cancellation
// are permanent; everything else (non-zero exit, network/DNS failure) is
// transient and worth retrying.
func initIsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var cfgErr *errtypes.ConfigError
	if errors.As(err, &cfgErr) {
		return false
	}
	var authErr *errtypes.AuthError
	return !errors.As(err, &authErr)
}

// probeVMEnumeration queries pvesh over SSH to check whether vmidBase is
// visible in the Proxmox QEMU list. Returns true when the vmid is found or
// when the probe cannot run (no sshExec, SSH error, parse error) — callers
// treat those cases as "do not suppress per-VM logs". Returns false only when
// the probe ran and parsed successfully but vmidBase was not present, meaning
// the VMs are not yet enumerable.
func (p *Provider) probeVMEnumeration(ctx context.Context, cfg *config.Config) bool {
	if p.sshExec == nil {
		return true
	}
	vmidBase := cfg.Topology.VMIDBase
	if vmidBase == 0 {
		vmidBase = 6000
	}
	params := &phase.RemoteISOParams{
		Host:           phase.ProxmoxBareHost(p.host),
		Node:           p.node,
		Exec:           p.sshExec,
		KnownHostsPath: p.knownHostsPath,
	}
	stdout, err := phase.PveshRun(ctx, params, "get", "/nodes/"+p.node+"/qemu")
	if err != nil {
		p.logger.Info("terraform: pvesh probe skipped", "err", err)
		return true
	}
	var vms []struct {
		VMID int `json:"vmid"`
	}
	if err := json.Unmarshal([]byte(stdout), &vms); err != nil {
		p.logger.Info("terraform: pvesh probe payload unparseable", "err", err)
		return true
	}
	for _, vm := range vms {
		if vm.VMID == vmidBase {
			return true
		}
	}
	p.logger.Info("terraform: vm not yet enumerable, install phase will retry", "vmid", vmidBase)
	return false
}
