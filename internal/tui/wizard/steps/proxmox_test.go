package steps

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func TestProxmoxStepWarnsOnAPlainHTTPEndpoint(t *testing.T) {
	step := NewProxmoxStep()
	step.LoadFromConfig(config.DefaultConfig(), false)
	for endpoint, want := range map[string]bool{"https://pve:8006": false, "HTTP://pve:8006": true, "http://10.0.0.5": true} {
		step.SetValue(config.FieldProxmoxEndpoint, endpoint)
		if got := strings.Contains(tuitest.StripANSI(step.View(120, 40)), "cleartext"); got != want {
			t.Errorf("endpoint %q: cleartext warning shown = %v, want %v", endpoint, got, want)
		}
	}
}
