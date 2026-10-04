package lifecycle

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func pump(t *testing.T, s *ExecStep, first tea.Cmd) tea.Msg {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	s.SetVisitContext(ctx)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	messages := make(chan tea.Msg, 32)
	dispatch := func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		workers.Go(func() {
			msg := cmd()
			select {
			case messages <- msg:
			case <-ctx.Done():
			}
		})
	}
	dispatch(first)
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			t.Fatal("execution did not finish")
			return nil
		case msg := <-messages:
			if batch, ok := msg.(tea.BatchMsg); ok {
				for _, cmd := range batch {
					dispatch(cmd)
				}
				continue
			}
			if _, ok := msg.(spinner.TickMsg); ok {
				continue
			}
			if _, ok := msg.(wizard.StepCompleteMsg); ok {
				return msg
			}
			_, next := s.Update(msg)
			dispatch(next)
		}
	}
}

func execState() *State {
	st := doneState()
	st.Scope = node.ResizeScope{Role: nodetypes.RoleMaster}
	return st
}

func TestExecStepRunsToCompletion(t *testing.T) {
	st := execState()
	executed := false
	s := NewExecStep(st, Hooks{Execute: func(_ *State, ch chan<- ExecEvent) error {
		executed = true
		ch <- ExecEvent{Node: "homelab-master0", Step: node.StepTFApply}
		ch <- ExecEvent{Desc: "waiting for etcd health (post-homelab-master0)", Done: true}
		return nil
	}})
	msg := pump(t, s, s.Init())
	if _, ok := msg.(wizard.StepCompleteMsg); !ok {
		t.Fatalf("final msg = %T, want StepCompleteMsg", msg)
	}
	if !executed {
		t.Error("Execute hook never ran")
	}
	if st.Result != nil {
		t.Errorf("Result = %v, want nil", st.Result)
	}
	out := s.View(90, 40)
	if !strings.Contains(out, "homelab-master0") {
		t.Errorf("view must render the node section:\n%s", out)
	}
}

func TestExecStepFailurePropagatesToState(t *testing.T) {
	st := execState()
	boom := errors.New("etcd health gate failed")
	s := NewExecStep(st, Hooks{Execute: func(*State, chan<- ExecEvent) error { return boom }})
	msg := pump(t, s, s.Init())
	if _, ok := msg.(wizard.StepCompleteMsg); !ok {
		t.Fatalf("failure must still advance to the done screen, got %T", msg)
	}
	if !errors.Is(st.Result, boom) {
		t.Errorf("Result = %v, want the execute error", st.Result)
	}
}

func TestExecStepQuitGuardCancelsThenForces(t *testing.T) {
	cancelled := false
	s := NewExecStep(execState(), Hooks{
		CancelOp: func() { cancelled = true },
		Execute:  func(*State, chan<- ExecEvent) error { return nil },
	})
	if !s.InterceptQuit() {
		t.Fatal("first ctrl+c must be intercepted")
	}
	if !cancelled {
		t.Fatal("first ctrl+c must invoke CancelOp")
	}
	if s.InterceptQuit() {
		t.Fatal("second ctrl+c must pass through (force quit)")
	}
}

func TestExecStepShouldShow(t *testing.T) {
	cfg := config.DefaultConfig()
	if NewExecStep(&State{Cfg: cfg, Proceed: false}, Hooks{}).ShouldShow(cfg) {
		t.Error("exec step must hide without consent")
	}
	if !NewExecStep(execState(), Hooks{}).ShouldShow(cfg) {
		t.Error("exec step must show after consent")
	}
}

func TestExecAndDoneStepsAreForwardOnly(t *testing.T) {
	st := execState()
	if !NewExecStep(st, Hooks{}).InterceptBack() {
		t.Error("exec step must intercept esc — navigating away orphans the event pump")
	}
	if !NewDoneStep(st).InterceptBack() {
		t.Error("done step must intercept esc — going back re-enters a finished run")
	}
}
