// Package proxmox implements the Proxmox VE provider: Provider drives
// Connect/Provision/Disconnect over a Terraform executor, with auth passed
// via env vars (PROXMOX_VE_ENDPOINT, PROXMOX_VE_API_TOKEN).
package proxmox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox/hostssh"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/sshpin"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// Provider drives the Proxmox VE lifecycle (connect, provision, disconnect)
// via a Terraform executor. All mutations MUST flow through it — direct HTTP
// calls are forbidden since the terraform provider owns retry/backoff.
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

// WithLogger sets the Provider's logger; nil resolves to logutil.NopLogger.
func WithLogger(l *slog.Logger) Option {
	return func(p *Provider) { p.logger = logutil.OrNop(l) }
}

// WithEnv passes env vars through to terraform for provider auth.
func WithEnv(env []string) Option {
	return func(p *Provider) {
		p.env = append(p.env, env...)
	}
}

// WithProgressReporter sets the long-running-op callback, defaulting to
// logutil.NopProgressReporter (silent) when omitted.
func WithProgressReporter(r logutil.ProgressReporter) Option {
	return func(p *Provider) { p.reporter = r }
}

// WithSSHExec sets the executor for post-apply pvesh enumeration probes;
// when omitted, Provision relies on the terraform output cross-check alone.
func WithSSHExec(e *executor.Executor) Option {
	return func(p *Provider) { p.sshExec = e }
}

// ZeroizeEnv blanks Provider's secret-keyed env entries, then clears it and
// zeroizes the terraform executor's copy too (setupTerraform's copy would
// else outlive GC). Call via defer immediately after construction.
func (p *Provider) ZeroizeEnv() {
	for i, kv := range p.env {
		key, _, _ := strings.Cut(kv, "=")
		if logutil.KeyIsSecret(key) {
			p.env[i] = ""
		}
	}
	clear(p.env)
	p.env = nil
	if p.terraformExec != nil {
		p.terraformExec.ZeroizeEnv()
	}
}

// New constructs a Provider with the given options; the logger defaults to
// a no-op logger if WithLogger is omitted.
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

// Connect validates required Proxmox config and records host/node; connectivity
// isn't checked (auth flows via env vars to terraform); ctx is for future symmetry.
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
			// AuthError failures return unchanged (exit 5) — wrapping would let
			// exitCodeFor's NetworkError(3) check match first and downgrade it.
			var authErr *errtypes.AuthError
			if errors.As(err, &authErr) {
				return err
			}
			return &errtypes.NetworkError{Msg: "proxmox host key verification failed", Err: err}
		}
		p.knownHostsPath = path
	}
	p.connected = true
	return nil
}

// Disconnect resets connection state; the ignored ctx is scaffolding for a
// future graceful-teardown handshake (Proxmox is the sole provider today).
func (p *Provider) Disconnect(_ context.Context) error {
	p.connected = false
	// Callers defer ZeroizeEnv+Disconnect together (LIFO runs Disconnect
	// first), so wipe the executor's env here before nilling the reference.
	if p.terraformExec != nil {
		p.terraformExec.ZeroizeEnv()
	}
	p.terraformExec = nil
	return nil
}

// setupTerraform (re)initializes the terraform executor for
// projectRoot/tfEnv; not safe for concurrent use (one deployment per CLI run).
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

// Provision runs terraform init/plan/apply for the configured environment;
// Connect must run first, or it returns ErrNotConnected.
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
	p.logger.Info("terraform: plan will create virtual machines", "count", totalNodes)

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
			// Bare wrap intentional: signalExitCode checks errors.Is(_, context.Canceled)
			// before exitCodeFor runs — don't wrap this in errtypes.ClusterError.
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
			p.logger.Info("terraform: vm provisioned", "node", n.name, "ip", n.ip)
		}
	}

	return nil
}

// planPreviewFileName differs from terraform.PlanFileName so preview and apply plans can't collide.
const planPreviewFileName = "plan-preview.tfplan"

// PlanPreview inits and plans without applying; opts.ProjectRoot/TerraformEnv
// must be set, and the plan file is always removed before returning.
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
	p.logger.Info("terraform: plan preview", "count", totalNodes)

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
// post-apply summary log; it makes no claim about observed VM state.
type vmNodeSpec struct {
	name string
	ip   string
}

// planProvisionedNodes validates the static IP range fits the topology/CIDR
// and returns config-derived name/IP pairs; no VM state is observed here.
func (p *Provider) planProvisionedNodes(cfg *config.Config) ([]vmNodeSpec, error) {
	if cfg.Networking.StaticIP.Start == "" {
		return nil, &errtypes.ConfigError{Msg: "static IP start address is required for OKD deployments"}
	}

	enum, err := nodetypes.ClusterNodes(cfg)
	if err != nil {
		return nil, err
	}
	nodes := make([]vmNodeSpec, len(enum))
	for i, n := range enum {
		nodes[i] = vmNodeSpec{name: n.Name(), ip: n.IP}
	}
	return nodes, nil
}

// checkTerraformOutputs cross-checks vm_ids counts from terraform output
// against topology; IP comparison is deferred until outputs.tf exposes IPs.
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

// initWithRetry retries terraform init via system.DefaultBackoff() for
// retryable failures (see initIsRetryable), demoting repeat warns to Debug.
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

// initIsRetryable classifies a terraform init error: config/auth, missing
// binary, and cancellation are permanent; anything else is retried.
func initIsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, exec.ErrNotFound) {
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
	// enumNo: probe ran and parsed, but vmidBase was absent.
	enumNo
	// enumProbeSkipped: probe failed to run or parse; callers default to VM present.
	enumProbeSkipped
)

// vmIDProbe is the pvesh QEMU-list element shape probeVMEnumeration parses.
type vmIDProbe struct {
	VMID int `json:"vmid"`
}

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
