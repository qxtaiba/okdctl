package proxmox

import (
	"testing"
	"time"
)

// ShutdownVM/StartVM/VMRunning/PowerCycleVM all require a live Proxmox API
// (no go-proxmox HTTP mock exists in this repo); their behavior is covered
// at the node.Runner layer via fakePower. timeout() is the only pure logic
// here and gets direct coverage.
func TestPowerCycler_timeout(t *testing.T) {
	t.Run("default when unset", func(t *testing.T) {
		pc := NewPowerCycler(&PowerCycleOptions{})
		if got := pc.timeout(); got != DefaultPowerCycleTimeout {
			t.Errorf("timeout() = %v; want %v", got, DefaultPowerCycleTimeout)
		}
	})

	t.Run("default when negative", func(t *testing.T) {
		pc := NewPowerCycler(&PowerCycleOptions{Timeout: -time.Second})
		if got := pc.timeout(); got != DefaultPowerCycleTimeout {
			t.Errorf("timeout() = %v; want %v", got, DefaultPowerCycleTimeout)
		}
	})

	t.Run("override honored", func(t *testing.T) {
		want := 90 * time.Second
		pc := NewPowerCycler(&PowerCycleOptions{Timeout: want})
		if got := pc.timeout(); got != want {
			t.Errorf("timeout() = %v; want %v", got, want)
		}
	})
}
