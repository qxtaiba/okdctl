package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestMergeNamedList(t *testing.T) {
	mk := func(kv ...string) kubeEntry {
		e := kubeEntry{}
		for i := 0; i+1 < len(kv); i += 2 {
			b, _ := json.Marshal(kv[i+1])
			e[kv[i]] = b
		}
		return e
	}
	entryName := func(e kubeEntry) string {
		var s string
		_ = json.Unmarshal(e["name"], &s)
		return s
	}

	t.Run("nil src returns dest unchanged", func(t *testing.T) {
		dest := []kubeEntry{mk("name", "a")}
		got := mergeNamedList(dest, nil)
		if len(got) != 1 || entryName(got[0]) != "a" {
			t.Errorf("got %v, want one entry 'a'", got)
		}
	})

	t.Run("empty src returns dest unchanged", func(t *testing.T) {
		dest := []kubeEntry{mk("name", "a")}
		got := mergeNamedList(dest, []kubeEntry{})
		if len(got) != 1 || entryName(got[0]) != "a" {
			t.Errorf("got %v, want one entry 'a'", got)
		}
	})

	t.Run("src entry with no name collision is appended", func(t *testing.T) {
		dest := []kubeEntry{mk("name", "existing")}
		src := []kubeEntry{mk("name", "new")}
		got := mergeNamedList(dest, src)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2: %+v", len(got), got)
		}
		if entryName(got[0]) != "existing" {
			t.Errorf("first entry lost")
		}
		if entryName(got[1]) != "new" {
			t.Errorf("new entry not appended")
		}
	})

	t.Run("src entry with same name is NOT appended (no-clobber)", func(t *testing.T) {
		dest := []kubeEntry{mk("name", "prod")}
		src := []kubeEntry{mk("name", "prod")}
		got := mergeNamedList(dest, src)
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1 (src must be dropped)", len(got))
		}
		if entryName(got[0]) != "prod" {
			t.Errorf("existing entry lost: %v", got[0])
		}
	})

	t.Run("mix: one collides, one does not", func(t *testing.T) {
		dest := []kubeEntry{mk("name", "prod")}
		src := []kubeEntry{mk("name", "prod"), mk("name", "staging")}
		got := mergeNamedList(dest, src)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		names := []string{entryName(got[0]), entryName(got[1])}
		want := []string{"prod", "staging"}
		if !reflect.DeepEqual(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})

	t.Run("entries without a name key are skipped silently", func(t *testing.T) {
		noName := kubeEntry{"cluster": json.RawMessage(`"no-name-key"`)}
		dest := []kubeEntry{}
		src := []kubeEntry{mk("name", "good"), noName}
		got := mergeNamedList(dest, src)
		if len(got) != 1 {
			t.Errorf("len = %d, want 1 (only named entry survives)", len(got))
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

func TestMergeKubeconfig_PreservesCurrentContext(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "config")

	destYAML := `apiVersion: v1
kind: Config
clusters:
- name: prod
  cluster:
    server: https://prod.example
users: []
contexts: []
current-context: prod
`
	if err := os.WriteFile(dest, []byte(destYAML), 0o600); err != nil {
		t.Fatalf("seed dest kubeconfig: %v", err)
	}

	t.Setenv("KUBECONFIG", dest)

	srcData := []byte(`apiVersion: v1
kind: Config
clusters:
- name: okd-test
  cluster:
    server: https://okd-test.example
- name: dev
  cluster:
    server: https://dev.example
users: []
contexts: []
current-context: okd-test
`)

	if err := mergeKubeconfig(srcData); err != nil {
		t.Fatalf("mergeKubeconfig: %v", err)
	}

	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read merged dest: %v", err)
	}

	var merged map[string]any
	if err := yaml.Unmarshal(raw, &merged); err != nil {
		t.Fatalf("parse merged kubeconfig: %v", err)
	}

	if got, _ := merged["current-context"].(string); got != "prod" {
		t.Errorf("current-context = %q, want %q", got, "prod")
	}

	clusters, _ := merged["clusters"].([]any)
	wantNames := []string{"prod", "okd-test", "dev"}
	if len(clusters) != len(wantNames) {
		t.Fatalf("clusters len = %d, want %d: %+v", len(clusters), len(wantNames), clusters)
	}
	for i, wantName := range wantNames {
		m, _ := clusters[i].(map[string]any)
		if got, _ := m["name"].(string); got != wantName {
			t.Errorf("clusters[%d].name = %q, want %q", i, got, wantName)
		}
	}

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("dest mode = %04o, want 0600", got)
	}
}

func TestMergeKubeconfig_EmptyDestTakesSrcCurrentContext(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "config")

	t.Setenv("KUBECONFIG", dest)

	srcData := []byte(`apiVersion: v1
kind: Config
clusters:
- name: okd-test
  cluster:
    server: https://okd-test.example
users:
- name: admin
  user:
    token: some-token
contexts:
- name: okd-test
  context:
    cluster: okd-test
    user: admin
current-context: okd-test
`)

	if err := mergeKubeconfig(srcData); err != nil {
		t.Fatalf("mergeKubeconfig: %v", err)
	}

	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read merged dest: %v", err)
	}

	var merged map[string]any
	if err := yaml.Unmarshal(raw, &merged); err != nil {
		t.Fatalf("parse merged kubeconfig: %v", err)
	}

	if got, _ := merged["current-context"].(string); got != "okd-test" {
		t.Errorf("current-context = %q, want %q", got, "okd-test")
	}

	clusters, _ := merged["clusters"].([]any)
	if len(clusters) != 1 {
		t.Fatalf("clusters len = %d, want 1: %+v", len(clusters), clusters)
	}
	m, _ := clusters[0].(map[string]any)
	if got, _ := m["name"].(string); got != "okd-test" {
		t.Errorf("clusters[0].name = %q, want %q", got, "okd-test")
	}

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("dest mode = %04o, want 0600", got)
	}
}

func TestMergeKubeconfig_PreservesUnknownFields(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "config")
	t.Setenv("KUBECONFIG", dest)

	srcData := []byte(`apiVersion: v1
kind: Config
clusters:
- name: okd-test
  cluster:
    server: https://okd-test.example
  x-custom-extension: preserved-value
users: []
contexts: []
current-context: okd-test
`)

	if err := mergeKubeconfig(srcData); err != nil {
		t.Fatalf("mergeKubeconfig: %v", err)
	}

	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}

	var merged map[string]any
	if err := yaml.Unmarshal(raw, &merged); err != nil {
		t.Fatalf("parse merged kubeconfig: %v", err)
	}

	clusters, _ := merged["clusters"].([]any)
	if len(clusters) != 1 {
		t.Fatalf("clusters len = %d, want 1", len(clusters))
	}
	entry, _ := clusters[0].(map[string]any)
	if entry["x-custom-extension"] != "preserved-value" {
		t.Errorf("x-custom-extension not preserved after merge: entry = %v", entry)
	}
}
