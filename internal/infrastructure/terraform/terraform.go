// Package terraform runs Terraform subcommands via internal/executor with an
// env allowlist, exposing Init, Plan, Apply, Destroy, and Output on Executor.
package terraform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

// PlanFileName is the default plan file name used by Plan, Apply, and CleanupPlans.
const PlanFileName = "tfplan"

// defaultLockTimeout bounds -lock-timeout so a stale lock from a SIGKILL-ed
// prior run waits then fails cleanly.
const defaultLockTimeout = "120s"

// ExecError is a true type alias for executor.ExitError, so errors.As works
// against either name interchangeably.
type ExecError = executor.ExitError

// Executor wraps terraform subcommand execution for a single working
// directory. Must be constructed via New; the zero value panics on first use.
type Executor struct {
	workDir string
	varFile string

	exec   *executor.Executor
	logger *slog.Logger
}

// Option configures an Executor at construction time.
type Option func(*Executor)

// WithLogger sets the slog logger used to narrate terraform invocations.
func WithLogger(l *slog.Logger) Option {
	return func(e *Executor) { e.logger = logutil.OrNop(l) }
}

// WorkDir returns the working directory this Executor is rooted at.
func (t *Executor) WorkDir() string { return t.workDir }

// WithEnv appends environment variables to all terraform subprocess calls,
// applied after the allowlist-filtered parent env so they override same-key
// allowlist values; multiple calls are cumulative, with later entries winning.
func WithEnv(env []string) Option {
	return func(e *Executor) {
		e.exec.AppendEnv(env...)
	}
}

// PlanOptions configures a terraform plan invocation; OutputPlanFile and
// Destroy are independent and may be combined.
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

// ApplyOptions configures a terraform apply invocation. When PlanFile is
// set, Vars/VarFile/AutoApprove are silently ignored — the plan file already
// encodes the full change set.
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
	// UsePlan creates a destroy plan first, then applies it; plan failures
	// surface as errors, not a silent fallback to direct destroy.
	UsePlan bool
}

// New constructs an Executor rooted at workDir with the default var-file path
// (<workDir>/terraform.tfvars).
func New(workDir string, opts ...Option) *Executor {
	// SIGINT is terraform's documented soft-cancel (graceful abort + lock
	// release), unlike the executor's SIGTERM default.
	e := &Executor{
		workDir: workDir,
		varFile: filepath.Join(workDir, "terraform.tfvars"),
		exec:    executor.New(executor.WithWorkDir(workDir), executor.WithCancelSignal(syscall.SIGINT)),
		logger:  logutil.NopLogger,
	}
	for _, opt := range opts {
		opt(e)
	}
	executor.WithLogger(e.logger)(e.exec)
	return e
}

func (t *Executor) run(ctx context.Context, args ...string) error {
	t.logger.Info("terraform: running", "cmd", args[0])

	result, err := t.exec.Run(ctx, "terraform", args...)
	if err != nil {
		return fmt.Errorf("terraform %s: %w", args[0], err)
	}
	if result.ExitCode != 0 {
		return executor.NewExitError(ctx, "terraform "+args[0], result.ExitCode, result.Stderr)
	}
	return nil
}

