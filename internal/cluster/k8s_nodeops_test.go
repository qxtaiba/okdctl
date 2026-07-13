package cluster

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
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
	if runtime.GOOS == "windows" {
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
