package deploy

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

type fakeProvisioner struct {
	guardOpts       []okd.SetupOpts
	setupCalls      int
	installCalls    int
	postCalls       int
	resumePostCalls int
}

func (f *fakeProvisioner) GuardSetup(_ *config.Config, opts okd.SetupOpts) error {
	f.guardOpts = append(f.guardOpts, opts)
	return nil
}

func (f *fakeProvisioner) Setup(context.Context, *config.Config, okd.SetupOpts) ([]distribution.StepResult, error) {
	f.setupCalls++
	return nil, nil
}

func (f *fakeProvisioner) Install(context.Context, *config.Config, *install.Options) ([]distribution.StepResult, error) {
	f.installCalls++
	return nil, nil
}

func (f *fakeProvisioner) PostInstall(context.Context, *config.Config, bool) (*postinstall.Result, []distribution.StepResult, error) {
	f.postCalls++
	return &postinstall.Result{}, nil, nil
}

func (f *fakeProvisioner) ResumePostInstall(context.Context, *config.Config, bool) (*postinstall.Result, []distribution.StepResult, error) {
	f.resumePostCalls++
	return &postinstall.Result{}, nil, nil
}

type failingProvisioner struct {
	fakeProvisioner
	installSteps []distribution.StepResult
	installErr   error
}

func (f *failingProvisioner) Install(context.Context, *config.Config, *install.Options) ([]distribution.StepResult, error) {
	return f.installSteps, f.installErr
}

func TestRunDeployPhases_FailureSummaryResumeFirst(t *testing.T) {
	dir := t.TempDir()
	markerPath := filepath.Join(dir, StateFileName)
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "prod"

	f := &failingProvisioner{
		installSteps: []distribution.StepResult{
			{StepID: "deploy-infrastructure", Duration: 90 * time.Second, Error: errors.New("terraform apply failed")},
		},
		installErr: errors.New("terraform apply failed"),
	}
	var buf bytes.Buffer
	resumeFrom, marker := resolveResumePhase(markerPath, cfg.Cluster.Name, false)
	_, _, err := runDeployPhases(context.Background(), f, cfg, dir, markerPath, "run-77", resumeFrom, marker, false, false, time.Now(), presenter{w: &buf, enabled: true})
	if err == nil {
		t.Fatal("expected install error to propagate; got nil")
	}

	out := buf.String()
	for _, want := range []string{
		"deploy failed",
		"run-77",
		"failed phase",
		"install",
		"failed step",
		"deploy-infrastructure",
		"elapsed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("failure summary missing %q:\n%s", want, out)
		}
	}

	resume := strings.Index(out, "to resume from install")
	fresh := strings.Index(out, "--fresh")
	destroy := strings.Index(out, "okdctl destroy")
	if resume < 0 || fresh < 0 || destroy < 0 {
		t.Fatalf("epilogue missing resume(%d)/fresh(%d)/destroy(%d) lines:\n%s", resume, fresh, destroy, out)
	}
	if resume > fresh || fresh > destroy {
		t.Errorf("epilogue order resume@%d fresh@%d destroy@%d; want resume first, destroy last:\n%s", resume, fresh, destroy, out)
	}
}

func TestRunDeployPhases_ResumeRouting(t *testing.T) {
	guardResume := func(v bool) *bool { return &v }
	tests := []struct {
		name        string
		markerPhase deployPhase // "" = no marker on disk
		// wantGuardResume nil skips the guard entirely; otherwise pins ResumeInProgress.
		wantGuardResume *bool
		wantSetup       int
		wantInstall     int
		wantPost        int
		wantResumePost  int
	}{
		{
			name:            "no marker runs all phases through the guard",
			wantGuardResume: guardResume(false),
			wantSetup:       1, wantInstall: 1, wantPost: 1,
		},
		{
			name:            "setup marker resumes through guard and wipe",
			markerPhase:     phaseSetup,
			wantGuardResume: guardResume(true),
			wantSetup:       1, wantInstall: 1, wantPost: 1,
		},
		{
			name:        "install marker never touches guard or setup",
			markerPhase: phaseInstall,
			wantInstall: 1, wantPost: 1,
		},
		{
			name:           "postinstall marker runs resume-postinstall only",
			markerPhase:    phasePostInstall,
			wantResumePost: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			markerPath := filepath.Join(dir, StateFileName)
			if tc.markerPhase != "" {
				if err := writeDeployState(markerPath, tc.markerPhase, "old-run", "prod"); err != nil {
					t.Fatalf("writeDeployState: %v", err)
				}
			}
			cfg := config.DefaultConfig()
			cfg.Cluster.Name = "prod"

			f := &fakeProvisioner{}
			var buf bytes.Buffer
			resumeFrom, marker := resolveResumePhase(markerPath, cfg.Cluster.Name, false)
			if _, _, err := runDeployPhases(context.Background(), f, cfg, dir, markerPath, "new-run", resumeFrom, marker, false, false, time.Now(), presenter{w: &buf, enabled: true}); err != nil {
				t.Fatalf("runDeployPhases: %v", err)
			}

			if tc.wantGuardResume == nil {
				if len(f.guardOpts) != 0 {
					t.Errorf("guard consulted %d times; want 0", len(f.guardOpts))
				}
			} else {
				if len(f.guardOpts) != 1 {
					t.Fatalf("guard consulted %d times; want 1", len(f.guardOpts))
				}
				if got := f.guardOpts[0].ResumeInProgress; got != *tc.wantGuardResume {
					t.Errorf("guard ResumeInProgress = %v; want %v", got, *tc.wantGuardResume)
				}
			}
			if f.setupCalls != tc.wantSetup {
				t.Errorf("setup calls = %d; want %d", f.setupCalls, tc.wantSetup)
			}
			if f.installCalls != tc.wantInstall {
				t.Errorf("install calls = %d; want %d", f.installCalls, tc.wantInstall)
			}
			if f.postCalls != tc.wantPost {
				t.Errorf("postinstall calls = %d; want %d", f.postCalls, tc.wantPost)
			}
			if f.resumePostCalls != tc.wantResumePost {
				t.Errorf("resume-postinstall calls = %d; want %d", f.resumePostCalls, tc.wantResumePost)
			}
		})
	}
}

