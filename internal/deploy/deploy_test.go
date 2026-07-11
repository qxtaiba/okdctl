package deploy

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
)

type fakeProvisioner struct {
	guardOpts       []okd.PrepareOpts
	prepareCalls    int
	installCalls    int
	configureCalls  int
	resumeConfCalls int
}

func (f *fakeProvisioner) GuardPrepare(_ *config.Config, opts okd.PrepareOpts) error {
	f.guardOpts = append(f.guardOpts, opts)
	return nil
}

func (f *fakeProvisioner) Prepare(context.Context, *config.Config, okd.PrepareOpts) ([]distribution.StepResult, error) {
	f.prepareCalls++
	return nil, nil
}

func (f *fakeProvisioner) Install(context.Context, *config.Config, *install.Options) ([]distribution.StepResult, error) {
	f.installCalls++
	return nil, nil
}

func (f *fakeProvisioner) Configure(context.Context, *config.Config) (*postinstall.Result, []distribution.StepResult, error) {
	f.configureCalls++
	return &postinstall.Result{}, nil, nil
}

func (f *fakeProvisioner) ResumeConfigure(context.Context, *config.Config) (*postinstall.Result, []distribution.StepResult, error) {
	f.resumeConfCalls++
	return &postinstall.Result{}, nil, nil
}

func TestRunDeployPhases_ResumeRouting(t *testing.T) {
	guardResume := func(v bool) *bool { return &v }
	tests := []struct {
		name        string
		markerPhase deployPhase // "" = no marker on disk
		// wantGuardResume nil means the guard (and prepare) must never be
		// consulted; otherwise it pins the ResumeInProgress value passed.
		wantGuardResume *bool
		wantPrepare     int
		wantInstall     int
		wantConfigure   int
		wantResumeConf  int
	}{
		{
			name:            "no marker runs all phases through the guard",
			wantGuardResume: guardResume(false),
			wantPrepare:     1, wantInstall: 1, wantConfigure: 1,
		},
		{
			name:            "prepare marker resumes through guard and wipe",
			markerPhase:     phasePrepare,
			wantGuardResume: guardResume(true),
			wantPrepare:     1, wantInstall: 1, wantConfigure: 1,
		},
		{
			name:        "install marker never touches guard or prepare",
			markerPhase: phaseInstall,
			wantInstall: 1, wantConfigure: 1,
		},
		{
			name:           "configure marker runs resume-configure only",
			markerPhase:    phaseConfigure,
			wantResumeConf: 1,
		},
		{
			name:            "unknown marker phase does not bypass the guard",
			markerPhase:     deployPhase("someday"),
			wantGuardResume: guardResume(false),
			wantPrepare:     1, wantInstall: 1, wantConfigure: 1,
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
			if _, _, err := runDeployPhases(context.Background(), f, cfg, dir, markerPath, "new-run", false, &buf); err != nil {
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
			if f.prepareCalls != tc.wantPrepare {
				t.Errorf("prepare calls = %d; want %d", f.prepareCalls, tc.wantPrepare)
			}
			if f.installCalls != tc.wantInstall {
				t.Errorf("install calls = %d; want %d", f.installCalls, tc.wantInstall)
			}
			if f.configureCalls != tc.wantConfigure {
				t.Errorf("configure calls = %d; want %d", f.configureCalls, tc.wantConfigure)
			}
			if f.resumeConfCalls != tc.wantResumeConf {
				t.Errorf("resume-configure calls = %d; want %d", f.resumeConfCalls, tc.wantResumeConf)
			}
		})
	}
}
