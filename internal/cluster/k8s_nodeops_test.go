package cluster

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

func TestNodeIndex(t *testing.T) {
	cases := []struct {
		name    string
		wantIdx int
		wantOK  bool
	}{
		{"worker2", 2, true},
		{"master0", 0, true},
		{"grappleberry-worker11", 11, true},
		// Kubernetes reports FQDNs; the index is in the first label, not the domain.
		{"grappleberry-worker0.grappleberry.k8s.local", 0, true},
		{"grappleberry-master2.grappleberry.k8s.local", 2, true},
		{"bootstrap", 0, false},
		{"bootstrap.grappleberry.k8s.local", 0, false},
		{"", 0, false},
		{"node-", 0, false},
	}
	for _, c := range cases {
		idx, ok := NodeIndex(c.name)
		if ok != c.wantOK || (ok && idx != c.wantIdx) {
			t.Errorf("NodeIndex(%q) = (%d,%v), want (%d,%v)", c.name, idx, ok, c.wantIdx, c.wantOK)
		}
	}
}

func TestParseNodeList(t *testing.T) {
	data := `{"items":[
	  {"metadata":{"name":"master0","labels":{"node-role.kubernetes.io/master":""}},
	   "status":{"conditions":[{"type":"Ready","status":"True"}]}},
	  {"metadata":{"name":"worker1","labels":{"node-role.kubernetes.io/worker":""}},
	   "status":{"conditions":[{"type":"Ready","status":"False"}]}}
	]}`
	nodes, err := parseNodeList([]byte(data))
	if err != nil {
		t.Fatalf("parseNodeList: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("want 2 nodes, got %d", len(nodes))
	}
	if nodes[0].Role != nodetypes.RoleMaster || !nodes[0].Ready {
		t.Errorf("master0 parsed wrong: %+v", nodes[0])
	}
	if nodes[1].Role != nodetypes.RoleWorker || nodes[1].Ready {
		t.Errorf("worker1 parsed wrong: %+v", nodes[1])
	}
}

func TestParseMastersSchedulable(t *testing.T) {
	on, err := parseMastersSchedulable([]byte(`{"spec":{"mastersSchedulable":true}}`))
	if err != nil || !on {
		t.Fatalf("want true, got %v (%v)", on, err)
	}
	off, err := parseMastersSchedulable([]byte(`{"spec":{}}`))
	if err != nil || off {
		t.Fatalf("want false default, got %v (%v)", off, err)
	}
}

// installFakeOCHugeOutput writes a PATH-shadow "oc" script emitting more
// than the executor's 4 MiB output-capture cap, so RunOutput sets
// Result.Truncated without needing a real oversized cluster response.
func installFakeOCHugeOutput(t *testing.T) {
	t.Helper()
	if runtime.GOOS == goosWindows {
		t.Skip("fake-oc script relies on POSIX sh")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
head -c 5000000 /dev/zero | tr '\0' '{'
exit 0
`
	path := filepath.Join(dir, "oc")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestMastersSchedulable_TruncatedOutputErrors is a regression test: before
// the getJSONChecked fold, MastersSchedulable checked ExitCode only and fed
// a capped (partial) payload straight to json.Unmarshal.
func TestMastersSchedulable_TruncatedOutputErrors(t *testing.T) {
	installFakeOCHugeOutput(t)
	c := New(WithCLI("oc"), WithLogger(logutil.NopLogger))

	_, err := c.MastersSchedulable(context.Background())
	if err == nil {
		t.Fatal("expected error for truncated output; got nil")
	}
	var ce *errtypes.ClusterError
	if !errors.As(err, &ce) {
		t.Fatalf("err is %T; want *errtypes.ClusterError", err)
	}
}

func TestParsePodPlacements(t *testing.T) {
	data := `{"items":[
	  {"metadata":{"name":"osd-0","namespace":"rook-ceph"},"spec":{"nodeName":"worker2"}},
	  {"metadata":{"name":"osd-1","namespace":"rook-ceph"},"spec":{"nodeName":"worker0"}}
	]}`
	pods, err := parsePodPlacements([]byte(data))
	if err != nil {
		t.Fatalf("parsePodPlacements: %v", err)
	}
	if len(pods) != 2 || pods[0].NodeName != "worker2" || pods[0].Namespace != "rook-ceph" {
		t.Fatalf("unexpected parse: %+v", pods)
	}
}

// installFakeOCGeneric writes a POSIX sh "oc" script that appends its full
// argv to an argv log, copies stdin to a stdin log, prints $OC_STDOUT
// verbatim, and exits $OC_EXIT_CODE (default 0). Returns the argv and stdin
// log paths. Combines installFakePatchOC's argv logging with
// installFakeOCEmitting's stdout emission so one fake covers both the
// argv-shape assertions and the JSON-emitting reads used by the
// node-lifecycle and raw-query primitives.
func installFakeOCGeneric(t *testing.T) (argvLog, stdinLog string) {
	t.Helper()
	if runtime.GOOS == goosWindows {
		t.Skip("fake-oc script relies on POSIX sh")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
echo "$@" >> "$OC_ARGV_LOG"
cat > "$OC_STDIN_LOG"
printf '%s' "${OC_STDOUT:-}"
exit "${OC_EXIT_CODE:-0}"
`
	path := filepath.Join(dir, "oc")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	argvLog = filepath.Join(dir, "argv.log")
	stdinLog = filepath.Join(dir, "stdin.log")
	t.Setenv("OC_ARGV_LOG", argvLog)
	t.Setenv("OC_STDIN_LOG", stdinLog)
	return argvLog, stdinLog
}

func readArgvLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("argv log not written: %v", err)
	}
	return strings.TrimSpace(string(data))
}

