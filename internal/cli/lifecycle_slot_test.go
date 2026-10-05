package cli

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/qxtaiba/okdctl/internal/tui/wizard/lifecycle"
)

// countingSession is a lifecycleSession whose close only records that it ran.
func countingSession(closes *atomic.Int32) *lifecycleSession {
	return &lifecycleSession{
		state: &lifecycle.State{},
		close: func() { closes.Add(1) },
	}
}

func TestLifecycleSlotClosesASessionArrivingAfterTheWizardExits(t *testing.T) {
	var slot lifecycleSlot
	var closes atomic.Int32

	// The wizard quit while prepareNodeOpsEnv was still probing: the main path
	// takes an empty slot, and the session lands afterwards.
	if got := slot.take(); got != nil {
		t.Fatalf("take() = %v on an empty slot, want nil", got)
	}

	if accepted := slot.put(countingSession(&closes)); accepted {
		t.Error("put() accepted a session after take(); nothing is left to close it")
	}
	if got := closes.Load(); got != 1 {
		t.Errorf("late-arriving session closed %d times, want 1 — its credentials stay live and its work directory root-owned otherwise", got)
	}
}

func TestLifecycleSlotClosesEachSessionExactlyOnce(t *testing.T) {
	// Run under -race: put and take genuinely contend, and whichever loses must
	// still leave exactly one close behind.
	for range 200 {
		var slot lifecycleSlot
		var closes atomic.Int32
		sess := countingSession(&closes)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			slot.put(sess)
		}()

		if taken := slot.take(); taken != nil {
			taken.close()
		}
		wg.Wait()

		if got := closes.Load(); got != 1 {
			t.Fatalf("session closed %d times across a put/take race, want exactly 1", got)
		}
	}
}

func TestLifecycleSlotClosesTheSessionItReplaces(t *testing.T) {
	var slot lifecycleSlot
	var first, second atomic.Int32

	slot.put(countingSession(&first))
	slot.put(countingSession(&second))

	if got := first.Load(); got != 1 {
		t.Errorf("the replaced session closed %d times, want 1 — re-entering manage nodes builds a new one and the old flow is gone", got)
	}
	if got := second.Load(); got != 0 {
		t.Errorf("the live session closed %d times, want 0", got)
	}

	taken := slot.take()
	if taken == nil {
		t.Fatal("take() = nil, want the session the slot holds")
	}
	taken.close()
	if got := second.Load(); got != 1 {
		t.Errorf("the held session closed %d times after take, want 1", got)
	}
}
