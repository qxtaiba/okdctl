package deployexec

import (
	"testing"
	"time"
)

// TestDemoWaitDelayHonorsEnvOverride pins OKDCTL_DEMO_STEP_DELAY: unset or
// invalid, the caller's own pace wins; a valid duration overrides it.
func TestDemoWaitDelayHonorsEnvOverride(t *testing.T) {
	t.Setenv(demoStepDelayEnv, "")
	if got := demoWaitDelay(120 * time.Millisecond); got != 120*time.Millisecond {
		t.Errorf("no override = %v, want the caller's 120ms", got)
	}

	t.Setenv(demoStepDelayEnv, "not-a-duration")
	if got := demoWaitDelay(120 * time.Millisecond); got != 120*time.Millisecond {
		t.Errorf("invalid override = %v, want the caller's 120ms unchanged", got)
	}

	t.Setenv(demoStepDelayEnv, "2s")
	if got := demoWaitDelay(120 * time.Millisecond); got != 2*time.Second {
		t.Errorf("valid override = %v, want 2s", got)
	}
}

// TestDemoHooksStepDelayOverrideLeavesReportedDurationsAlone proves the
// decoupling: slowing the feed down for a screenshot tape via
// OKDCTL_DEMO_STEP_DELAY must not also inflate the Took/Duration numbers the
// checklist renders — only the wait a human or a tape watches changes.
func TestDemoHooksStepDelayOverrideLeavesReportedDurationsAlone(t *testing.T) {
	t.Setenv(demoStepDelayEnv, "1ms")

	st := streamState()
	hooks := DemoHooks(42 * time.Second)

	events := make(chan Event, 256)
	done := make(chan error, 1)
	go func() {
		err := hooks.Execute(st, events)
		close(events)
		done <- err
	}()

	var sawDone bool
	for ev := range events {
		if ev.Done {
			sawDone = true
			if ev.Took != 42*time.Second {
				t.Errorf("event Took = %v, want the caller's 42s, not the overridden wait", ev.Took)
			}
		}
	}
	if err := <-done; err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !sawDone {
		t.Fatal("setup: no Done event observed")
	}
	for _, r := range st.Steps {
		if r.Duration != 42*time.Second {
			t.Errorf("recorded step duration = %v, want the caller's 42s", r.Duration)
		}
	}
}