func TestClientCordon_ArgvShape(t *testing.T) {
	argvLog, _ := installFakeOCGeneric(t)
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	if err := c.Cordon(context.Background(), "worker0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := readArgvLog(t, argvLog), "adm cordon worker0"; got != want {
		t.Errorf("argv = %q; want %q", got, want)
	}
}

func TestClientCordon_NonZeroExitWrapsClusterError(t *testing.T) {
	installFakeOCGeneric(t)
	t.Setenv("OC_EXIT_CODE", "1")
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	err := c.Cordon(context.Background(), "worker0")
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
	var ce *errtypes.ClusterError
	if !errors.As(err, &ce) {
		t.Fatalf("err is %T; want *errtypes.ClusterError", err)
	}
}

func TestClientUncordon_ArgvShape(t *testing.T) {
	argvLog, _ := installFakeOCGeneric(t)
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	if err := c.Uncordon(context.Background(), "worker0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := readArgvLog(t, argvLog), "adm uncordon worker0"; got != want {
		t.Errorf("argv = %q; want %q", got, want)
	}
}

func TestClientUncordon_NonZeroExitWrapsClusterError(t *testing.T) {
	installFakeOCGeneric(t)
	t.Setenv("OC_EXIT_CODE", "1")
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	err := c.Uncordon(context.Background(), "worker0")
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
	var ce *errtypes.ClusterError
	if !errors.As(err, &ce) {
		t.Fatalf("err is %T; want *errtypes.ClusterError", err)
	}
}

