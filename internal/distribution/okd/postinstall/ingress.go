package postinstall

import (
	"context"
	"strings"
)

// GetGrappleberryRouterIP gets the grappleberry router LoadBalancer IP if it exists.
func (p *Phase) GetGrappleberryRouterIP(ctx context.Context) string {
	result, err := p.Exec.Run(ctx, "oc", "get", "svc", "router-grappleberry",
		"-n", "openshift-ingress",
		"-o", "jsonpath={.status.loadBalancer.ingress[0].ip}")
	if err == nil && result.ExitCode == 0 {
		return strings.TrimSpace(result.Stdout)
	}

	return ""
}
