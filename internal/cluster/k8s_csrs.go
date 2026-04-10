package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// PendingCSRs returns CSRs whose status.conditions slice is empty, which
// is how the kube-controller-manager represents a request that has neither
// been approved nor denied. An Approved or Denied CSR carries at least one
// condition entry and is excluded.
func (c *K8sClient) PendingCSRs(ctx context.Context) ([]CSR, error) {
	result, err := c.run(ctx, "get", "csr", "-o", "json")
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("failed to get CSRs: %s", strings.TrimSpace(result.Stderr))
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

	if err := json.Unmarshal([]byte(result.Stdout), &csrList); err != nil {
		return nil, fmt.Errorf("failed to parse CSRs: %w", err)
	}

	var pendingCSRs []CSR
	for _, item := range csrList.Items {
		if len(item.Status.Conditions) == 0 {
			pendingCSRs = append(pendingCSRs, CSR{
				Name:    item.Metadata.Name,
				Pending: true,
			})
		}
	}

	return pendingCSRs, nil
}

func (c *K8sClient) ApprovePendingCSRs(ctx context.Context) (int, error) {
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
		return 0, fmt.Errorf("failed to approve CSRs: %w", err)
	}

	return len(names), nil
}
