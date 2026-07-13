package cluster

import (
	"testing"

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
