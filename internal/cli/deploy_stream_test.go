package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/deploy"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/deployexec"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

func TestDeployNoTUIFlagIsLongFormOnly(t *testing.T) {
	f := deployCmd.Flags().Lookup(flagNoTUI)
	if f == nil {
		t.Fatalf("--%s must be registered on deploy", flagNoTUI)
	}
	if f.Shorthand != "" {
		t.Errorf("--%s has shorthand %q, want none (shorthand allowlist is closed)", flagNoTUI, f.Shorthand)
	}
	if len(f.Usage) > 120 {
		t.Errorf("--%s usage is %d chars, want <= 120", flagNoTUI, len(f.Usage))
	}
	if strings.ContainsAny(f.Usage, "\n") {
		t.Errorf("--%s usage must be one line: %q", flagNoTUI, f.Usage)
	}
}

func TestDeployFollowOnHooksBuildTheHubFlows(t *testing.T) {
	t.Setenv(wizardDemoEnv, "1")
	cfg := demoConfig()
	slot := &lifecycleSlot{}
	hooks := deployFollowOnHooks(deployCmd, cfg, slot)
	if hooks.Finish == nil || hooks.Finish.ManageNodes == nil || hooks.Finish.OpenConsole == nil {
		t.Fatal("successful deploy must expose manage and console actions")
	}

	manage, _, err := hooks.Finish.ManageNodes()
	if err != nil || len(manage) == 0 {
		t.Fatalf("manage flow = %d steps, %v", len(manage), err)
	}
	if sess := slot.take(); sess != nil {
		sess.close()
	}
}

func TestBrowserCommandUsesThePlatformOpener(t *testing.T) {
	for _, tc := range []struct {
		goos string
		name string
		args []string
		fail bool
	}{
		{goos: "darwin", name: "open", args: []string{"https://example.test"}},
		{goos: "dragonfly", name: "open", args: []string{"https://example.test"}},
		{goos: "freebsd", name: "open", args: []string{"https://example.test"}},
		{goos: "netbsd", name: "open", args: []string{"https://example.test"}},
		{goos: "openbsd", name: "open", args: []string{"https://example.test"}},
		{goos: "android", name: "xdg-open", args: []string{"https://example.test"}},
		{goos: "illumos", name: "xdg-open", args: []string{"https://example.test"}},
		{goos: "linux", name: "xdg-open", args: []string{"https://example.test"}},
		{goos: "solaris", name: "xdg-open", args: []string{"https://example.test"}},
		{goos: "windows", name: "rundll32", args: []string{"url.dll,FileProtocolHandler", "https://example.test"}},
		{goos: "plan9", fail: true},
	} {
		name, args, err := browserCommand(tc.goos, "https://example.test")
		if (err != nil) != tc.fail {
			t.Fatalf("browserCommand(%q) error = %v, want failure %t", tc.goos, err, tc.fail)
		}
		if !tc.fail && (name != tc.name || !slices.Equal(args, tc.args)) {
			t.Errorf("browserCommand(%q) = %q %q, want %q %q", tc.goos, name, args, tc.name, tc.args)
		}
	}
}

// TestDeployStreamDisabledByNoTUI proves the opt-out never consults the
// terminal at all, so a scripted run can force the plain checklist on a TTY.
func TestDeployStreamDisabledByNoTUI(t *testing.T) {
	t.Cleanup(func() { deployNoTUI = false })
	deployNoTUI = true
	if deployStreamEnabled() {
		t.Error("--no-tui must opt out of the stream screen")
	}
}

// TestDeployStreamDisabledWithoutATerminal pins the non-TTY contract: under go
// test neither stdio end is a terminal, so the gate must stay closed and the
// plain checklist path run byte-for-byte as before.
func TestDeployStreamDisabledWithoutATerminal(t *testing.T) {
	t.Cleanup(func() { deployNoTUI = false })
	deployNoTUI = false
	if deployStreamEnabled() {
		t.Error("the stream screen must never start without a terminal on both ends")
	}
}

