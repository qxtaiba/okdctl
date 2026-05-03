package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMergeNamedList(t *testing.T) {
	t.Run("nil src returns dest unchanged", func(t *testing.T) {
		dest := []any{map[string]any{"name": "a"}}
		got := mergeNamedList(dest, nil)
		if !reflect.DeepEqual(got, dest) {
			t.Errorf("got %v, want %v", got, dest)
		}
	})

	t.Run("empty src returns dest unchanged", func(t *testing.T) {
		dest := []any{map[string]any{"name": "a"}}
		got := mergeNamedList(dest, []any{})
		if !reflect.DeepEqual(got, dest) {
			t.Errorf("got %v, want %v", got, dest)
		}
	})

	t.Run("src entry with no name collision is appended", func(t *testing.T) {
		dest := []any{map[string]any{"name": "existing"}}
		src := []any{map[string]any{"name": "new"}}
		got, _ := mergeNamedList(dest, src).([]any)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2: %+v", len(got), got)
		}
		if got[0].(map[string]any)["name"] != "existing" {
			t.Errorf("first entry lost")
		}
		if got[1].(map[string]any)["name"] != "new" {
			t.Errorf("new entry not appended")
		}
	})

	t.Run("src entry with same name is NOT appended (no-clobber)", func(t *testing.T) {
		dest := []any{map[string]any{"name": "prod", "cluster": map[string]any{"server": "https://prod.example"}}}
		src := []any{map[string]any{"name": "prod", "cluster": map[string]any{"server": "https://EVIL.example"}}}
		got, _ := mergeNamedList(dest, src).([]any)
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1 (src must be dropped)", len(got))
		}
		srv, _ := got[0].(map[string]any)["cluster"].(map[string]any)
		if srv["server"] != "https://prod.example" {
			t.Errorf("existing entry was clobbered: got %v", got[0])
		}
	})

	t.Run("mix: one collides, one does not", func(t *testing.T) {
		dest := []any{map[string]any{"name": "prod"}}
		src := []any{
			map[string]any{"name": "prod"},
			map[string]any{"name": "staging"},
		}
		got, _ := mergeNamedList(dest, src).([]any)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		names := []string{
			got[0].(map[string]any)["name"].(string),
			got[1].(map[string]any)["name"].(string),
		}
		want := []string{"prod", "staging"}
		if !reflect.DeepEqual(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})

	t.Run("entries without a name key are skipped silently", func(t *testing.T) {
		dest := []any{}
		src := []any{
			map[string]any{"name": "good"},
			map[string]any{"cluster": "no-name-key"},
			"not-a-map",
		}
		got, _ := mergeNamedList(dest, src).([]any)
		if len(got) != 1 {
			t.Errorf("len = %d, want 1 (only named map survives)", len(got))
		}
	})
}

func TestMergeKubeconfig_Perms(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "config")

	existingKubeconfig := `apiVersion: v1
kind: Config
users:
- name: existing-user
  user:
    token: real-token
clusters: []
contexts: []
current-context: ""
`
	if err := os.WriteFile(dest, []byte(existingKubeconfig), 0o600); err != nil {
		t.Fatalf("seed dest kubeconfig: %v", err)
	}

	t.Setenv("KUBECONFIG", dest)

	srcKubeconfig := []byte(`apiVersion: v1
kind: Config
users:
- name: new-user
  user:
    token: new-token
clusters: []
contexts: []
current-context: new-context
`)

	if err := mergeKubeconfig(srcKubeconfig); err != nil {
		t.Fatalf("mergeKubeconfig: %v", err)
	}

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("dest mode = %04o, want 0600", got)
	}

	merged, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	mergedStr := string(merged)

	if !strings.Contains(mergedStr, "real-token") {
		t.Errorf("original token not preserved in merged kubeconfig:\n%s", mergedStr)
	}
	if !strings.Contains(mergedStr, "new-token") {
		t.Errorf("src user token not appended in merged kubeconfig:\n%s", mergedStr)
	}

	leftovers, err := filepath.Glob(filepath.Join(dir, ".tmp-*"))
	if err != nil {
		t.Fatalf("glob .tmp-*: %v", err)
	}
	if len(leftovers) != 0 {
		t.Errorf("AtomicWrite left temp artifacts: %v", leftovers)
	}

	if err := os.Remove(dest); err != nil {
		t.Errorf("remove dest: %v", err)
	}
}
