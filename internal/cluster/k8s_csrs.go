package cluster

import (
	"context"
	"encoding/json"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// PendingCSRs returns CSRs whose status.conditions is empty — the
// kube-controller-manager's representation of neither approved nor denied.
func (c *Client) PendingCSRs(ctx context.Context) ([]CSR, error) {
	data, err := c.getJSONChecked(ctx, "get csrs", "get", "csr", "-o", "json")
	if err != nil {
		return nil, err
	}

	var csrList struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Conditions []json.RawMessage `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}

	if err := json.Unmarshal(data, &csrList); err != nil {
		return nil, &errtypes.ClusterError{Msg: "parse CSRs", Err: err}
	}

	var pendingCSRs []CSR
	for _, item := range csrList.Items {
		if len(item.Status.Conditions) == 0 {
			pendingCSRs = append(pendingCSRs, CSR{Name: item.Metadata.Name})
		}
	}

	return pendingCSRs, nil
}

// ApprovePendingCSRs approves every CSR returned by PendingCSRs in one
// `oc adm certificate approve` invocation and returns the count approved.
func (c *Client) ApprovePendingCSRs(ctx context.Context) (int, error) {
	csrs, err := c.PendingCSRs(ctx)
	if err != nil {
		return 0, err
	}

	if len(csrs) == 0 {
		return 0, nil
	}

	names := make([]string, len(csrs))
	for i, csr := range csrs {
		names[i] = csr.Name
	}

	args := append([]string{"adm", "certificate", "approve"}, names...)
	if err := c.runCheck(ctx, args...); err != nil {
		return 0, &errtypes.ClusterError{Msg: "approve CSRs", Err: err}
	}

	return len(names), nil
}
