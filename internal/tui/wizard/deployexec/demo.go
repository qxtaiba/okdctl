package deployexec

import (
	"context"
	"time"

	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// DemoKubeVipIP is the fixed api vip the demo outcome reports, so the done
// screen's dns rows render without a cluster. Deliberately a documentation
// address (RFC 5737), never a real homelab one.
const DemoKubeVipIP = "192.0.2.10"

// DemoHooks returns Hooks that replay the real step plan as a scripted feed
// paced by stepDelay, so every deploy screen renders without a hypervisor.
func DemoHooks(stepDelay time.Duration) Hooks {
	// ctx has no caller context to inherit — a demo run's only cancellation
	// source is the stream screen's graceful cancel, wired through CancelDeploy.
	ctx, cancel := context.WithCancel(context.Background())
	return Hooks{
		CancelDeploy: cancel,
		Done:         ctx.Done(),
		Execute: func(st *State, events chan<- Event) error {
			return demoExecute(ctx, st, events, stepDelay)
		},
	}
}

// demoExecute walks st.Plan start-to-finish, honouring ctx cancellation between
// every event so a graceful cancel unblocks promptly instead of running the
// fixture out. Each step also logs one human line, so the log pane fills from
// the same stream a real run feeds it.
func demoExecute(ctx context.Context, st *State, events chan<- Event, stepDelay time.Duration) error {
	steps := make([]distribution.StepResult, 0, len(st.Plan))
	for _, m := range st.Plan {
		if err := demoSend(ctx, events, Event{StepID: m.ID}); err != nil {
			return err
		}
		logutil.Info("deploy step started", logutil.LF("step", string(m.ID)), logutil.LF("phase", m.Phase))
		if err := demoWait(ctx, stepDelay); err != nil {
			return err
		}
		if err := demoSend(ctx, events, Event{StepID: m.ID, Done: true, Took: stepDelay}); err != nil {
			return err
		}
		steps = append(steps, distribution.StepResult{StepID: m.ID, Success: true, Duration: stepDelay})
	}

	st.Steps = steps
	st.Summary = &postinstall.Result{
		BootstrapCleaned: true,
		DNSDeployed:      true,
		KubeVipIP:        DemoKubeVipIP,
	}
	return nil
}

func demoSend(ctx context.Context, events chan<- Event, ev Event) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case events <- ev:
		return nil
	}
}

// demoWait pauses for d, or returns ctx's error the moment it's cancelled — a
// timer instead of time.Sleep so cancellation is never left waiting out a step.
func demoWait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
