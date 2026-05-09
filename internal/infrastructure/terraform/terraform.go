// Package terraform provides a high-level interface for Terraform operations.
// It can be used by any infrastructure provider that uses Terraform.
package terraform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

// PlanFileName is the default plan file name used by Plan, Apply, and Cleanup.
const PlanFileName = "tfplan"

// ExecError reports a non-zero exit from a terraform subprocess. Aliased to
// the canonical executor.ExitError so callers can errors.As against either
// shape and so the two types do not drift.
type ExecError = executor.ExitError

// Executor wraps terraform subcommand execution for a single working
// directory with an optional var-file and verbose-logging toggle.
type Executor struct {
	WorkDir string
	VarFile string
	Verbose bool

	exec   *executor.Executor
	logger *slog.Logger
}

// Option configures an Executor at construction time.
type Option func(*Executor)

// WithLogger sets the slog logger used to narrate terraform invocations.
func WithLogger(l *slog.Logger) Option {
	return func(e *Executor) { e.logger = logutil.OrNop(l) }
}

// WithVerbose toggles verbose subprocess logging.
func WithVerbose(v bool) Option {
	return func(e *Executor) {
		e.Verbose = v
	}
}

// WithVarFile overrides the default var-file path (<workDir>/terraform.tfvars).
func WithVarFile(path string) Option {
	return func(e *Executor) { e.VarFile = path }
}

// WithEnv appends environment variables to be passed to all terraform subprocess calls.
// At execution time they are appended after the executor's allowlist-filtered
// parent env, so entries here override allowlist values for the same key.
// Multiple calls to WithEnv are cumulative; later entries for the same key win.
func WithEnv(env []string) Option {
	return func(e *Executor) {
		e.exec.Env = append(e.exec.Env, env...)
	}
}

// PlanOptions configures a terraform plan invocation.
//
// OutputPlanFile and Destroy are independent and may be combined (destroy plan
// saved to a file). Vars and VarFile are additive — both can be set.
type PlanOptions struct {
	// VarFile overrides the default terraform.tfvars path.
	VarFile        string
	OutputPlanFile string
	// Destroy creates a destruction plan instead of an apply plan.
	Destroy bool
	Vars    map[string]string
	// Targets limits the plan to specific resource addresses.
	Targets []string
}

// ApplyOptions configures a terraform apply invocation.
//
// PlanFile and Vars/VarFile are mutually exclusive: when PlanFile is set,
// terraform ignores Vars, VarFile, and AutoApprove because the plan file
// already encodes the full set of changes. If both are provided, PlanFile
// takes precedence and the other fields are silently unused.
type ApplyOptions struct {
	// VarFile overrides the default terraform.tfvars path.
	VarFile     string
	PlanFile    string
	AutoApprove bool
	Vars        map[string]string
	// Targets limits the apply to specific resource addresses.
	Targets []string
}

// DestroyOptions configures a terraform destroy invocation.
type DestroyOptions struct {
	// VarFile overrides the default terraform.tfvars path.
	VarFile     string
	AutoApprove bool
	Parallelism int
	// Targets limits the destroy to specific resource addresses.
	Targets []string
	// UsePlan creates a destroy plan first, then applies it.
	// Safer because it previews changes. Plan failures surface as errors
	// rather than silently degrading to direct destroy.
	UsePlan bool
}

// New constructs an Executor rooted at workDir with the default var-file
// path (<workDir>/terraform.tfvars).
func New(workDir string, opts ...Option) *Executor {
	e := &Executor{
		WorkDir: workDir,
		VarFile: filepath.Join(workDir, "terraform.tfvars"),
		exec:    executor.New(executor.WithWorkDir(workDir)),
		logger:  logutil.NopLogger,
	}
	for _, opt := range opts {
		opt(e)
	}
	executor.WithLogger(e.logger)(e.exec)
	return e
}

func (t *Executor) run(ctx context.Context, args ...string) error {
	t.exec.Verbose = t.Verbose

	t.logger.Info("terraform: running", "cmd", args[0])

	result, err := t.exec.Run(ctx, "terraform", args...)
	if err != nil {
		return fmt.Errorf("terraform %s failed: %w", args[0], err)
	}
	if result.ExitCode != 0 {
		return &ExecError{
			Command:  "terraform " + args[0],
			ExitCode: result.ExitCode,
			Stderr:   result.Stderr,
		}
	}
	return nil
}

// Init runs "terraform init" when the working directory is not already
// initialized. A partial init (some artifacts missing) triggers a re-init.
func (t *Executor) Init(ctx context.Context) error {
	terraformDir := filepath.Join(t.WorkDir, ".terraform")
	lockFile := filepath.Join(t.WorkDir, ".terraform.lock.hcl")
	providersDir := filepath.Join(terraformDir, "providers")

	dirOK := system.DirExists(terraformDir)
	lockOK := system.FileExists(lockFile)
	provOK := system.DirExists(providersDir)

	if dirOK && lockOK && provOK {
		t.logger.Info("terraform: already initialized")
		return nil
	}

	if dirOK || lockOK || provOK {
		t.logger.Info("terraform: partial initialization detected, re-initializing")
	} else {
		t.logger.Info("terraform: initializing")
	}

	return t.run(ctx, "init")
}

