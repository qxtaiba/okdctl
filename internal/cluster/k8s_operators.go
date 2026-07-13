package cluster

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// OperatorHealth summarizes ClusterOperator conditions across the cluster.
// Degraded and Progressing hold the operator names in each state; Available
// counts operators reporting Available=True over Total. A stable cluster has
// both name slices empty and Available == Total.
type OperatorHealth struct {
	Degraded    []string
	Progressing []string
	Available   int
	Total       int
}

// ClusterOperatorHealth reports Degraded and Progressing ClusterOperators plus
// an available/total count from `oc get clusteroperators -o json`. Day-2 health
// probes read this to distinguish a stable cluster from one mid-rollout
// (Progressing) or carrying a broken operator (Degraded).
func (c *Client) ClusterOperatorHealth(ctx context.Context) (OperatorHealth, error) {
	data, err := c.getJSONChecked(ctx, "get cluster operators", "get", "clusteroperators", "-o", "json")
	if err != nil {
		return OperatorHealth{}, err
	}
	h, err := parseOperatorHealth(data)
	if err != nil {
		return OperatorHealth{}, &errtypes.ClusterError{Msg: "parse cluster operators", Err: err}
	}
	return h, nil
}

func parseOperatorHealth(data []byte) (OperatorHealth, error) {
	var col struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Conditions []k8sCondition `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &col); err != nil {
		return OperatorHealth{}, fmt.Errorf("parse clusteroperator list json: %w", err)
	}
	h := OperatorHealth{Total: len(col.Items)}
	for _, op := range col.Items {
		var available, degraded, progressing bool
		for _, cond := range op.Status.Conditions {
			isTrue := cond.Status == string(nodetypes.ConditionStatusTrue)
			switch nodetypes.ConditionType(cond.Type) {
			case nodetypes.ConditionTypeAvailable:
				available = isTrue
			case nodetypes.ConditionTypeDegraded:
				degraded = isTrue
			case nodetypes.ConditionTypeProgressing:
				progressing = isTrue
			}
		}
		if available {
			h.Available++
		}
		if degraded {
			h.Degraded = append(h.Degraded, op.Metadata.Name)
		}
		if progressing {
			h.Progressing = append(h.Progressing, op.Metadata.Name)
		}
	}
	return h, nil
}
