package cluster

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// signerNamespace and signerName locate the CA whose expiry gates kubelet
// certificate rotation. When kube-apiserver-to-kubelet-signer expires,
// kubelets can no longer complete their CSRs and nodes drift to NotReady —
// the day-2 doctor check surfaces the runway before that happens.
const (
	signerNamespace = "openshift-kube-apiserver-operator"
	signerName      = "kube-apiserver-to-kubelet-signer"
)

// SignerNotAfter returns the NotAfter of the kube-apiserver-to-kubelet-signer
// CA, parsed from the tls.crt of its secret in the
// openshift-kube-apiserver-operator namespace.
func (c *Client) SignerNotAfter(ctx context.Context) (time.Time, error) {
	data, err := c.getJSONChecked(ctx, "get kubelet signer",
		"get", "secret", signerName, "-n", signerNamespace, "-o", "json")
	if err != nil {
		return time.Time{}, err
	}
	notAfter, err := parseSignerNotAfter(data)
	if err != nil {
		return time.Time{}, &errtypes.ClusterError{Msg: "parse kubelet signer", Err: err}
	}
	return notAfter, nil
}

func parseSignerNotAfter(data []byte) (time.Time, error) {
	var secret struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(data, &secret); err != nil {
		return time.Time{}, fmt.Errorf("parse secret json: %w", err)
	}
	b64 := secret.Data["tls.crt"]
	if b64 == "" {
		return time.Time{}, fmt.Errorf("secret carries no tls.crt")
	}
	pemBytes, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return time.Time{}, fmt.Errorf("decode tls.crt base64: %w", err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return time.Time{}, fmt.Errorf("no pem block in tls.crt")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse certificate: %w", err)
	}
	return cert.NotAfter, nil
}