func TestClientDrain_ArgvShape_AllOptions(t *testing.T) {
	argvLog, _ := installFakeOCGeneric(t)
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	opts := DrainOptions{
		Force:            true,
		Timeout:          "10m",
		DeleteEmptyDir:   true,
		IgnoreDaemonsets: true,
	}
	if err := c.Drain(context.Background(), "worker0", opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "adm drain worker0 --ignore-daemonsets --delete-emptydir-data --force --timeout=10m"
	if got := readArgvLog(t, argvLog); got != want {
		t.Errorf("argv = %q; want %q", got, want)
	}
}

func TestClientDrain_ArgvShape_NoOptions(t *testing.T) {
	argvLog, _ := installFakeOCGeneric(t)
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	if err := c.Drain(context.Background(), "worker0", DrainOptions{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := readArgvLog(t, argvLog), "adm drain worker0"; got != want {
		t.Errorf("argv = %q; want %q", got, want)
	}
}

func TestClientDrain_NonZeroExitWrapsClusterError(t *testing.T) {
	installFakeOCGeneric(t)
	t.Setenv("OC_EXIT_CODE", "1")
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	err := c.Drain(context.Background(), "worker0", DrainOptions{})
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
	var ce *errtypes.ClusterError
	if !errors.As(err, &ce) {
		t.Fatalf("err is %T; want *errtypes.ClusterError", err)
	}
}

func TestClientDeleteNode_ArgvShape(t *testing.T) {
	argvLog, _ := installFakeOCGeneric(t)
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	if err := c.DeleteNode(context.Background(), "worker0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := readArgvLog(t, argvLog), "delete node worker0 --ignore-not-found"; got != want {
		t.Errorf("argv = %q; want %q", got, want)
	}
}

func TestClientDeleteNode_NonZeroExitWrapsClusterError(t *testing.T) {
	installFakeOCGeneric(t)
	t.Setenv("OC_EXIT_CODE", "1")
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	err := c.DeleteNode(context.Background(), "worker0")
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
	var ce *errtypes.ClusterError
	if !errors.As(err, &ce) {
		t.Fatalf("err is %T; want *errtypes.ClusterError", err)
	}
}

func TestClientSetMastersSchedulable_ArgvShape(t *testing.T) {
	argvLog, _ := installFakeOCGeneric(t)
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	if err := c.SetMastersSchedulable(context.Background(), true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `patch schedulers.config.openshift.io cluster --type=merge -p {"spec":{"mastersSchedulable":true}}`
	if got := readArgvLog(t, argvLog); got != want {
		t.Errorf("argv = %q; want %q", got, want)
	}
}

func TestClientSetMastersSchedulable_NonZeroExitWrapsClusterError(t *testing.T) {
	installFakeOCGeneric(t)
	t.Setenv("OC_EXIT_CODE", "1")
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	err := c.SetMastersSchedulable(context.Background(), false)
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
	var ce *errtypes.ClusterError
	if !errors.As(err, &ce) {
		t.Fatalf("err is %T; want *errtypes.ClusterError", err)
	}
}

func TestClientListNodes_FakeOC(t *testing.T) {
	installFakeOCGeneric(t)
	t.Setenv("OC_STDOUT", `{"items":[
	  {"metadata":{"name":"master0","labels":{"node-role.kubernetes.io/master":""}},
	   "status":{"conditions":[{"type":"Ready","status":"True"}]}}
	]}`)
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	nodes, err := c.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 1 || nodes[0].Name != "master0" || !nodes[0].Ready {
		t.Errorf("unexpected nodes: %+v", nodes)
	}
}

func TestClientListNodes_NonZeroExitWrapsClusterError(t *testing.T) {
	installFakeOCGeneric(t)
	t.Setenv("OC_EXIT_CODE", "1")
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	_, err := c.ListNodes(context.Background())
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
	var ce *errtypes.ClusterError
	if !errors.As(err, &ce) {
		t.Fatalf("err is %T; want *errtypes.ClusterError", err)
	}
}

func TestClientPodsForSelector_ArgvShape_AllNamespaces(t *testing.T) {
	argvLog, _ := installFakeOCGeneric(t)
	t.Setenv("OC_STDOUT", `{"items":[]}`)
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	if _, err := c.PodsForSelector(context.Background(), "", "app=rook-ceph-osd"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := readArgvLog(t, argvLog), "get pods -A -l app=rook-ceph-osd -o json"; got != want {
		t.Errorf("argv = %q; want %q", got, want)
	}
}

func TestClientPodsForSelector_ArgvShape_Namespaced(t *testing.T) {
	argvLog, _ := installFakeOCGeneric(t)
	t.Setenv("OC_STDOUT", `{"items":[
	  {"metadata":{"name":"osd-0","namespace":"rook-ceph"},"spec":{"nodeName":"worker2"}}
	]}`)
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	pods, err := c.PodsForSelector(context.Background(), "rook-ceph", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pods) != 1 || pods[0].NodeName != "worker2" {
		t.Errorf("unexpected pods: %+v", pods)
	}
	if got, want := readArgvLog(t, argvLog), "get pods -n rook-ceph -o json"; got != want {
		t.Errorf("argv = %q; want %q", got, want)
	}
}

func TestClientPodsForSelector_NonZeroExitWrapsClusterError(t *testing.T) {
	installFakeOCGeneric(t)
	t.Setenv("OC_EXIT_CODE", "1")
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	_, err := c.PodsForSelector(context.Background(), "", "")
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
	var ce *errtypes.ClusterError
	if !errors.As(err, &ce) {
		t.Fatalf("err is %T; want *errtypes.ClusterError", err)
	}
}

func TestClientApply_ArgvShapeAndStdin(t *testing.T) {
	argvLog, stdinLog := installFakeOCGeneric(t)
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	manifest := []byte(`{"kind":"ConfigMap"}`)
	if err := c.Apply(context.Background(), manifest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := readArgvLog(t, argvLog), "apply -f -"; got != want {
		t.Errorf("argv = %q; want %q", got, want)
	}
	data, err := os.ReadFile(stdinLog)
	if err != nil {
		t.Fatalf("stdin log not written: %v", err)
	}
	if !bytes.Equal(data, manifest) {
		t.Errorf("stdin = %q; want %q", data, manifest)
	}
}

func TestClientApply_NonZeroExitWrapsClusterError(t *testing.T) {
	installFakeOCGeneric(t)
	t.Setenv("OC_EXIT_CODE", "1")
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	err := c.Apply(context.Background(), []byte(`{}`))
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
	var ce *errtypes.ClusterError
	if !errors.As(err, &ce) {
		t.Fatalf("err is %T; want *errtypes.ClusterError", err)
	}
}
