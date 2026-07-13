package cluster

import (
	"context"
	"fmt"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// Patch patches a cluster-scoped resource via `oc patch <resource> <name>
// --type=<patchType> -p <patch>`. patchType is typically "merge" or "json".
func (c *Client) Patch(ctx context.Context, resource, name, patchType, patch string) error {
	if err := c.runCheck(ctx, "patch", resource, name, "--type="+patchType, "-p", patch); err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("patch %s/%s", resource, name), Err: err}
	}
	return nil
}
