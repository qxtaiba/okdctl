// Package terraform provides a high-level interface for Terraform operations.
// It can be used by any infrastructure provider that uses Terraform.
package terraform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

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
	logger utils.Logger
}

type Option func(*Executor)

func WithLogger(l utils.Logger) Option {
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

// WithEnv propagates environment variables to all terraform subprocess calls.
func WithEnv(env []string) Option {
	return func(e *Executor) {
		e.exec.Env = append(e.exec.Env, env...)
	}
}

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
		logger:  utils.NoopLogger(),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

func NewWithVarFile(workDir, varFile string, opts ...Option) *Executor {
	e := &Executor{
		WorkDir: workDir,
		VarFile: varFile,
		exec:    executor.New(executor.WithWorkDir(workDir)),
		logger:  utils.NoopLogger(),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

func (t *Executor) run(ctx context.Context, args ...string) error {
	t.exec.Verbose = t.Verbose

	t.logger.Info(fmt.Sprintf("terraform: running %s", args[0]))

	result, err := t.exec.Run(ctx, "terraform", args...)
	if err != nil {
		return utils.WrapErrorf(err, "terraform %s failed", args[0])
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

	if system.DirExists(terraformDir) && system.FileExists(lockFile) && system.DirExists(providersDir) {
		t.logger.Info("terraform: already initialized")
		return nil
	}

	return t.run(ctx, "init")
}

// EnsureInitialized operates on an arbitrary directory, not necessarily the executor's WorkDir.
func EnsureInitialized(ctx context.Context, workDir string, verbose bool) error {
	terraformCache := filepath.Join(workDir, ".terraform")
	lockFile := filepath.Join(workDir, ".terraform.lock.hcl")
	providersDir := filepath.Join(terraformCache, "providers")

	if system.DirExists(terraformCache) && system.FileExists(lockFile) && system.DirExists(providersDir) {
		return nil
	}

	tempExec := New(workDir)
	tempExec.Verbose = verbose
	return tempExec.Init(ctx)
}

func (t *Executor) buildVarArgs(varFile string, vars map[string]string) []string {
	var args []string

	vf := varFile
	if vf == "" {
		vf = t.VarFile
	}
	if system.FileExists(vf) {
		args = append(args, "-var-file="+vf)
	}

	if len(vars) > 0 {
		keys := make([]string, 0, len(vars))
		for k := range vars {
			keys = append(keys, k)
		}
		sort.Strings(keys)

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

	if opts.PlanFile != "" && system.FileExists(opts.PlanFile) {
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
		filepath.Join(t.WorkDir, "tfplan"),
		filepath.Join(t.WorkDir, "destroy.tfplan"),
		filepath.Join(t.WorkDir, "terraform.tfstate.backup"),
	}
	for _, f := range files {
		if err := system.SafeRemove(f); err != nil && !os.IsNotExist(err) {
			errs = append(errs, utils.WrapErrorf(err, "failed to remove %s", f))
		}
	}
	return errors.Join(errs...)
}

// Version is used for health checks and compatibility verification.
func (t *Executor) Version(ctx context.Context) error {
	result, err := t.exec.Run(ctx, "terraform", "version")
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("terraform version: exit code %d", result.ExitCode)
	}
	return nil
}

func (t *Executor) GetVersion(ctx context.Context) (string, error) {
	result, err := t.exec.Run(ctx, "terraform", "version", "-json")
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("terraform version: %s", result.Stderr)
	}
	return result.Stdout, nil
}

func (t *Executor) GetWorkDir() string {
	return t.WorkDir
}