func (t *Executor) buildVarArgs(varFile string, vars map[string]string) []string {
	var args []string

	vf := varFile
	if vf == "" {
		vf = t.VarFile
	}
	if vf != "" {
		if system.FileExists(vf) {
			args = append(args, "-var-file="+vf)
		} else {
			t.logger.Warn("terraform: var file not found, skipping", "path", vf)
		}
	}

	for _, k := range slices.Sorted(maps.Keys(vars)) {
		args = append(args, "-var", fmt.Sprintf("%s=%s", k, vars[k]))
	}

	return args
}

// Plan runs "terraform plan" with the options in opts. When Destroy is true
// the plan is a destruction plan.
func (t *Executor) Plan(ctx context.Context, opts PlanOptions) error {
	args := []string{"plan"}
	args = append(args, t.buildVarArgs(opts.VarFile, opts.Vars)...)

	if opts.Destroy {
		args = append(args, "-destroy")
	}
	if opts.OutputPlanFile != "" {
		args = append(args, "-out="+opts.OutputPlanFile)
	}
	for _, target := range opts.Targets {
		args = append(args, "-target="+target)
	}

	return t.run(ctx, args...)
}

// PlanStreamed runs "terraform plan" streaming stdout and stderr directly to the
// terminal. Use instead of Plan when the operator must see the plan output —
// Plan captures into internal buffers and only surfaces stderr on failure.
func (t *Executor) PlanStreamed(ctx context.Context, opts PlanOptions) error {
	args := []string{"plan"}
	args = append(args, t.buildVarArgs(opts.VarFile, opts.Vars)...)

	if opts.Destroy {
		args = append(args, "-destroy")
	}
	if opts.OutputPlanFile != "" {
		args = append(args, "-out="+opts.OutputPlanFile)
	}
	for _, target := range opts.Targets {
		args = append(args, "-target="+target)
	}

	t.logger.Info("terraform: running plan (streaming to terminal)")
	return t.exec.RunInteractive(ctx, "terraform", args...)
}

// Apply runs "terraform apply". When opts.PlanFile is set, Vars, VarFile,
// and AutoApprove are ignored — the plan file encodes the full change set.
func (t *Executor) Apply(ctx context.Context, opts ApplyOptions) error {
	args := []string{"apply"}

	if opts.PlanFile != "" {
		args = append(args, opts.PlanFile)
		return t.run(ctx, args...)
	}

	args = append(args, t.buildVarArgs(opts.VarFile, opts.Vars)...)
	if opts.AutoApprove {
		args = append(args, "-auto-approve")
	}
	for _, target := range opts.Targets {
		args = append(args, "-target="+target)
	}

	return t.run(ctx, args...)
}

// Destroy runs "terraform destroy". When opts.UsePlan is true the destroy
// plan is generated first and applied, so plan failures surface cleanly
// before any infra mutation.
func (t *Executor) Destroy(ctx context.Context, opts DestroyOptions) error {
	if opts.UsePlan {
		return t.destroyWithPlan(ctx, opts)
	}
	return t.destroyDirect(ctx, opts)
}

// destroyWithPlan runs `terraform plan -destroy` and then applies the plan.
// Plan failures are returned to the caller; we do NOT silently fall back to
// direct destroy because a plan failure usually signals an auth/state issue
// the operator needs to see before mutating infra.
func (t *Executor) destroyWithPlan(ctx context.Context, opts DestroyOptions) error {
	planFile := filepath.Join(t.WorkDir, "destroy.tfplan")

	planErr := t.Plan(ctx, PlanOptions{
		VarFile:        opts.VarFile,
		OutputPlanFile: planFile,
		Destroy:        true,
		Targets:        opts.Targets,
	})

	if planErr != nil {
		return fmt.Errorf("terraform destroy plan failed: %w (re-run with an explicit fix or pass UsePlan=false to skip the plan step)", planErr)
	}

	applyOpts := ApplyOptions{
		PlanFile: planFile,
	}

	if err := t.Apply(ctx, applyOpts); err != nil {
		return err
	}

	return nil
}

// destroyDirect runs terraform destroy without an intermediate plan file.
// Currently no caller — Destroy is always invoked with UsePlan=true today —
// but retained as the canonical "emergency destroy" path so the argv shape
// (parallelism, -target injection) stays locked under regression coverage
// when an opt-in future caller lands.
func (t *Executor) destroyDirect(ctx context.Context, opts DestroyOptions) error {
	args := []string{"destroy"}
	args = append(args, t.buildVarArgs(opts.VarFile, nil)...)

	if opts.AutoApprove {
		args = append(args, "-auto-approve")
	}
	if opts.Parallelism > 0 {
		args = append(args, fmt.Sprintf("-parallelism=%d", opts.Parallelism))
	}
	for _, target := range opts.Targets {
		args = append(args, "-target="+target)
	}

	return t.run(ctx, args...)
}

