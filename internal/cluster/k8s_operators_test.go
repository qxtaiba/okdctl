package cluster

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

func TestParseOperatorHealth(t *testing.T) {
	data := `{"items":[
	  {"metadata":{"name":"authentication"},"status":{"conditions":[
	    {"type":"Available","status":"True"},{"type":"Degraded","status":"False"},{"type":"Progressing","status":"False"}]}},
	  {"metadata":{"name":"ingress"},"status":{"conditions":[
	    {"type":"Available","status":"True"},{"type":"Degraded","status":"True"}]}},
	  {"metadata":{"name":"kube-apiserver"},"status":{"conditions":[
	    {"type":"Available","status":"True"},{"type":"Progressing","status":"True"}]}}
	]}`
	h, err := parseOperatorHealth([]byte(data))
	if err != nil {
		t.Fatalf("parseOperatorHealth: %v", err)
	}
	if h.Total != 3 || h.Available != 3 {
		t.Errorf("Total/Available = %d/%d; want 3/3", h.Total, h.Available)
	}
	if !slices.Equal(h.Degraded, []string{"ingress"}) {
		t.Errorf("Degraded = %v; want [ingress]", h.Degraded)
	}
	if !slices.Equal(h.Progressing, []string{"kube-apiserver"}) {
		t.Errorf("Progressing = %v; want [kube-apiserver]", h.Progressing)
	}
}

func TestClusterOperatorHealth_NonZeroExit(t *testing.T) {
	installFakeOCGeneric(t)
	t.Setenv("OC_EXIT_CODE", "1")

	c := New(WithCLI("oc"), WithLogger(logutil.NopLogger))
	_, err := c.ClusterOperatorHealth(context.Background())
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
	var ce *errtypes.ClusterError
	if !errors.As(err, &ce) {
		t.Fatalf("err is %T; want *errtypes.ClusterError", err)
	}
}
