package lifecycle

import "testing"

func TestOptionsFromState(t *testing.T) {
	st := &State{
		MemoryMB: 24576, CPU: 6, OSDiskGB: 100, SkipDrain: true, Ack: true,
		ForceStorage: true, DrainTimeout: "5m", Count: 2,
	}

	ro := ResizeOptionsFrom(st)
	if ro.MemoryMB != 24576 || ro.CPU != 6 || ro.OSDiskGB != 100 || !ro.SkipDrain || !ro.Acknowledge {
		t.Errorf("ResizeOptionsFrom = %+v", ro)
	}
	rm := RemoveOptionsFrom(st)
	if !rm.ForceStorage || !rm.SkipDrain || rm.DrainTimeout != "5m" || !rm.Acknowledge {
		t.Errorf("RemoveOptionsFrom = %+v", rm)
	}
	ao := AddOptionsFrom(st)
	if ao.Count != 2 || !ao.Acknowledge {
		t.Errorf("AddOptionsFrom = %+v", ao)
	}
}
