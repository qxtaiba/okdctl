package phase

import "testing"

func TestParseNodeRole(t *testing.T) {
	t.Run("accepts canonical values", func(t *testing.T) {
		for _, v := range []string{"bootstrap", "master", "worker"} {
			got, err := ParseNodeRole(v)
			if err != nil {
				t.Errorf("ParseNodeRole(%q) error: %v", v, err)
			}
			if string(got) != v {
				t.Errorf("ParseNodeRole(%q) = %q", v, got)
			}
		}
	})

	t.Run("rejects unknown and case variants", func(t *testing.T) {
		bad := []string{"", "Bootstrap", "MASTER", "workers", "agent", "control-plane"}
		for _, v := range bad {
			if _, err := ParseNodeRole(v); err == nil {
				t.Errorf("ParseNodeRole(%q) accepted unknown role", v)
			}
		}
	})

	t.Run("constants stay lowercase - load-bearing for openshift-install", func(t *testing.T) {
		if string(RoleBootstrap) != "bootstrap" {
			t.Errorf("RoleBootstrap = %q", RoleBootstrap)
		}
		if string(RoleMaster) != "master" {
			t.Errorf("RoleMaster = %q", RoleMaster)
		}
		if string(RoleWorker) != "worker" {
			t.Errorf("RoleWorker = %q", RoleWorker)
		}
	})
}
