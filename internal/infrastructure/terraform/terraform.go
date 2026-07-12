// Package terraform provides a high-level interface for Terraform operations.
// Subcommands run via internal/executor with an env allowlist; state snapshots
// are written atomically via system.AtomicWrite. Executor exposes Init, Plan,
// Apply, Destroy, and Output as the primary call surface.
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

// PlanFileName is the default plan file name used by Plan, Apply, and Cleanup.
const PlanFileName = "tfplan"

// defaultLockTimeout is passed as -lock-timeout to every state-locking
// terraform subcommand so a stale lock from a SIGKILL-ed prior run waits
// then fails with a clean diagnostic instead of failing immediately.
const defaultLockTimeout = "120s"

// ExecError is a type alias for executor.ExitError. Because it is a true
// alias (not a defined type), errors.As(&terraform.ExecError{}) and
// errors.As(&executor.ExitError{}) are equivalent — callers may use either
// target interchangeably without a double unwrap.
type ExecError = executor.ExitError

// Executor wraps terraform subcommand execution for a single working
// directory. varFile is the default var-file path; per-invocation
// overrides come via the VarFile field on Plan/Apply/DestroyOptions.
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

// WithEnv appends environment variables to be passed to all terraform subprocess calls.
// At execution time they are appended after the executor's allowlist-filtered
// parent env, so entries here override allowlist values for the same key.
// Multiple calls to WithEnv are cumulative; later entries for the same key win.
func WithEnv(env []string) Option {
	return func(e *Executor) {
		e.exec.AppendEnv(env...)
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
	// SIGINT is terraform's documented soft-cancel: it triggers a graceful
	// plan/apply abort and releases the state lock before exit, unlike the
	// executor package's SIGTERM default.
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
		return fmt.Errorf("terraform %s failed: %w", args[0], err)
	}
	if result.ExitCode != 0 {
		return executor.NewExitError(ctx, "terraform "+args[0], result.ExitCode, result.Stderr)
	}
	return nil
}

// Init runs "terraform init" when the working directory is not already
// initialized. A partial init (some artifacts missing) triggers a re-init.
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

// LockHint returns a *errtypes.ConfigError when the Terraform local-backend
// lock file (.terraform.tfstate.lock.info) is present in WorkDir, indicating
// a stale lock from a prior crashed run. Returns nil when absent. Callers
// must not auto-unlock — the message names the lock ID so the operator can
// run terraform force-unlock after confirming no live process holds it.
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