func TestReportDeployStreamOutcome(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "homelab"
	cfg.Cluster.Domain = "lab.example.com"
	boom := errors.New("terraform apply failed")

	cases := []struct {
		name      string
		st        *deployexec.State
		wantErrIs error
		wantMsg   string
		wantOut   string
	}{
		{
			name:    "interrupted mid-install exits non-zero",
			st:      &deployexec.State{Cfg: cfg, Started: true},
			wantMsg: "interrupted mid-install",
		},
		{
			name:      "engine failure propagates unchanged",
			st:        &deployexec.State{Cfg: cfg, Started: true, Executed: true, Result: boom},
			wantErrIs: boom,
		},
		{
			name:    "success prints the recap",
			st:      &deployexec.State{Cfg: cfg, Started: true, Executed: true, Elapsed: 90 * time.Second},
			wantOut: "cluster deployed · homelab.lab.example.com · 1m30s",
		},
		{
			name: "a run that never started changes nothing",
			st:   &deployexec.State{Cfg: cfg},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := reportDeployStreamOutcome(&out, tc.st)

			switch {
			case tc.wantErrIs != nil:
				if !errors.Is(err, tc.wantErrIs) {
					t.Fatalf("err = %v, want %v", err, tc.wantErrIs)
				}
			case tc.wantMsg != "":
				var clusterErr *errtypes.ClusterError
				if !errors.As(err, &clusterErr) {
					t.Fatalf("err = %v, want a ClusterError", err)
				}
				if !strings.Contains(clusterErr.Error(), tc.wantMsg) {
					t.Errorf("err = %q, want it to mention %q", clusterErr.Error(), tc.wantMsg)
				}
			default:
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
			}

			if tc.wantOut == "" {
				if out.Len() != 0 {
					t.Errorf("printed %q, want nothing", out.String())
				}
				return
			}
			if !strings.Contains(out.String(), tc.wantOut) {
				t.Errorf("recap = %q, want it to contain %q", out.String(), tc.wantOut)
			}
			// The AltScreen already cleared the box; the recap replaces it and
			// must never redraw one.
			if strings.ContainsAny(out.String(), "╭╮╰╯") {
				t.Errorf("recap must never reprint the summary box:\n%s", out.String())
			}
		})
	}
}

// TestDeployStreamHooksUseTheDemoFeedUnderTheDemoEnv keeps the screenshot and
// demo paths off the real engine.
func TestDeployStreamHooksUseTheDemoFeedUnderTheDemoEnv(t *testing.T) {
	t.Setenv(wizardDemoEnv, "1")
	hooks := deployStreamHooks(context.Background(), config.DefaultConfig(), &deploy.Options{})
	if hooks.Execute == nil {
		t.Fatal("the demo feed must still supply an Execute hook")
	}
	if hooks.CancelDeploy == nil {
		t.Error("the demo feed must supply its own cancel")
	}
}

// TestDeployStreamClearsVerboseForTheEngine pins the fix for --verbose painting
// raw subprocess output under the AltScreen: streamWriters tees stdout/stderr to
// the terminal when Verbose is set, which the stream screen owns.
func TestDeployStreamClearsVerboseForTheEngine(t *testing.T) {
	var got deploy.Options
	prev := deployExecuteFn
	t.Cleanup(func() { deployExecuteFn = prev })
	deployExecuteFn = func(_ context.Context, _ *config.Config, opts *deploy.Options, _ io.Writer) (*deploy.Outcome, error) {
		got = *opts
		return &deploy.Outcome{}, nil
	}

	opts := &deploy.Options{Verbose: true, LogSink: io.Discard}
	hooks := deployStreamHooks(context.Background(), config.DefaultConfig(), opts)
	if err := hooks.Execute(&deployexec.State{}, make(chan deployexec.Event, 8)); err != nil {
		t.Fatalf("execute hook: %v", err)
	}

	if got.Verbose {
		t.Error("the engine must not tee subprocess output to a terminal the stream screen owns")
	}
	if got.LogSink == nil {
		t.Error("the sink must still receive everything")
	}
	if !opts.Verbose {
		t.Error("the caller's own options must not be mutated")
	}
}

