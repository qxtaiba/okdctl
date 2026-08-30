package cluster

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

func TestNodeIndex(t *testing.T) {
	cases := []struct {
		name    string
		wantIdx int
		wantOK  bool
	}{
		{"worker2", 2, true},
		{"grappleberry-worker11", 11, true},
		// Kubernetes reports FQDNs; the index is in the first label, not the domain.
		{"grappleberry-worker0.grappleberry.k8s.local", 0, true},
		{"bootstrap", 0, false},
		{"", 0, false},
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

// installFakeOCHugeOutput emits >4MiB so RunOutput sets Result.Truncated.
func installFakeOCHugeOutput(t *testing.T) {
	t.Helper()
	testutil.InstallFakeBin(t, "oc", `#!/bin/sh
head -c 5000000 /dev/zero | tr '\0' '{'
exit 0
`)
}

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

// installFakeOCGeneric installs an "oc" fake logging argv/stdin and echoing
// $OC_STDOUT, exit $OC_EXIT_CODE (default 0); returns the log paths.
func installFakeOCGeneric(t *testing.T) (argvLog, stdinLog string) {
	t.Helper()
	testutil.InstallFakeBin(t, "oc", `#!/bin/sh
echo "$@" >> "$OC_ARGV_LOG"
cat > "$OC_STDIN_LOG"
printf '%s' "${OC_STDOUT:-}"
exit "${OC_EXIT_CODE:-0}"
`)
	dir := t.TempDir()
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

func TestClientArgvShapes(t *testing.T) {
	tests := []struct {
		name   string
		stdout string // OC_STDOUT for verbs that parse output
		call   func(ctx context.Context, c *Client) error
		want   string
	}{
		{
			name: "cordon",
			call: func(ctx context.Context, c *Client) error { return c.Cordon(ctx, "worker0") },
			want: "adm cordon worker0",
		},
		{
			name: "uncordon",
			call: func(ctx context.Context, c *Client) error { return c.Uncordon(ctx, "worker0") },
			want: "adm uncordon worker0",
		},
		{
			name: "drain all options",
			call: func(ctx context.Context, c *Client) error {
				return c.Drain(ctx, "worker0", DrainOptions{
					Force:            true,
					Timeout:          "10m",
					DeleteEmptyDir:   true,
					IgnoreDaemonsets: true,
				})
			},
			want: "adm drain worker0 --ignore-daemonsets --delete-emptydir-data --force --timeout=10m",
		},
		{
			name: "drain no options",
			call: func(ctx context.Context, c *Client) error { return c.Drain(ctx, "worker0", DrainOptions{}) },
			want: "adm drain worker0",
		},
		{
			name: "delete node",
			call: func(ctx context.Context, c *Client) error { return c.DeleteNode(ctx, "worker0") },
			want: "delete node worker0 --ignore-not-found",
		},
		{
			name: "set masters schedulable",
			call: func(ctx context.Context, c *Client) error { return c.SetMastersSchedulable(ctx, true) },
			want: `patch schedulers.config.openshift.io cluster --type=merge -p {"spec":{"mastersSchedulable":true}}`,
		},
		{
			name:   "pods for selector all namespaces",
			stdout: `{"items":[]}`,
			call: func(ctx context.Context, c *Client) error {
				_, err := c.PodsForSelector(ctx, "", "app=rook-ceph-osd")
				return err
			},
			want: "get pods -A -l app=rook-ceph-osd -o json",
		},
		{
			name: "patch",
			call: func(ctx context.Context, c *Client) error {
				return c.Patch(ctx, "operatorhub.config.openshift.io", "cluster", "merge", `{"spec":{"sources":[]}}`)
			},
			want: `patch operatorhub.config.openshift.io cluster --type=merge -p {"spec":{"sources":[]}}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			argvLog, _ := installFakeOCGeneric(t)
			if tc.stdout != "" {
				t.Setenv("OC_STDOUT", tc.stdout)
			}
			c := New(WithCLI("oc"), WithExecutor(executor.New()))
			if err := tc.call(context.Background(), c); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := readArgvLog(t, argvLog); got != tc.want {
				t.Errorf("argv = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestClientNonZeroExitWrapsClusterError(t *testing.T) {
	cases := []struct {
		name string
		call func(ctx context.Context, c *Client) error
	}{
		{"Cordon", func(ctx context.Context, c *Client) error { return c.Cordon(ctx, "worker0") }},
		{"Uncordon", func(ctx context.Context, c *Client) error { return c.Uncordon(ctx, "worker0") }},
		{"Drain", func(ctx context.Context, c *Client) error { return c.Drain(ctx, "worker0", DrainOptions{}) }},
		{"DeleteNode", func(ctx context.Context, c *Client) error { return c.DeleteNode(ctx, "worker0") }},
		{"SetMastersSchedulable", func(ctx context.Context, c *Client) error { return c.SetMastersSchedulable(ctx, false) }},
		{"ListNodes", func(ctx context.Context, c *Client) error { _, err := c.ListNodes(ctx); return err }},
		{"PodsForSelector", func(ctx context.Context, c *Client) error { _, err := c.PodsForSelector(ctx, "", ""); return err }},
		{"Apply", func(ctx context.Context, c *Client) error { return c.Apply(ctx, []byte(`{}`)) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installFakeOCGeneric(t)
			t.Setenv("OC_EXIT_CODE", "1")
			c := New(WithCLI("oc"), WithExecutor(executor.New()))

			err := tc.call(context.Background(), c)
			if err == nil {
				t.Fatal("expected error on non-zero exit")
			}
			var ce *errtypes.ClusterError
			if !errors.As(err, &ce) {
				t.Fatalf("err is %T; want *errtypes.ClusterError", err)
			}
		})
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
