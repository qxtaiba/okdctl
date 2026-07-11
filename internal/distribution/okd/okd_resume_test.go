package okd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

func TestResumePostInstall_MissingKubeconfigFailsFast(t *testing.T) {
	p := New(WithProjectRoot(t.TempDir()), WithLogger(logutil.NopLogger))

	_, _, err := p.ResumePostInstall(context.Background(), config.DefaultConfig())
	if err == nil {
		t.Fatal("expected error when kubeconfig is missing; got nil")
	}
	var cErr *errtypes.ClusterError
	if !errors.As(err, &cErr) {
		t.Errorf("err = %v; want *errtypes.ClusterError", err)
	}
	for _, remedy := range []string{"okdctl destroy", "--fresh"} {
		if !strings.Contains(err.Error(), remedy) {
			t.Errorf("error %q must name remediation %q", err, remedy)
		}
	}
}
