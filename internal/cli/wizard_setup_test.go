package cli

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/lifecycle"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/steps"
)

func TestDemoClusterStatusCarriesNoCredentials(t *testing.T) {
	st := demoClusterStatus()

	var rendered strings.Builder
	rendered.WriteString(string(st.Phase))
	for _, n := range st.Nodes {
		rendered.WriteString(" " + n.Name + " " + string(n.Role))
	}
	for _, a := range st.Addons {
		rendered.WriteString(" " + a.Name + " " + a.Error)
	}

	for _, forbidden := range []string{"password", "token", "secret", "@pam", "https://", "root@"} {
		if strings.Contains(strings.ToLower(rendered.String()), forbidden) {
			t.Errorf("demo status fixture leaks %q; it renders into screenshots: %s", forbidden, rendered.String())
		}
	}
	if len(st.Nodes) != 6 {
		t.Errorf("demo status fixture has %d nodes, want the lifecycle fixture's 6", len(st.Nodes))
	}
	if !strings.HasPrefix(st.Nodes[0].Name, lifecycle.DemoClusterName) {
		t.Errorf("node %q must be named after lifecycle.DemoClusterName so both demo screens agree", st.Nodes[0].Name)
	}
	if st.Nodes[0].Role != nodetypes.RoleMaster {
		t.Errorf("first node role = %q, want master", st.Nodes[0].Role)
	}
}

func TestSaveSlotStateIsConfiguredWithoutInfrastructure(t *testing.T) {
	t.Chdir(t.TempDir())

	cfg := config.DefaultConfig()
	if got := saveSlotState(cfg); got != steps.SaveSlotConfigured {
		t.Errorf("saveSlotState() = %q in an empty workspace, want %q — the hub must never overstate a cluster",
			got, steps.SaveSlotConfigured)
	}
}
