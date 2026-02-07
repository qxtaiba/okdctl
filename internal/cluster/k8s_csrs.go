package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

// PendingCSRs returns pending certificate signing requests.
func (c *K8sClient) PendingCSRs(ctx context.Context) ([]CSR, error) {
	result, err := c.run(ctx, "get", "csr", "-o", "json")
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, utils.WrapError("failed to get CSRs", errors.New(strings.TrimSpace(result.Stderr)))
	}

	var csrList struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Spec struct {
				Username   string `json:"username"`
				SignerName string `json:"signerName"`
			} `json:"spec"`
			Status struct {
				Conditions []struct {
					Type string `json:"type"`
				} `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}

	if err := json.Unmarshal([]byte(result.Stdout), &csrList); err != nil {
		return nil, utils.WrapError("failed to parse CSRs", err)
	}

	var pendingCSRs []CSR
	for _, item := range csrList.Items {
		approved := false
		denied := false
		for _, cond := range item.Status.Conditions {
			if cond.Type == "Approved" {
				approved = true
			}
			if cond.Type == "Denied" {
				denied = true
			}
		}

		csr := CSR{
			Name:       item.Metadata.Name,
			Requester:  item.Spec.Username,
			SignerName: item.Spec.SignerName,
			Approved:   approved,
			Denied:     denied,
			Pending:    len(item.Status.Conditions) == 0,
		}

		if csr.Pending {
			pendingCSRs = append(pendingCSRs, csr)
		}
	}

	return pendingCSRs, nil
}

// ApproveCSRs approves the specified CSRs.
func (c *K8sClient) ApproveCSRs(ctx context.Context, csrNames []string) error {
	if len(csrNames) == 0 {
		return nil
	}

	args := append([]string{"adm", "certificate", "approve"}, csrNames...)
	return c.runCheck(ctx, args...)
}

// ApprovePendingCSRs finds and approves all pending CSRs.
func (c *K8sClient) ApprovePendingCSRs(ctx context.Context) (int, error) {
	csrs, err := c.PendingCSRs(ctx)
	if err != nil {
		return 0, err
	}

	if len(csrs) == 0 {
		return 0, nil
	}

	var names []string
	for _, csr := range csrs {
		names = append(names, csr.Name)
	}

	if err := c.ApproveCSRs(ctx, names); err != nil {
		return 0, utils.WrapError("failed to approve CSRs", err)
	}

	return len(names), nil
}
