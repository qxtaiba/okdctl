package cluster

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
)

func TestClientPatch_NonZeroExitWrapsClusterError(t *testing.T) {
	installFakeOCGeneric(t)
	t.Setenv("OC_EXIT_CODE", "1")
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	err := c.Patch(context.Background(), "operatorhub.config.openshift.io", "cluster", "merge", `{}`)
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
	var ce *errtypes.ClusterError
	if !errors.As(err, &ce) {
		t.Fatalf("err is %T; want *errtypes.ClusterError", err)
	}
	if !strings.Contains(ce.Msg, "operatorhub.config.openshift.io/cluster") {
		t.Errorf("ClusterError.Msg = %q; want resource/name named", ce.Msg)
	}
}
