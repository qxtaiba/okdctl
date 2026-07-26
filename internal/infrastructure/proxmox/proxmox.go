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
	"slices"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox/hostssh"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/netutil"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/sshpin"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// Provider drives the Proxmox VE infrastructure lifecycle (connect, provision,
// disconnect) via a Terraform executor.
//
// Mutation invariant: all Proxmox mutations MUST flow through
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
// implementation is local-only. See Disconnect for the scaffolding rationale.
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
	if err := config.ValidateProxmoxNodeName(p.node); err != nil {
		return &errtypes.ConfigError{Msg: "proxmox node name is invalid", Err: err}
	}
	if p.sshExec != nil && (cfg.Provider.Proxmox.SSHHostFingerprint != "" || cfg.Provider.Proxmox.RequirePinnedFingerprint) {
		path, err := sshpin.Verify(ctx, hostssh.ProxmoxBareHost(p.host), cfg.Provider.Proxmox.SSHHostFingerprint, cfg.Provider.Proxmox.RequirePinnedFingerprint, p.logger)
		if err != nil {
			return &errtypes.NetworkError{Msg: "proxmox host key verification failed", Err: err}
		}
		p.knownHostsPath = path
	}
	p.connected = true
	return nil
}

// Disconnect resets connection state. ctx is accepted for symmetry with future
// network-bound providers; this implementation is local-only. The _ receiver
// is intentional — if Proxmox ever adds a graceful session-teardown handshake
// (e.g., via the Proxmox VE API), ctx threads through without a signature
// change. Multi-provider support is explicitly out of scope (Proxmox is
// the sole provider); this scaffolding is for a future network-bound
// Proxmox disconnect, not a provider abstraction.
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

	tfDir := workspace.TerraformEnvDir(projectRoot, tfEnv)

	tfOpts := []terraform.Option{
		terraform.WithLogger(p.logger),
	}
	if len(p.env) > 0 {
		tfOpts = append(tfOpts, terraform.WithEnv(p.env))
	}

	p.terraformExec = terraform.New(tfDir, tfOpts...)
}

// Provision runs terraform init/plan/apply for the configured environment.
// Connect must have run first; otherwise this returns ErrNotConnected. It is
// error-only: the prior *ProvisionResult return fabricated VMStatus.Status
// as StateRunning before any VM was observed running and set APIServerIP to
// the network gateway (a different machine); the sole caller discarded the
// value anyway.
func (p *Provider) Provision(ctx context.Context, cfg *config.Config, opts ProvisionOptions) error {
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

	p.logger.Info("terraform: creating execution plan")
	planOpts := terraform.PlanOptions{
		OutputPlanFile: terraform.PlanFileName,
	}
	if err := p.terraformExec.Plan(ctx, planOpts); err != nil {
		return &errtypes.ClusterError{Msg: "terraform plan failed", Err: err}
	}
	defer func() { _ = p.terraformExec.CleanupPlans() }()

	totalNodes := 1 + cfg.Topology.ControlPlane.Count + cfg.Topology.Workers.Count
	p.logger.Info("terraform: plan will create virtual machines", "vm_count", totalNodes)

	p.logger.Info("terraform: applying infrastructure changes")
	stopSpinner := p.reporter("applying terraform infrastructure")
	applyOpts := terraform.ApplyOptions{
		PlanFile:    filepath.Join(p.terraformExec.WorkDir(), terraform.PlanFileName),
		AutoApprove: opts.AutoApprove,
	}
	snapPath, snapErr := p.terraformExec.SnapshotState(ctx)
	if snapErr != nil {
		return &errtypes.ClusterError{Msg: "provision: state snapshot failed", Err: snapErr}
	}

	applyErr := p.terraformExec.Apply(ctx, applyOpts)
	stopSpinner()
	if applyErr != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			// Bare wrap intentional: cli/root.go::signalExitCode walks the chain
			// via errors.Is(err, context.Canceled) before exitCodeFor runs,
			// mapping SIGINT→130 / SIGTERM→143 without a typed error. Do not
			// wrap this in errtypes.ClusterError.
			return fmt.Errorf("terraform apply interrupted: %w", errors.Join(ctx.Err(), applyErr))
		}
		p.logger.Warn("terraform: apply failed; partial infrastructure may exist. run 'okdctl destroy' to clean up", "err", applyErr)
		msg := "terraform apply failed"
		if snapPath != "" {
			msg = fmt.Sprintf("terraform apply failed (state backup: %s)", snapPath)
		}
		return &errtypes.ClusterError{Msg: msg, Err: applyErr}
	}

	nodes, err := p.planProvisionedNodes(cfg)
	if err != nil {
		return &errtypes.ClusterError{Msg: "terraform apply succeeded but IP validation failed; run 'okdctl destroy' to clean up", Err: err}
	}

	p.checkTerraformOutputs(ctx, cfg)
	enumState := p.probeVMEnumeration(ctx, cfg)

	p.logger.Info("terraform: vms provisioned", "count", len(nodes))
	if enumState != enumNo {
		for _, n := range nodes {
			p.logger.Info("terraform: vm provisioned", "vm", n.name, "ip", n.ip)
		}
	}

	return nil
}

// planPreviewFileName is the plan file PlanPreview writes, distinct from
// terraform.PlanFileName so a preview run never collides with a plan file
// an apply left behind (concurrent runs are still serialized by runlock).
const planPreviewFileName = "plan-preview.tfplan"

