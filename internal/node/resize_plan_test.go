package node

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

func TestResizePlanDisruption(t *testing.T) {
	for _, tc := range []struct {
		diskOnly, skipDrain bool
		want                ResizeMode
	}{
		{true, false, ResizeLiveDisk},
		{true, true, ResizeLiveDisk},
		{false, false, ResizeDrainedRestart},
		{false, true, ResizeUndrainedRestart},
	} {
		plan := resizePlan(nodetypes.RoleWorker, nil, "test", ResizeOptions{MemoryMB: 8192, OSDiskGB: 100, SkipDrain: tc.skipDrain}, tc.diskOnly)
		if plan.ResizeMode != tc.want {
			t.Fatalf("%+v: got %v", tc, plan.ResizeMode)
		}
	}
}