// TestDeployStreamGracefulCancelReportsTheResumeGuidance pins that ctrl+c #1
// exits with the same interrupted message the force-quit path prints, not a
// bare "context canceled".
func TestDeployStreamGracefulCancelReportsTheResumeGuidance(t *testing.T) {
	cfg := config.DefaultConfig()
	cases := []struct {
		name string
		st   *deployexec.State
	}{
		{
			name: "force quit mid-install",
			st:   &deployexec.State{Cfg: cfg, Started: true},
		},
		{
			name: "graceful cancel unwound the engine",
			st:   &deployexec.State{Cfg: cfg, Started: true, Executed: true, Result: context.Canceled},
		},
		{
			name: "graceful cancel wrapped by the engine",
			st: &deployexec.State{
				Cfg: cfg, Started: true, Executed: true,
				Result: fmt.Errorf("monitor installation: %w", context.Canceled),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := reportDeployStreamOutcome(&out, tc.st)
			if err == nil {
				t.Fatal("an interrupted deploy must exit non-zero")
			}
			if !strings.Contains(err.Error(), "re-run 'okdctl deploy' to resume") {
				t.Errorf("err = %q, want the resume guidance", err.Error())
			}
			if out.Len() != 0 {
				t.Errorf("an interrupted deploy must print no recap; got %q", out.String())
			}
		})
	}
}

// TestDeployStreamDemoCancelStopsTheFeed proves the screen's ctrl+c reaches the
// demo feed: its own cancel must survive session assembly, since its events come
// from its own context and not the engine's.
func TestDeployStreamDemoCancelStopsTheFeed(t *testing.T) {
	t.Setenv(wizardDemoEnv, "1")

	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "homelab"
	engineCtx, engineCancel := context.WithCancel(context.Background())
	defer engineCancel()

	hooks := deployStreamSession(engineCtx, engineCancel, cfg, &deploy.Options{})

	st := &deployexec.State{Cfg: cfg, Plan: demoStreamPlan()}
	events := make(chan deployexec.Event, 256)
	done := make(chan error, 1)
	go func() { done <- hooks.Execute(st, events) }()

	<-events // the first step opened, so the feed is live
	hooks.CancelDeploy()

	var err error
	select {
	case err = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelling the screen never stopped the demo feed")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("demo feed returned %v, want context.Canceled", err)
	}

	st.Result, st.Executed = err, true
	card := tuitest.StripANSI(deployexec.NewDoneStep(st, hooks).View(96, 40))
	if !strings.Contains(card, "deploy interrupted") {
		t.Errorf("a cancelled demo must land on the interrupted card, not the success box:\n%s", card)
	}
}

// demoStreamPlan is a short stand-in for deploy.PlannedSteps' real listing, so
// the cancel test never depends on a workspace.
func demoStreamPlan() []tui.StepMeta {
	plan := make([]tui.StepMeta, 40)
	for i := range plan {
		plan[i] = tui.StepMeta{ID: distribution.StepID(fmt.Sprintf("step-%02d", i)), Name: "step", Phase: "setup"}
	}
	return plan
}

// TestDeployStreamOptedOutByYes pins the ruling that a scripted deploy keeps the
// plain checklist: its durable record is scrollback, which an AltScreen erases.
func TestDeployStreamOptedOutByYes(t *testing.T) {
	t.Cleanup(func() { deployNoTUI, deployYes = false, false })

	deployNoTUI, deployYes = false, false
	if deployStreamOptedOut() {
		t.Error("an ordinary interactive deploy must not opt out")
	}

	deployYes = true
	if !deployStreamOptedOut() {
		t.Error("--yes must take the plain checklist, never the AltScreen")
	}
	if deployStreamEnabled() {
		t.Error("--yes must close the stream gate outright")
	}

	deployYes, deployNoTUI = false, true
	if !deployStreamOptedOut() {
		t.Error("--no-tui must still opt out")
	}
}

