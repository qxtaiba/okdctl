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

func (f *fakeProvisioner) PostInstall(context.Context, *config.Config) (*postinstall.Result, []distribution.StepResult, error) {
	f.postCalls++
	return &postinstall.Result{}, nil, nil
}

func (f *fakeProvisioner) ResumePostInstall(context.Context, *config.Config) (*postinstall.Result, []distribution.StepResult, error) {
	f.resumePostCalls++
	return &postinstall.Result{}, nil, nil
}

func TestRunDeployPhases_ResumeRouting(t *testing.T) {
	guardResume := func(v bool) *bool { return &v }
	tests := []struct {
		name        string
		markerPhase deployPhase // "" = no marker on disk
		// wantGuardResume nil means the guard (and setup) must never be
		// consulted; otherwise it pins the ResumeInProgress value passed.
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
		{
			name:            "unknown marker phase does not bypass the guard",
			markerPhase:     deployPhase("someday"),
			wantGuardResume: guardResume(false),
			wantSetup:       1, wantInstall: 1, wantPost: 1,
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