// Init runs terraform init unless already initialized; a partial init (missing
// artifacts) triggers a re-init.
func (t *Executor) Init(ctx context.Context) error {
	stateFile := filepath.Join(t.workDir, "terraform.tfstate")
	if err := checkStateMajorVersion(stateFile, t.logger); err != nil {
		return err
	}

	terraformDir := filepath.Join(t.workDir, ".terraform")
	lockFile := filepath.Join(t.workDir, ".terraform.lock.hcl")
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
		vf = t.varFile
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

// LockHint returns a *errtypes.ConfigError naming the stale lock ID when
// Terraform's local-backend lock file is present, nil when absent. Callers
// must not auto-unlock — the operator must confirm no live process holds it first.
func (t *Executor) LockHint() error {
	lockFile := filepath.Join(t.workDir, ".terraform.tfstate.lock.info")
	if !system.FileExists(lockFile) {
		return nil
	}
	id := parseLockID(lockFile)
	if id == "" {
		id = "<id>"
	}
	return &errtypes.ConfigError{
		Msg: fmt.Sprintf(
			"terraform state locked at %s — run 'terraform force-unlock %s' in %s after confirming no other okdctl run is active",
			lockFile, id, t.workDir,
		),
	}
}

func parseLockID(lockFile string) string {
	raw, err := os.ReadFile(lockFile)
	if err != nil {
		return ""
	}
	var info struct {
		ID string `json:"ID"`
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		return ""
	}
	return info.ID
}

// WithLockHint attaches the LockHint diagnostic to err as next-step text,
// preserving err's concrete type so the exit code is unaffected. A
// non-errtypes err gets the hint via a plain %w wrap so no ConfigError enters the chain.
func (t *Executor) WithLockHint(err error) error {
	if err == nil {
		return nil
	}
	hint := t.LockHint()
	if hint == nil {
		return err
	}

	hintMsg := hint.Error()
	var cfgHint *errtypes.ConfigError
	if errors.As(hint, &cfgHint) {
		hintMsg = cfgHint.Msg
	}

	var appender errtypes.HintAppender
	if !errors.As(err, &appender) {
		return fmt.Errorf("%w; %s", err, hintMsg)
	}
	return appender.WithHint(hintMsg)
}

func (t *Executor) planArgs(opts PlanOptions) []string {
	args := []string{"plan", "-lock-timeout=" + defaultLockTimeout}
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
	return args
}

// Plan runs terraform plan with opts; non-destroy plans fail closed on a stale destroy override.
func (t *Executor) Plan(ctx context.Context, opts PlanOptions) error {
	if !opts.Destroy {
		if err := t.refuseStaleDestroyOverride(); err != nil {
			return err
		}
	}
	return t.run(ctx, t.planArgs(opts)...)
}

// PlanDetailed runs terraform plan -detailed-exitcode; exit 2 means changes
// are pending, exit 0 means none, and any other exit is a genuine failure.
func (t *Executor) PlanDetailed(ctx context.Context, opts PlanOptions) (bool, error) {
	if !opts.Destroy {
		if err := t.refuseStaleDestroyOverride(); err != nil {
			return false, err
		}
	}
	args := append(t.planArgs(opts), "-detailed-exitcode")
	t.logger.Info("terraform: running", "cmd", args[0])

	result, err := t.exec.Run(ctx, "terraform", args...)
	if err != nil {
		return false, fmt.Errorf("terraform %s: %w", args[0], err)
	}
	switch result.ExitCode {
	case 0:
		return false, nil
	case 2:
		return true, nil
	default:
		return false, executor.NewExitError(ctx, "terraform "+args[0], result.ExitCode, result.Stderr)
	}
}

// PlanStreamed runs terraform plan, streaming stdout/stderr directly to the
// terminal; use instead of Plan when the operator must see live output.
func (t *Executor) PlanStreamed(ctx context.Context, opts PlanOptions) error {
	if !opts.Destroy {
		if err := t.refuseStaleDestroyOverride(); err != nil {
			return err
		}
	}
	args := t.planArgs(opts)
	t.logger.Info("terraform: running plan (streaming to terminal)")
	return t.exec.RunInteractive(ctx, "terraform", args...)
}

// Apply runs terraform apply; when opts.PlanFile is set, Vars/VarFile/
// AutoApprove are ignored. Fails closed on a stale destroy override; only
// Destroy's internal apply bypasses the guard.
func (t *Executor) Apply(ctx context.Context, opts ApplyOptions) error {
	if err := t.refuseStaleDestroyOverride(); err != nil {
		return err
	}
	return t.apply(ctx, opts)
}

func (t *Executor) apply(ctx context.Context, opts ApplyOptions) error {
	args := []string{"apply", "-lock-timeout=" + defaultLockTimeout}

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

// Destroy runs terraform destroy; when opts.UsePlan is true, the plan is
// generated first so failures surface before any mutation.
func (t *Executor) Destroy(ctx context.Context, opts DestroyOptions) error {
	if opts.UsePlan {
		return t.destroyWithPlan(ctx, opts)
	}
	return t.destroyDirect(ctx, opts)
}

// destroyWithPlan runs plan -destroy then applies it; plan failures are never
// silently downgraded to direct destroy.
func (t *Executor) destroyWithPlan(ctx context.Context, opts DestroyOptions) error {
	planFile := filepath.Join(t.workDir, "destroy.tfplan")

	planErr := t.Plan(ctx, PlanOptions{
		VarFile:        opts.VarFile,
		OutputPlanFile: planFile,
		Destroy:        true,
		Targets:        opts.Targets,
	})

	if planErr != nil {
		return fmt.Errorf("destroy plan: %w (pass UsePlan=false to skip the plan step)", planErr)
	}

	// t.apply, not t.Apply: the destroy session legitimately bypasses the override guard.
	return t.apply(ctx, ApplyOptions{PlanFile: planFile})
}

// destroyDirect has no current caller; kept as an emergency-destroy path pinned
// by regression coverage.
func (t *Executor) destroyDirect(ctx context.Context, opts DestroyOptions) error {
	args := []string{"destroy", "-lock-timeout=" + defaultLockTimeout}
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

// StateStatusValue classifies the terraform.tfstate present in WorkDir.
type StateStatusValue string

const (
	// StateStatusMissing means terraform.tfstate does not exist.
	StateStatusMissing StateStatusValue = "missing"
	// StateStatusEmpty means the file exists but has no managed resources.
	StateStatusEmpty StateStatusValue = "empty"
	// StateStatusPopulated means at least one managed resource is present.
	StateStatusPopulated StateStatusValue = "populated"
	// StateStatusCorrupt means the file exists but cannot be read or parsed;
	// never treat this as already-destroyed.
	StateStatusCorrupt StateStatusValue = "corrupt"
)

// StateStatus classifies the terraform.tfstate in WorkDir, distinguishing
// corrupt files from empty/missing ones.
func (t *Executor) StateStatus() StateStatusValue {
	stateFile := filepath.Join(t.workDir, "terraform.tfstate")
	if !system.FileExists(stateFile) {
		return StateStatusMissing
	}
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.logger.Warn("terraform: cannot read state file", "path", stateFile, "err", err)
		return StateStatusCorrupt
	}
	var s struct {
		Resources []json.RawMessage `json:"resources"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		t.logger.Warn("terraform: state file is corrupt or unreadable; inspect before retrying destroy",
			"path", stateFile, "err", err)
		return StateStatusCorrupt
	}
	if len(s.Resources) == 0 {
		return StateStatusEmpty
	}
	return StateStatusPopulated
}

// HasState reports whether WorkDir's terraform.tfstate has at least one managed resource.
func (t *Executor) HasState() bool {
	return t.StateStatus() == StateStatusPopulated
}

// NewestBakSnapshot returns the most recent terraform.tfstate.*.bak path in
// WorkDir, or "" when none exist; timestamp-encoded names sort chronologically
// under os.ReadDir's name order.
func (t *Executor) NewestBakSnapshot() string {
	entries, err := os.ReadDir(t.workDir)
	if err != nil {
		return ""
	}
	latest := ""
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, "terraform.tfstate.") && strings.HasSuffix(n, ".bak") {
			latest = filepath.Join(t.workDir, n)
		}
	}
	return latest
}

// SnapshotState copies terraform.tfstate to terraform.tfstate.<timestamp>.bak
// before a destructive operation ("", nil when no state file exists). A
// write failure is a hard error; callers must not proceed without a saved backup.
func (t *Executor) SnapshotState(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	src := filepath.Join(t.workDir, "terraform.tfstate")
	if !system.FileExists(src) {
		return "", nil
	}
	// O_NOFOLLOW: pre-sudo symlink redirection guard — mirrors ignition.go:readNoFollow.
	if info, err := os.Lstat(src); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("terraform snapshot: state file %q is a symlink; refusing to follow", src)
	}
	f, err := os.OpenFile(src, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return "", fmt.Errorf("terraform snapshot: read state: %w", err)
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		return "", fmt.Errorf("terraform snapshot: read state: %w", err)
	}
	ts := strings.ReplaceAll(time.Now().UTC().Format(time.RFC3339), ":", "-")
	dst := filepath.Join(t.workDir, "terraform.tfstate."+ts+".bak")
	if err := system.AtomicWrite(dst, data, 0o600); err != nil {
		return "", fmt.Errorf("terraform snapshot: write %s: %w", dst, err)
	}
	t.pruneSnapshots()
	return dst, nil
}

// pruneSnapshots keeps the 5 most recent terraform.tfstate.*.bak files by
// os.ReadDir's chronological name order.
func (t *Executor) pruneSnapshots() {
	entries, err := os.ReadDir(t.workDir)
	if err != nil {
		t.logger.Warn("terraform: snapshot prune: cannot read workdir", "dir", t.workDir, "err", err)
		return
	}
	const retain = 5
	var snaps []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "terraform.tfstate.") && strings.HasSuffix(name, ".bak") {
			snaps = append(snaps, filepath.Join(t.workDir, name))
		}
	}
	if len(snaps) <= retain {
		return
	}
	for _, old := range snaps[:len(snaps)-retain] {
		if err := os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.logger.Warn("terraform: snapshot prune: remove failed", "path", old, "err", err)
		}
	}
}

// PruneBakSnapshotsExceptNewest removes every terraform.tfstate.*.bak file in
// WorkDir except the most recent, returning its path (or "" if none exist).
func (t *Executor) PruneBakSnapshotsExceptNewest() (string, error) {
	newest := t.NewestBakSnapshot()
	entries, err := os.ReadDir(t.workDir)
	if err != nil {
		return newest, fmt.Errorf("prune bak snapshots: read workdir: %w", err)
	}
	var errs []error
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "terraform.tfstate.") || !strings.HasSuffix(name, ".bak") {
			continue
		}
		path := filepath.Join(t.workDir, name)
		if path == newest {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("remove %s: %w", path, err))
		}
	}
	return newest, errors.Join(errs...)
}

// ZeroizeEnv delegates to the inner executor's ZeroizeEnv; call via defer after
// terraform operations complete.
func (t *Executor) ZeroizeEnv() {
	if t.exec == nil {
		return
	}
	t.exec.ZeroizeEnv()
}

// Output runs terraform output -json; each value remains JSON-encoded for
// callers to unmarshal individually.
func (t *Executor) Output(ctx context.Context) (map[string]json.RawMessage, error) {
	result, err := t.exec.RunOutputChecked(ctx, 0, "terraform", "output", "-json")
	if err != nil {
		return nil, fmt.Errorf("terraform output: %w", err)
	}
	if result.Truncated {
		return nil, fmt.Errorf("terraform output: output truncated after %d bytes", len(result.Stdout))
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		return nil, fmt.Errorf("terraform output: invalid json: %w", err)
	}
	return out, nil
}

// CleanupPlans removes tfplan and destroy.tfplan; terraform.tfstate.backup is
// left as the operator's rollback artefact.
func (t *Executor) CleanupPlans() error {
	var errs []error
	files := []string{
		filepath.Join(t.workDir, PlanFileName),
		filepath.Join(t.workDir, "destroy.tfplan"),
	}
	for _, f := range files {
		if err := system.SafeRemove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("remove %s: %w", f, err))
		}
	}
	return errors.Join(errs...)
}