// HasState reports whether the working directory contains a terraform.tfstate
// with at least one managed resource. A missing file, an empty state ({} or
// {"resources":[]}), or a file that fails JSON parse all return false. On a
// parse failure the file path is logged at Warn so the operator can inspect
// or remove the corrupt state before retrying.
func (t *Executor) HasState() bool {
	stateFile := filepath.Join(t.WorkDir, "terraform.tfstate")
	if !system.FileExists(stateFile) {
		return false
	}
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.logger.Warn("terraform: cannot read state file", "path", stateFile, "err", err)
		return false
	}
	var s struct {
		Resources []json.RawMessage `json:"resources"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		t.logger.Warn("terraform: state file is corrupt or unreadable; inspect before retrying destroy",
			"path", stateFile, "err", err)
		return false
	}
	return len(s.Resources) > 0
}

// SnapshotState copies terraform.tfstate to terraform.tfstate.<timestamp>.bak
// in WorkDir immediately before a destructive operation. Returns the snapshot
// path so callers can include it in error messages. Returns ("", nil) when no
// state file is present — callers treat an empty path as "nothing to snapshot".
// A write failure is a hard error; callers must not proceed with the
// destructive operation without a saved backup.
func (t *Executor) SnapshotState(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	src := filepath.Join(t.WorkDir, "terraform.tfstate")
	if !system.FileExists(src) {
		return "", nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf("terraform snapshot: read state: %w", err)
	}
	ts := strings.ReplaceAll(time.Now().UTC().Format(time.RFC3339), ":", "-")
	dst := filepath.Join(t.WorkDir, "terraform.tfstate."+ts+".bak")
	if err := system.AtomicWrite(dst, data, 0o600); err != nil {
		return "", fmt.Errorf("terraform snapshot: write %s: %w", dst, err)
	}
	t.pruneSnapshots()
	return dst, nil
}

// pruneSnapshots removes older terraform.tfstate.*.bak files from WorkDir,
// keeping only the 5 most recent. os.ReadDir returns entries sorted by name;
// because names encode UTC timestamps (lexicographic == chronological), no
// additional sort step is needed.
func (t *Executor) pruneSnapshots() {
	entries, err := os.ReadDir(t.WorkDir)
	if err != nil {
		t.logger.Warn("terraform: snapshot prune: cannot read workdir", "dir", t.WorkDir, "err", err)
		return
	}
	const retain = 5
	var snaps []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "terraform.tfstate.") && strings.HasSuffix(name, ".bak") {
			snaps = append(snaps, filepath.Join(t.WorkDir, name))
		}
	}
	if len(snaps) <= retain {
		return
	}
	for _, old := range snaps[:len(snaps)-retain] {
		if err := os.Remove(old); err != nil && !os.IsNotExist(err) {
			t.logger.Warn("terraform: snapshot prune: remove failed", "path", old, "err", err)
		}
	}
}

// ZeroizeEnv overwrites and clears the credential strings stored in the
// executor's Env slice. Entries whose key is PROXMOX_VE_PASSWORD or
// PROXMOX_VE_API_TOKEN are blanked first; the full slice is then cleared so
// all string headers are zeroed. Call this (typically via defer) after all
// terraform operations complete to bound the lifetime of plaintext credential
// strings in process memory.
func (t *Executor) ZeroizeEnv() {
	if t.exec == nil {
		return
	}
	for i, kv := range t.exec.Env {
		key, _, _ := strings.Cut(kv, "=")
		if key == "PROXMOX_VE_PASSWORD" || key == "PROXMOX_VE_API_TOKEN" {
			t.exec.Env[i] = ""
		}
	}
	clear(t.exec.Env)
	t.exec.Env = nil
}

// Output runs "terraform output -json" and returns the decoded top-level
// map. Each value remains JSON-encoded; callers unmarshal individual entries.
func (t *Executor) Output(ctx context.Context) (map[string]json.RawMessage, error) {
	t.exec.Verbose = t.Verbose
	result, err := t.exec.Run(ctx, "terraform", "output", "-json")
	if err != nil {
		return nil, fmt.Errorf("terraform output failed: %w", err)
	}
	if result.ExitCode != 0 {
		return nil, &ExecError{
			Command:  "terraform output",
			ExitCode: result.ExitCode,
			Stderr:   result.Stderr,
		}
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		return nil, fmt.Errorf("terraform output: invalid json: %w", err)
	}
	return out, nil
}

// CleanupPlans removes tfplan and destroy.tfplan; non-existent files are
// ignored. terraform.tfstate.backup is intentionally left so the operator
// retains a rollback artefact if the live tfstate is later corrupted.
func (t *Executor) CleanupPlans() error {
	var errs []error
	files := []string{
		filepath.Join(t.WorkDir, PlanFileName),
		filepath.Join(t.WorkDir, "destroy.tfplan"),
	}
	for _, f := range files {
		if err := system.SafeRemove(f); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("failed to remove %s: %w", f, err))
		}
	}
	return errors.Join(errs...)
}
