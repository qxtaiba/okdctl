package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
)

// PendingCSRs returns requests without approval or denial conditions.
func (c *Client) PendingCSRs(ctx context.Context) ([]CSR, error) {
	data, err := c.getJSONChecked(ctx, "get csrs", "get", "csr", "-o", "json")
	if err != nil {
		return nil, err
	}
	var list certificatesv1.CertificateSigningRequestList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, &errtypes.ClusterError{Msg: "parse CSRs", Err: err}
	}
	var pending []CSR
	for i := range list.Items {
		item := &list.Items[i]
		if len(item.Status.Conditions) == 0 {
			pending = append(pending, CSR{Name: item.Name, Request: *item})
		}
	}
	return pending, nil
}

// ApprovePendingCSRs approves only requests matching an explicit operation identity.
// An empty identity set approves nothing; conflicts are left for the next policy-checked poll.
func (c *Client) ApprovePendingCSRs(ctx context.Context, identities ...CSRIdentity) (int, error) {
	csrs, err := c.PendingCSRs(ctx)
	if err != nil {
		return 0, err
	}
	approved := 0
	for i := range csrs {
		request := &csrs[i].Request
		if err := validateCSR(request, identities); err != nil {
			c.logger.Debug("leave unrelated csr pending", "csr", request.Name, "reason", err)
			continue
		}
		if err := c.approveCSR(ctx, request); err != nil {
			return approved, err
		}
		approved++
	}
	return approved, nil
}

func (c *Client) approveCSR(ctx context.Context, request *certificatesv1.CertificateSigningRequest) error {
	request.APIVersion = "certificates.k8s.io/v1"
	request.Kind = "CertificateSigningRequest"
	request.Status.Conditions = append(request.Status.Conditions, certificatesv1.CertificateSigningRequestCondition{
		Type: certificatesv1.CertificateApproved, Status: corev1.ConditionTrue,
		Reason: "ExpectedNode", Message: "Request matches the active node operation", LastUpdateTime: metav1.Now(),
	})
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode csr approval: %w", err)
	}
	path := "/apis/certificates.k8s.io/v1/certificatesigningrequests/" + url.PathEscape(request.Name) + "/approval"
	result, err := c.exec.RunWithStdin(ctx, string(body), c.CLI, "replace", "--raw", path, "-f", "-")
	if err != nil {
		return &errtypes.ClusterError{Msg: "approve CSRs", Err: err}
	}
	if result.ExitCode != 0 {
		return &errtypes.ClusterError{Msg: "approve CSRs", Err: executor.NewExitError(ctx, c.CLI+" replace csr approval", result.ExitCode, result.Stderr)}
	}
	return nil
}