// WithLockHint appends the terraform state-lock diagnostic (see LockHint) to
// err's message when a stale local-backend lock is present, preserving err's
// concrete type so a terraform failure exits with the same code whether or
// not a stale lock is involved — a bare errors.Join would let exitCodeFor
// match the hint's *errtypes.ConfigError before err's own type (see
// errtypes.HintAppender). Returns err unchanged when err is nil or no lock
// file is present, and falls back to errors.Join when err's type does not
// implement HintAppender.
func (t *Executor) WithLockHint(err error) error {
	if err == nil {
		return nil
	}
	hint := t.LockHint()
	if hint == nil {
		return err
	}

	var appender errtypes.HintAppender
	if !errors.As(err, &appender) {
		return errors.Join(hint, err)
	}

	hintMsg := hint.Error()
	var cfgHint *errtypes.ConfigError
	if errors.As(hint, &cfgHint) {
		hintMsg = cfgHint.Msg
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

// Plan runs "terraform plan" with the options in opts. When Destroy is true
// the plan is a destruction plan.
func (t *Executor) Plan(ctx context.Context, opts PlanOptions) error {
	return t.run(ctx, t.planArgs(opts)...)
}

// PlanStreamed runs "terraform plan" streaming stdout and stderr directly to the
// terminal. Use instead of Plan when the operator must see the plan output —
// Plan captures into internal buffers and only surfaces stderr on failure.
func (t *Executor) PlanStreamed(ctx context.Context, opts PlanOptions) error {
	args := t.planArgs(opts)
	t.logger.Info("terraform: running plan (streaming to terminal)")
	return t.exec.RunInteractive(ctx, "terraform", args...)
}

// Apply runs "terraform apply". When opts.PlanFile is set, Vars, VarFile,
// and AutoApprove are ignored — the plan file encodes the full change set.
func (t *Executor) Apply(ctx context.Context, opts ApplyOptions) error {
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
	planFile := filepath.Join(t.workDir, "destroy.tfplan")

	planErr := t.Plan(ctx, PlanOptions{
		VarFile:        opts.VarFile,
		OutputPlanFile: planFile,
		Destroy:        true,
		Targets:        opts.Targets,
	})

	if planErr != nil {
		return fmt.Errorf("terraform destroy plan failed: %w (re-run with an explicit fix or pass UsePlan=false to skip the plan step)", planErr)
	}

	return t.Apply(ctx, ApplyOptions{PlanFile: planFile})
}

// destroyDirect runs terraform destroy without an intermediate plan file.
// Currently no caller — Destroy is always invoked with UsePlan=true today —
// but kept as an emergency-destroy path so the argv shape (parallelism,
// -target injection) stays pinned by regression coverage if an opt-in
// caller lands later.
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
	// StateStatusCorrupt means the file exists but cannot be read or parsed.
	// Callers must not treat this as "already destroyed" — surface a
	// recovery error instead.
	StateStatusCorrupt StateStatusValue = "corrupt"
)

// StateStatus classifies the terraform.tfstate in WorkDir. It distinguishes
// corrupt files from genuinely empty or missing ones so callers can surface
// actionable diagnostics instead of treating corruption as "already destroyed".
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

// HasState reports whether WorkDir contains a terraform.tfstate with at least
// one managed resource. Returns false for missing, empty, or corrupt state.
func (t *Executor) HasState() bool {
	return t.StateStatus() == StateStatusPopulated
}

// NewestBakSnapshot returns the absolute path of the most recent
// terraform.tfstate.*.bak file in WorkDir, or "" when none are present.
// os.ReadDir entries are name-sorted; timestamp-encoded names are therefore
// chronologically ordered so the last matching entry is the newest.
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
// in WorkDir immediately before a destructive operation. Returns the snapshot
// path so callers can include it in error messages. Returns ("", nil) when no
// state file is present — callers treat an empty path as "nothing to snapshot".
// A write failure is a hard error; callers must not proceed with the
// destructive operation without a saved backup.
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

// pruneSnapshots removes older terraform.tfstate.*.bak files from WorkDir,
// keeping only the 5 most recent. os.ReadDir returns entries sorted by name;
// because names encode UTC timestamps (lexicographic == chronological), no
// additional sort step is needed.
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
// Mirrors CleanupPlans' policy of keeping one rollback artefact rather than
// deleting every snapshot outright.
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

// ZeroizeEnv delegates to the inner executor's ZeroizeEnv, bounding the
// lifetime of plaintext credential strings in process memory. Call via defer
// after all terraform operations complete.
func (t *Executor) ZeroizeEnv() {
	if t.exec == nil {
		return
	}
	t.exec.ZeroizeEnv()
}

// Output runs "terraform output -json" and returns the decoded top-level
// map. Each value remains JSON-encoded; callers unmarshal individual entries.
func (t *Executor) Output(ctx context.Context) (map[string]json.RawMessage, error) {
	result, err := t.exec.RunOutputChecked(ctx, 0, "terraform", "output", "-json")
	if err != nil {
		return nil, fmt.Errorf("terraform output failed: %w", err)
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

// CleanupPlans removes tfplan and destroy.tfplan; non-existent files are
// ignored. terraform.tfstate.backup is intentionally left so the operator
// retains a rollback artefact if the live tfstate is later corrupted.
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
