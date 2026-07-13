package cluster

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// ClusterOperatorsAvailable returns how many cluster operators report
// Available=True out of the total, from `get clusteroperators -o json`. The
// install monitor calls it each poll tick to surface convergence progress on
// the status line; a transient query failure returns the error so the caller
// can skip the tick without aborting the wait.
func (c *Client) ClusterOperatorsAvailable(ctx context.Context) (available, total int, err error) {
	raw, err := c.getJSONChecked(ctx, "get", "clusteroperators", "-o", "json")
	if err != nil {
		return 0, 0, err
	}
	return parseClusterOperatorsAvailable(raw)
}

func parseClusterOperatorsAvailable(data []byte) (available, total int, err error) {
	var list struct {
		Items []struct {
			Status struct {
				Conditions []k8sCondition `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return 0, 0, fmt.Errorf("parse clusteroperators json: %w", err)
	}
	total = len(list.Items)
	for _, co := range list.Items {
		for _, cond := range co.Status.Conditions {
			if nodetypes.ConditionType(cond.Type) == nodetypes.ConditionTypeAvailable &&
				cond.Status == string(nodetypes.ConditionStatusTrue) {
				available++
			}
		}
	}
	return available, total, nil
}