// PlanPreview runs terraform init and a saved plan for the configured
// environment, returning the parsed non-no-op resource changes without
// applying anything. opts.ProjectRoot and opts.TerraformEnv must both be
// set. The plan file is removed before returning, success or failure, so
// PlanPreview never leaves an apply-able artefact on disk. Used by both
// `okdctl plan` and `deploy --dry-run`.
func (p *Provider) PlanPreview(ctx context.Context, cfg *config.Config, opts ProvisionOptions) ([]terraform.ResourceChange, error) {
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

	absPlanFile := filepath.Join(p.terraformExec.WorkDir(), planPreviewFileName)
	defer func() { _ = system.SafeRemove(absPlanFile) }()

	totalNodes := 1 + cfg.Topology.ControlPlane.Count + cfg.Topology.Workers.Count
	p.logger.Info("terraform: plan preview", "vm_count", totalNodes)

	hasChanges, err := p.terraformExec.PlanDetailed(ctx, terraform.PlanOptions{OutputPlanFile: planPreviewFileName})
	if err != nil {
		return nil, &errtypes.ClusterError{Msg: "terraform plan failed", Err: err}
	}
	if !hasChanges {
		return nil, nil
	}

	changes, err := p.terraformExec.ShowPlanChanges(ctx, absPlanFile)
	if err != nil {
		return nil, &errtypes.ClusterError{Msg: "terraform show plan failed", Err: err}
	}
	return changes, nil
}

// vmNodeSpec pairs a node name with its config-derived static IP for the
// post-apply summary log. It makes no claim about observed VM state — see
// planProvisionedNodes.
type vmNodeSpec struct {
	name string
	ip   string
}

// planProvisionedNodes validates that the configured static IP range fits
// the topology and CIDR, then returns the name/IP pairs Provision logs
// after a successful apply. It reports config-derived addresses only, with
// no state claim — no VM's running status or API server reachability is
// observed here.
// IP scheme: bootstrap = start IP, masters = start+1..N, workers = start+N+1 onwards.
func (p *Provider) planProvisionedNodes(cfg *config.Config) ([]vmNodeSpec, error) {
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

	nodes := []vmNodeSpec{{name: string(nodetypes.RoleBootstrap), ip: startIP}}

	for i := range cfg.Topology.ControlPlane.Count {
		ip, err := netutil.CalculateVMIP(startIP, 1+i)
		if err != nil {
			return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("calculate %s%d IP", nodetypes.RoleMaster, i), Err: err}
		}
		nodes = append(nodes, vmNodeSpec{name: fmt.Sprintf("%s%d", nodetypes.RoleMaster, i), ip: ip})
	}

	workerOffset := 1 + cfg.Topology.ControlPlane.Count
	for i := range cfg.Topology.Workers.Count {
		ip, err := netutil.CalculateVMIP(startIP, workerOffset+i)
		if err != nil {
			return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("calculate %s%d IP", nodetypes.RoleWorker, i), Err: err}
		}
		nodes = append(nodes, vmNodeSpec{name: fmt.Sprintf("%s%d", nodetypes.RoleWorker, i), ip: ip})
	}

	return nodes, nil
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
// API unavailability) via system.DefaultBackoff(). Permanent failures
// (config/auth errors, context cancellation) abort on the first attempt.
// Repeated identical failures demote to Debug after the first Warn so a
// long retry run doesn't spam the log.
func (p *Provider) initWithRetry(ctx context.Context) error {
	initWarn := logutil.NewDedupWarner(p.logger)
	var attempt int
	return system.Retry(ctx, system.DefaultBackoff(), initIsRetryable, func(ctx context.Context) error {
		attempt++
		initErr := p.terraformExec.Init(ctx)
		if initErr == nil || !initIsRetryable(initErr) {
			return initErr
		}
		initWarn.Warn(initErr.Error(), "terraform: init failed, retrying", "attempt", attempt, "err", initErr)
		return initErr
	})
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

// vmEnumerationState classifies the pvesh VM-enumeration probe outcome.
type vmEnumerationState int

const (
	// enumYes: vmidBase was found in the pvesh QEMU list.
	enumYes vmEnumerationState = iota
	// enumNo: the probe ran and parsed successfully but vmidBase was absent
	// — the VM is not yet enumerable.
	enumNo
	// enumProbeSkipped: no sshExec, an SSH error, or a parse error — the
	// probe could not run, so callers treat the VM as present by default.
	enumProbeSkipped
)

// vmIDProbe is the pvesh QEMU-list element shape probeVMEnumeration parses.
type vmIDProbe struct {
	VMID int `json:"vmid"`
}

// probeVMEnumeration queries pvesh over SSH to check whether vmidBase is
// visible in the Proxmox QEMU list.
func (p *Provider) probeVMEnumeration(ctx context.Context, cfg *config.Config) vmEnumerationState {
	if p.sshExec == nil {
		return enumProbeSkipped
	}
	vmidBase := cfg.Topology.VMIDBase
	if vmidBase == 0 {
		vmidBase = config.DefaultVMIDBase
	}
	params := &hostssh.RemoteISOParams{
		Host:           hostssh.ProxmoxBareHost(p.host),
		Node:           p.node,
		Exec:           p.sshExec,
		KnownHostsPath: p.knownHostsPath,
	}
	stdout, err := hostssh.PveshRun(ctx, params, "get", "qemu")
	if err != nil {
		p.logger.Debug("terraform: pvesh probe skipped", "err", err)
		return enumProbeSkipped
	}
	var vms []vmIDProbe
	if err := json.Unmarshal([]byte(stdout), &vms); err != nil {
		p.logger.Debug("terraform: pvesh probe payload unparseable", "err", err)
		return enumProbeSkipped
	}
	if slices.ContainsFunc(vms, func(v vmIDProbe) bool { return v.VMID == vmidBase }) {
		return enumYes
	}
	p.logger.Info("terraform: vm not yet enumerable, install phase will retry", "vmid", vmidBase)
	return enumNo
}