// checklistRecorder must stay a true nil (not a wrapped concrete pointer)
// when progress rendering is off, and checklistPrefix must not dereference it.
func TestChecklistRecorder_NilWhenProgressOff(t *testing.T) {
	prev := logutil.ProgressBarsEnabled()
	logutil.SetProgressBarsEnabled(false)
	t.Cleanup(func() { logutil.SetProgressBarsEnabled(prev) })

	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "prod"

	rec := checklistRecorder(cfg, t.TempDir(), phaseSetup, nil)
	if rec != nil {
		t.Fatalf("checklistRecorder = %v, want nil when progress bars are off", rec)
	}
	if got := checklistPrefix(rec); got != "" {
		t.Errorf("checklistPrefix(nil) = %q, want empty", got)
	}
}

// TestPresenterDisabledLeavesBoxAndPresentationToTheCaller pins the seam a
// TUI-driven run relies on: no box on w, and the raw error, so the wizard can
// render in-frame and the top-level handler still reports once it exits.
func TestPresenterDisabledLeavesBoxAndPresentationToTheCaller(t *testing.T) {
	boom := errors.New("terraform apply failed")
	steps := []distribution.StepResult{{StepID: "deploy-infrastructure", Duration: time.Second, Error: boom}}

	var quiet bytes.Buffer
	got := presenter{w: &quiet, enabled: false}.fail(boom, phaseInstall, steps, "run-1", time.Now(), "cancel hint")
	if quiet.Len() != 0 {
		t.Errorf("a TUI-driven run must write no box; got %q", quiet.String())
	}
	if !errors.Is(got, boom) {
		t.Errorf("fail() = %v, want the engine error unchanged", got)
	}

	var loud bytes.Buffer
	got = presenter{w: &loud, enabled: true}.fail(boom, phaseInstall, steps, "run-1", time.Now(), "cancel hint")
	if !strings.Contains(loud.String(), "deploy failed") {
		t.Errorf("a plain run must print the failure box; got %q", loud.String())
	}
	if !strings.Contains(got.Error(), boom.Error()) {
		t.Errorf("fail() = %v, want it to still carry the engine error", got)
	}
}

func TestPlannedStepsCoversEveryPhaseOnAFreshRun(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "prod"

	plan := PlannedSteps(cfg, t.TempDir(), false)
	if len(plan) == 0 {
		t.Fatal("a fresh run must plan every registered step")
	}

	seen := map[string]bool{}
	for _, s := range plan {
		seen[s.Phase] = true
		if s.ID == "" || s.Name == "" {
			t.Errorf("planned step %+v is missing an id or name", s)
		}
	}
	for _, phase := range []okd.DeployPhase{okd.PhaseSetup, okd.PhaseInstall, okd.PhasePostInstall} {
		if !seen[string(phase)] {
			t.Errorf("fresh plan is missing the %s phase", phase)
		}
	}
}

// TestPlannedStepsDropsPhasesAResumeSkips proves the wizard checklist's total
// matches what a resumed run actually executes.
func TestPlannedStepsDropsPhasesAResumeSkips(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "prod"

	workDir := workspace.WorkDir(dir)
	if err := os.MkdirAll(workDir, 0o750); err != nil {
		t.Fatalf("seed work dir: %v", err)
	}
	if err := markDeployPhaseFatal(filepath.Join(workDir, StateFileName), phasePostInstall, "run-1", cfg.Cluster.Name); err != nil {
		t.Fatalf("seed marker: %v", err)
	}

	plan := PlannedSteps(cfg, dir, false)
	if len(plan) == 0 {
		t.Fatal("a postinstall resume must still plan its own phase")
	}
	for _, s := range plan {
		if s.Phase != string(okd.PhasePostInstall) {
			t.Fatalf("a postinstall resume planned a %s step (%s)", s.Phase, s.ID)
		}
	}

	if fresh := PlannedSteps(cfg, dir, true); len(fresh) <= len(plan) {
		t.Errorf("--fresh planned %d steps, want more than the resume's %d", len(fresh), len(plan))
	}
}
