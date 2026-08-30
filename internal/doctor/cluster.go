package doctor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
)

// ClusterProbe is the subset of *cluster.Client the day-2 checks consume, owned
// here so tests can fake it.
type ClusterProbe interface {
	ListNodes(ctx context.Context) ([]cluster.NodeDetail, error)
	ClusterOperatorHealth(ctx context.Context) (cluster.OperatorHealth, error)
	PendingCSRs(ctx context.Context) ([]cluster.CSR, error)
	EtcdHealthy(ctx context.Context) (cluster.EtcdHealth, error)
	SignerNotAfter(ctx context.Context) (time.Time, error)
}

const signerWarnWindow = 30 * 24 * time.Hour

// ClusterCheck builds the day-2 "cluster" section from a live probe; the CLI
// appends it only when a kubeconfig is present.
func ClusterCheck(probe ClusterProbe) Check {
	return Check{
		Name: "cluster",
		Desc: "day-2 cluster health",
		Fn:   func(ctx context.Context) Result { return clusterHealth(ctx, probe) },
	}
}

func clusterHealth(ctx context.Context, probe ClusterProbe) Result {
	var items []Item
	worst := Pass
	add := func(sev Severity, name, note string) {
		items = append(items, Item{Sev: sev, Name: name, Note: note})
		worst = max(worst, sev)
	}

	// Nodes double as the reachability probe, short-circuiting to one finding
	// instead of five cascading errors.
	nodes, err := probe.ListNodes(ctx)
	if err != nil {
		return Result{
			Sev:   Warn,
			Items: []Item{{Sev: Warn, Name: "api", Note: "cluster unreachable: " + oneLine(err)}},
		}
	}
	var notReady []string
	for _, n := range nodes {
		if !n.Ready {
			notReady = append(notReady, n.Name)
		}
	}
	if len(notReady) > 0 {
		add(Fail, "nodes", fmt.Sprintf("%d NotReady: %s", len(notReady), strings.Join(notReady, ", ")))
	} else {
		add(Pass, "nodes", fmt.Sprintf("%d ready", len(nodes)))
	}

	if h, err := probe.ClusterOperatorHealth(ctx); err != nil {
		add(Warn, "cluster operators", "query failed: "+oneLine(err))
	} else {
		switch {
		case len(h.Degraded) > 0:
			add(Fail, "cluster operators", "degraded: "+strings.Join(h.Degraded, ", "))
		case len(h.Progressing) > 0:
			add(Warn, "cluster operators", "progressing: "+strings.Join(h.Progressing, ", "))
		default:
			add(Pass, "cluster operators", fmt.Sprintf("%d/%d available", h.Available, h.Total))
		}
	}

	pending := -1
	if csrs, err := probe.PendingCSRs(ctx); err != nil {
		add(Warn, "pending csrs", "query failed: "+oneLine(err))
	} else {
		pending = len(csrs)
		if pending > 0 {
			add(Warn, "pending csrs", fmt.Sprintf("%d awaiting approval", pending))
		} else {
			add(Pass, "pending csrs", "none")
		}
	}

	if eh, err := probe.EtcdHealthy(ctx); err != nil {
		add(Warn, "etcd", "query failed: "+oneLine(err))
	} else if eh.Healthy {
		add(Pass, "etcd", fmt.Sprintf("healthy (%d/%d pods ready)", eh.PodsReady, eh.PodsTotal))
	} else {
		add(Fail, "etcd", eh.Reason)
	}

	if notAfter, err := probe.SignerNotAfter(ctx); err != nil {
		add(Warn, "signer expiry", "query failed: "+oneLine(err))
	} else {
		remaining := time.Until(notAfter)
		days := int(remaining.Hours() / 24)
		date := notAfter.Format("2006-01-02")
		switch {
		case remaining <= 0:
			add(Fail, "signer expiry",
				fmt.Sprintf("kube-apiserver-to-kubelet-signer EXPIRED %s", date))
		case remaining < signerWarnWindow:
			add(Warn, "signer expiry",
				fmt.Sprintf("kube-apiserver-to-kubelet-signer expires in %d days (%s) — rotate before kubelets fail", days, date))
		default:
			add(Pass, "signer expiry", fmt.Sprintf("%d days remaining (%s)", days, date))
		}
	}

	// Pending CSRs with NotReady nodes is the classic post-signer-expiry recovery state.
	if pending > 0 && len(notReady) > 0 {
		add(Warn, "csr recovery",
			"NotReady nodes with pending CSRs — approve them: oc get csr -o name | xargs oc adm certificate approve")
	}

	return Result{Sev: worst, Items: items}
}

// oneLine flattens an error so multi-line oc stderr can't smear the aligned item list.
func oneLine(err error) string {
	return strings.ReplaceAll(strings.TrimSpace(err.Error()), "\n", "; ")
}
