package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// EtcdHealth summarizes the etcd quorum's fitness for a one-at-a-time
// control-plane mutation. Healthy is true only when the etcd ClusterOperator
// is Available and neither Degraded nor Progressing, every etcd pod is Ready,
// and no static-pod revision rollout is in flight. Reason names the first
// failing check for operator-facing messages.
type EtcdHealth struct {
	Healthy         bool
	OperatorOK      bool
	PodsReady       int
	PodsTotal       int
	RolloutInFlight bool
	Reason          string
}

// EtcdHealthy probes the etcd ClusterOperator, the openshift-etcd pods, and the
// etcd resource's rollout condition. A master resize/removal MUST gate on this
// before and after each node so quorum is never mutated mid-rollout.
func (c *Client) EtcdHealthy(ctx context.Context) (EtcdHealth, error) {
	coRaw, err := c.getJSONChecked(ctx, "get", "clusteroperator", "etcd", "-o", "json")
	if err != nil {
		return EtcdHealth{}, err
	}
	operatorOK, err := parseOperatorAvailableStable(coRaw)
	if err != nil {
		return EtcdHealth{}, &errtypes.ClusterError{Msg: "parse etcd operator", Err: err}
	}

	podsRaw, err := c.getJSONChecked(ctx, "get", "pods", "-n", "openshift-etcd",
		"-l", "app=etcd", "-o", "json")
	if err != nil {
		return EtcdHealth{}, err
	}
	ready, total, err := parsePodsReady(podsRaw)
	if err != nil {
		return EtcdHealth{}, &errtypes.ClusterError{Msg: "parse etcd pods", Err: err}
	}

	etcdRaw, err := c.getJSONChecked(ctx, "get", "etcd", "cluster", "-o", "json")
	if err != nil {
		return EtcdHealth{}, err
	}
	rollout, err := parseEtcdRolloutInFlight(etcdRaw)
	if err != nil {
		return EtcdHealth{}, &errtypes.ClusterError{Msg: "parse etcd rollout", Err: err}
	}

	h := EtcdHealth{
		OperatorOK:      operatorOK,
		PodsReady:       ready,
		PodsTotal:       total,
		RolloutInFlight: rollout,
	}
	switch {
	case !operatorOK:
		h.Reason = "etcd cluster operator is not Available/stable (Degraded or Progressing)"
	case total == 0:
		h.Reason = "no etcd pods found in openshift-etcd"
	case ready != total:
		h.Reason = fmt.Sprintf("etcd pods not all ready (%d/%d)", ready, total)
	case rollout:
		h.Reason = "etcd static-pod revision rollout in progress"
	default:
		h.Healthy = true
	}
	return h, nil
}

func (c *Client) getJSONChecked(ctx context.Context, args ...string) ([]byte, error) {
	result, err := c.runOutput(ctx, args...)
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, &errtypes.ClusterError{
			Msg: fmt.Sprintf("%s %s", c.CLI, subcommand(args)),
			Err: executor.NewExitError(ctx, c.CLI+" "+subcommand(args), result.ExitCode, strings.TrimSpace(result.Stderr)),
		}
	}
	if result.Truncated {
		return nil, &errtypes.ClusterError{Msg: fmt.Sprintf("%s %s: output truncated; cannot parse", c.CLI, subcommand(args))}
	}
	return []byte(result.Stdout), nil
}

type k8sCondition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

// parseOperatorAvailableStable reports whether a ClusterOperator is Available
// with Degraded and Progressing both false — the "stable" state a quorum
// mutation requires.
func parseOperatorAvailableStable(data []byte) (bool, error) {
	var co struct {
		Status struct {
			Conditions []k8sCondition `json:"conditions"`
		} `json:"status"`
	}
	if err := json.Unmarshal(data, &co); err != nil {
		return false, fmt.Errorf("parse clusteroperator json: %w", err)
	}
	available, degraded, progressing := false, false, false
	for _, cond := range co.Status.Conditions {
		switch nodetypes.ConditionType(cond.Type) {
		case nodetypes.ConditionTypeAvailable:
			available = cond.Status == string(nodetypes.ConditionStatusTrue)
		case nodetypes.ConditionTypeDegraded:
			degraded = cond.Status == string(nodetypes.ConditionStatusTrue)
		case nodetypes.ConditionTypeProgressing:
			progressing = cond.Status == string(nodetypes.ConditionStatusTrue)
		}
	}
	return available && !degraded && !progressing, nil
}

// parsePodsReady counts pods whose Ready condition is True over the total,
// from a `get pods -o json` list.
func parsePodsReady(data []byte) (ready, total int, err error) {
	var pl struct {
		Items []struct {
			Status struct {
				Conditions []k8sCondition `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &pl); err != nil {
		return 0, 0, fmt.Errorf("parse pod list json: %w", err)
	}
	total = len(pl.Items)
	for _, p := range pl.Items {
		for _, cond := range p.Status.Conditions {
			if cond.Type == string(nodetypes.ConditionTypeReady) && cond.Status == string(nodetypes.ConditionStatusTrue) {
				ready++
			}
		}
	}
	return ready, total, nil
}

// parseEtcdRolloutInFlight reports whether the etcd operator is mid-rollout,
// detected via a NodeInstallerProgressing=True condition on the etcd resource.
func parseEtcdRolloutInFlight(data []byte) (bool, error) {
	var e struct {
		Status struct {
			Conditions []k8sCondition `json:"conditions"`
		} `json:"status"`
	}
	if err := json.Unmarshal(data, &e); err != nil {
		return false, fmt.Errorf("parse etcd json: %w", err)
	}
	for _, cond := range e.Status.Conditions {
		if cond.Type == "NodeInstallerProgressing" && cond.Status == string(nodetypes.ConditionStatusTrue) {
			return true, nil
		}
	}
	return false, nil
}