// TestDeployStreamRecapUsesTheEnginesOwnDuration keeps the recap reporting the
// span the engine measured rather than the screen's, which also counts the lock
// wait and config validation ahead of it.
func TestDeployStreamRecapUsesTheEnginesOwnDuration(t *testing.T) {
	prev := deployExecuteFn
	t.Cleanup(func() { deployExecuteFn = prev })
	deployExecuteFn = func(context.Context, *config.Config, *deploy.Options, io.Writer) (*deploy.Outcome, error) {
		return &deploy.Outcome{RunID: "run-9", Duration: 42 * time.Minute}, nil
	}

	st := &deployexec.State{Cfg: config.DefaultConfig()}
	hooks := deployStreamHooks(context.Background(), st.Cfg, &deploy.Options{})
	if err := hooks.Execute(st, make(chan deployexec.Event, 8)); err != nil {
		t.Fatalf("execute hook: %v", err)
	}

	if st.Elapsed != 42*time.Minute {
		t.Fatalf("Elapsed = %s, want the engine's own 42m0s", st.Elapsed)
	}

	st.Executed = true
	var out bytes.Buffer
	if err := reportDeployStreamOutcome(&out, st); err != nil {
		t.Fatalf("report: %v", err)
	}
	if !strings.Contains(out.String(), "42m0s") {
		t.Errorf("recap = %q, want the engine's duration", out.String())
	}
}

// TestDeployStreamSessionBoundsTheEngineGoroutine proves the screen's feed guard
// is actually armed: without a Done channel the step's final send could outlive a
// force-quit on a feed nobody drains.
func TestDeployStreamSessionBoundsTheEngineGoroutine(t *testing.T) {
	engineCtx, engineCancel := context.WithCancel(context.Background())
	defer engineCancel()

	hooks := deployStreamSession(engineCtx, engineCancel, config.DefaultConfig(), &deploy.Options{})
	if hooks.Done == nil {
		t.Fatal("the session must hand the screen a stop signal for its own feed")
	}

	select {
	case <-hooks.Done:
		t.Fatal("the stop signal fired before the run was cancelled")
	default:
	}

	engineCancel()
	select {
	case <-hooks.Done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelling the run never fired the screen's stop signal")
	}
}

// TestDeployHistorySeedsAndRecordsAroundTheStream pins the weight model's
// persistence loop: a finished run's measured durations land in the history
// file, and the next session's stream state is seeded from them.
func TestDeployHistorySeedsAndRecordsAroundTheStream(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(workspace.WorkDir(root), 0o750); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "homelab"

	done := &deployexec.State{Cfg: cfg, RunID: "run-1", Executed: true, Steps: []distribution.StepResult{
		{StepID: setup.StepInstallPackages, Success: true, Duration: 40 * time.Second},
	}}
	recordDeployHistory(done, root)

	next := &deployexec.State{Cfg: cfg}
	loadDeployHistory(next, root)
	if next.History[setup.StepInstallPackages] != 40*time.Second {
		t.Fatalf("History = %v, want the recorded 40s seed", next.History)
	}

	// The demo feed must neither read nor write schedule data.
	t.Setenv(wizardDemoEnv, "1")
	demo := &deployexec.State{Cfg: cfg, RunID: "run-2", Executed: true, Steps: done.Steps}
	recordDeployHistory(demo, root)
	fresh := &deployexec.State{Cfg: cfg}
	loadDeployHistory(fresh, root)
	if fresh.History != nil {
		t.Errorf("demo run must not touch the history store, got %v", fresh.History)
	}
}

// TestRecordDeployHistorySkipsAnAbandonedRun pins the force-quit race: a
// quit abandons the engine goroutine mid-write, so State.Steps may still be
// mutating after RunFlow returns. Only a run whose final event was consumed
// (Executed — the channel-synchronized handoff) may be read back into the
// history store; under -race this test fails if the recorder touches the
// slice a late writer still owns.
func TestRecordDeployHistorySkipsAnAbandonedRun(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(workspace.WorkDir(root), 0o750); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "homelab"

	st := &deployexec.State{Cfg: cfg, RunID: "run-1", Started: true} // Executed false: abandoned
	stop := make(chan struct{})
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				st.Steps = append(st.Steps, distribution.StepResult{
					StepID: setup.StepInstallPackages, Success: true, Duration: time.Second,
				})
				if i == 0 {
					close(started)
				}
			}
		}
	}()

	<-started
	for range 50 {
		recordDeployHistory(st, root)
	}
	close(stop)
	<-done

	if got := deployexec.LoadStepHistory(workspace.WorkDir(root), "homelab"); got != nil {
		t.Errorf("an abandoned run must not seed the history store, got %v", got)
	}
}
