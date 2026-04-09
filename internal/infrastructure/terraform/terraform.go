// Package terraform provides a high-level interface for Terraform operations.
// It can be used by any infrastructure provider that uses Terraform.
package terraform

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// PlanFileName is the default plan file name used by Plan, Apply, and Cleanup.
const PlanFileName = "tfplan"

type ExecError struct {
	Command  string
	ExitCode int
	Stderr   string
}

func (e *ExecError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("terraform %s failed (exit code %d): %s", e.Command, e.ExitCode, e.Stderr)
	}
	return fmt.Sprintf("terraform %s failed with exit code %d", e.Command, e.ExitCode)
}

type Executor struct {
	WorkDir string
	VarFile string
	Verbose bool

	exec   *executor.Executor
	logger *slog.Logger
}

type Option func(*Executor)

func WithLogger(l *slog.Logger) Option {
	return func(e *Executor) {
		if l != nil {
			e.logger = l
		}
	}
}

func WithVerbose(v bool) Option {
	return func(e *Executor) {
		e.Verbose = v
	}
}

// WithEnv appends environment variables to be passed to all terraform subprocess calls.
// At execution time they are appended after os.Environ(), so entries here override
// identically-named variables from the inherited environment. Multiple calls to
// WithEnv are cumulative; later entries for the same key win.
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

type DestroyOptions struct {
	// VarFile overrides the default terraform.tfvars path.
	VarFile     string
	AutoApprove bool
	Parallelism int
	// UsePlan creates a destroy plan first, then applies it.
	// Safer because it previews changes; falls back to direct destroy on failure.
	UsePlan bool
}

func New(workDir string, opts ...Option) *Executor {
	e := &Executor{
		WorkDir: workDir,
		VarFile: filepath.Join(workDir, "terraform.tfvars"),
		exec:    executor.New(executor.WithWorkDir(workDir)),
		logger:  slog.New(slog.DiscardHandler),
	}
	for _, opt := range opts {
		opt(e)
	}
	executor.WithLogger(e.logger)(e.exec)
	return e
}

func NewWithVarFile(workDir, varFile string, opts ...Option) *Executor {
	e := &Executor{
		WorkDir: workDir,
		VarFile: varFile,
		exec:    executor.New(executor.WithWorkDir(workDir)),
		logger:  slog.New(slog.DiscardHandler),
	}
	for _, opt := range opts {
		opt(e)
	}
	executor.WithLogger(e.logger)(e.exec)
	return e
}

func (t *Executor) run(ctx context.Context, args ...string) error {
	t.exec.Verbose = t.Verbose

	t.logger.Info(fmt.Sprintf("terraform: running %s", args[0]))

	result, err := t.exec.Run(ctx, "terraform", args...)
	if err != nil {
		return fmt.Errorf("terraform %s failed: %w", args[0], err)
	}
	if result.ExitCode != 0 {
		return &ExecError{
			Command:  args[0],
			ExitCode: result.ExitCode,
			Stderr:   result.Stderr,
		}
	}
	return nil
}

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
			t.logger.Warn(fmt.Sprintf("terraform: var file %s not found, skipping", vf))
		}
	}

	if len(vars) > 0 {
		keys := make([]string, 0, len(vars))
		for k := range vars {
			keys = append(keys, k)
		}
		slices.Sort(keys)

		for _, k := range keys {
			args = append(args, "-var", fmt.Sprintf("%s=%s", k, vars[k]))
		}
	}

	return args
}

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

func (t *Executor) Destroy(ctx context.Context, opts DestroyOptions) error {
	if opts.UsePlan {
		return t.destroyWithPlan(ctx, opts)
	}
	return t.destroyDirect(ctx, opts)
}

// destroyWithPlan falls back to direct destroy if plan creation fails.
func (t *Executor) destroyWithPlan(ctx context.Context, opts DestroyOptions) error {
	planFile := filepath.Join(t.WorkDir, "destroy.tfplan")

	planErr := t.Plan(ctx, PlanOptions{
		VarFile:        opts.VarFile,
		OutputPlanFile: planFile,
		Destroy:        true,
	})

	if planErr != nil {
		t.logger.Warn("terraform: destroy plan failed, falling back to direct destroy")
		return t.destroyDirect(ctx, opts)
	}

	applyOpts := ApplyOptions{
		PlanFile: planFile,
	}

	if err := t.Apply(ctx, applyOpts); err != nil {
		return err
	}

	return nil
}

func (t *Executor) destroyDirect(ctx context.Context, opts DestroyOptions) error {
	args := []string{"destroy"}
	args = append(args, t.buildVarArgs(opts.VarFile, nil)...)

	if opts.AutoApprove {
		args = append(args, "-auto-approve")
	}
	if opts.Parallelism > 0 {
		args = append(args, fmt.Sprintf("-parallelism=%d", opts.Parallelism))
	}

	return t.run(ctx, args...)
}

func (t *Executor) Output(ctx context.Context, name string) (string, error) {
	result, err := t.exec.Run(ctx, "terraform", "output", "-raw", name)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("terraform output %s: %s", name, result.Stderr)
	}
	return result.Stdout, nil
}

func (t *Executor) HasState() bool {
	stateFile := filepath.Join(t.WorkDir, "terraform.tfstate")
	return system.FileExists(stateFile)
}

func (t *Executor) StateFile() string {
	return filepath.Join(t.WorkDir, "terraform.tfstate")
}

// Cleanup returns an aggregated error if any removal fails (non-existent files are ignored).
func (t *Executor) Cleanup() error {
	var errs []error
	files := []string{
		filepath.Join(t.WorkDir, PlanFileName),
		filepath.Join(t.WorkDir, "destroy.tfplan"),
		filepath.Join(t.WorkDir, "terraform.tfstate.backup"),
	}
	for _, f := range files {
		if err := system.SafeRemove(f); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("failed to remove %s: %w", f, err))
		}
	}
	return errors.Join(errs...)
}

func (t *Executor) GetWorkDir() string {
	return t.WorkDir
}
