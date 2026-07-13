package doctor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
)

// fakeProbe is a scripted ClusterProbe: each field supplies one probe's return.
type fakeProbe struct {
	nodes    []cluster.NodeDetail
	nodesErr error
	ops      cluster.OperatorHealth
	opsErr   error
	csrs     []cluster.CSR
	csrsErr  error
	etcd     cluster.EtcdHealth
	etcdErr  error
	notAfter time.Time
	signErr  error
}

func (f *fakeProbe) ListNodes(context.Context) ([]cluster.NodeDetail, error) {
	return f.nodes, f.nodesErr
}

func (f *fakeProbe) ClusterOperatorHealth(context.Context) (cluster.OperatorHealth, error) {
	return f.ops, f.opsErr
}

func (f *fakeProbe) PendingCSRs(context.Context) ([]cluster.CSR, error) {
	return f.csrs, f.csrsErr
}

func (f *fakeProbe) EtcdHealthy(context.Context) (cluster.EtcdHealth, error) {
	return f.etcd, f.etcdErr
}

func (f *fakeProbe) SignerNotAfter(context.Context) (time.Time, error) {
	return f.notAfter, f.signErr
}

func itemBySuffix(items []Item, name string) (Item, bool) {
	for _, it := range items {
		if it.Name == name {
			return it, true
		}
	}
	return Item{}, false
}

func healthyProbe() *fakeProbe {
	return &fakeProbe{
		nodes:    []cluster.NodeDetail{{Name: "master0", Ready: true}, {Name: "worker0", Ready: true}},
		ops:      cluster.OperatorHealth{Available: 30, Total: 30},
		csrs:     nil,
		etcd:     cluster.EtcdHealth{Healthy: true, PodsReady: 3, PodsTotal: 3},
		notAfter: time.Now().Add(200 * 24 * time.Hour),
	}
}

func TestClusterHealth_AllHealthy(t *testing.T) {
	r := clusterHealth(context.Background(), healthyProbe())
	if r.Sev != Pass {
		t.Fatalf("Sev = %v; want Pass", r.Sev)
	}
	for _, it := range r.Items {
		if it.Sev != Pass {
			t.Errorf("item %q Sev = %v; want Pass", it.Name, it.Sev)
		}
	}
}

func TestClusterHealth_Unreachable(t *testing.T) {
	p := &fakeProbe{nodesErr: errors.New("dial tcp: connection refused")}
	r := clusterHealth(context.Background(), p)
	if r.Sev != Warn {
		t.Fatalf("Sev = %v; want Warn", r.Sev)
	}
	if len(r.Items) != 1 {
		t.Fatalf("items = %d; want exactly 1 (no cascade)", len(r.Items))
	}
	if r.Items[0].Name != "api" || !strings.Contains(r.Items[0].Note, "unreachable") {
		t.Errorf("item = %+v; want single api-unreachable finding", r.Items[0])
	}
}

func TestClusterHealth_DegradedOperatorFails(t *testing.T) {
	p := healthyProbe()
	p.ops = cluster.OperatorHealth{Degraded: []string{"ingress"}, Available: 29, Total: 30}
	r := clusterHealth(context.Background(), p)
	if r.Sev != Fail {
		t.Fatalf("Sev = %v; want Fail", r.Sev)
	}
	it, ok := itemBySuffix(r.Items, "cluster operators")
	if !ok || it.Sev != Fail || !strings.Contains(it.Note, "ingress") {
		t.Errorf("operators item = %+v ok=%v; want Fail naming ingress", it, ok)
	}
}

func TestClusterHealth_ProgressingOperatorWarns(t *testing.T) {
	p := healthyProbe()
	p.ops = cluster.OperatorHealth{Progressing: []string{"kube-apiserver"}, Available: 30, Total: 30}
	r := clusterHealth(context.Background(), p)
	if r.Sev != Warn {
		t.Fatalf("Sev = %v; want Warn", r.Sev)
	}
	it, _ := itemBySuffix(r.Items, "cluster operators")
	if it.Sev != Warn || !strings.Contains(it.Note, "kube-apiserver") {
		t.Errorf("operators item = %+v; want Warn naming kube-apiserver", it)
	}
}

func TestClusterHealth_EtcdUnhealthyFails(t *testing.T) {
	p := healthyProbe()
	p.etcd = cluster.EtcdHealth{Healthy: false, Reason: "etcd pods not all ready (2/3)"}
	r := clusterHealth(context.Background(), p)
	if r.Sev != Fail {
		t.Fatalf("Sev = %v; want Fail", r.Sev)
	}
	it, _ := itemBySuffix(r.Items, "etcd")
	if it.Sev != Fail || !strings.Contains(it.Note, "2/3") {
		t.Errorf("etcd item = %+v; want Fail with reason", it)
	}
}

func TestClusterHealth_SignerExpiringWarns(t *testing.T) {
	p := healthyProbe()
	p.notAfter = time.Now().Add(12 * 24 * time.Hour)
	r := clusterHealth(context.Background(), p)
	if r.Sev != Warn {
		t.Fatalf("Sev = %v; want Warn", r.Sev)
	}
	it, _ := itemBySuffix(r.Items, "signer expiry")
	if it.Sev != Warn || !strings.Contains(it.Note, "expires in") {
		t.Errorf("signer item = %+v; want Warn with days remaining", it)
	}
}

func TestClusterHealth_SignerExpiredFails(t *testing.T) {
	p := healthyProbe()
	p.notAfter = time.Now().Add(-time.Hour)
	r := clusterHealth(context.Background(), p)
	it, _ := itemBySuffix(r.Items, "signer expiry")
	if r.Sev != Fail || it.Sev != Fail || !strings.Contains(it.Note, "EXPIRED") {
		t.Errorf("Sev = %v, signer item = %+v; want Fail EXPIRED", r.Sev, it)
	}
}

func TestClusterHealth_CSRRecoveryHint(t *testing.T) {
	p := healthyProbe()
	p.nodes = []cluster.NodeDetail{{Name: "master0", Ready: true}, {Name: "worker0", Ready: false}}
	p.csrs = []cluster.CSR{{Name: "csr-1"}, {Name: "csr-2"}}
	r := clusterHealth(context.Background(), p)
	it, ok := itemBySuffix(r.Items, "csr recovery")
	if !ok || !strings.Contains(it.Note, "certificate approve") {
		t.Errorf("recovery item = %+v ok=%v; want approval suggestion", it, ok)
	}
	// NotReady node itself must fail.
	if r.Sev != Fail {
		t.Errorf("Sev = %v; want Fail (NotReady node)", r.Sev)
	}
}

func TestClusterHealth_NoCSRRecoveryWhenNodesReady(t *testing.T) {
	p := healthyProbe()
	p.csrs = []cluster.CSR{{Name: "csr-1"}}
	r := clusterHealth(context.Background(), p)
	if _, ok := itemBySuffix(r.Items, "csr recovery"); ok {
		t.Error("csr recovery hint present with all nodes Ready; want absent")
	}
	// Pending CSRs alone are a warning, not a failure.
	if r.Sev != Warn {
		t.Errorf("Sev = %v; want Warn (pending csrs only)", r.Sev)
	}
}
