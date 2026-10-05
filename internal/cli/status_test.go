package cli

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

func TestValidateFormat_InvalidIsUsageError(t *testing.T) {
	err := validateFormat("yaml")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	var ue *errtypes.UsageError
	if !errors.As(err, &ue) {
		t.Fatalf("expected *errtypes.UsageError, got %T: %v", err, err)
	}
}

func TestValidateFormat_ValidReturnsNil(t *testing.T) {
	for _, f := range []string{outputText, outputJSON} {
		if err := validateFormat(f); err != nil {
			t.Errorf("validateFormat(%q) = %v, want nil", f, err)
		}
	}
}

func TestPrintClusterStatusIncludesTableAndFooterCounts(t *testing.T) {
	st := &okd.ClusterStatus{
		Phase:        okd.PhaseDegraded,
		APIReachable: true,
		Nodes: []okd.NodeStatus{
			{Name: "master-0", Role: nodetypes.RoleMaster, Ready: true},
			{Name: "worker-0", Role: nodetypes.RoleWorker, Ready: false},
		},
		DegradedOperators: 1,
	}

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	if err := printClusterStatus(cmd, st); err != nil {
		t.Fatalf("printClusterStatus: %v", err)
	}
	out := buf.String()

	for _, want := range []string{"master-0", "worker-0", "masters", "workers", "total", strconv.Itoa(len(st.Nodes))} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintClusterStatusNoNodesReported(t *testing.T) {
	st := &okd.ClusterStatus{Phase: okd.PhaseUnknown}

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	if err := printClusterStatus(cmd, st); err != nil {
		t.Fatalf("printClusterStatus: %v", err)
	}
	if !strings.Contains(buf.String(), "no nodes reported") {
		t.Errorf("status output missing empty-nodes message:\n%s", buf.String())
	}
}

func TestPrintClusterStatusEndsWithOneBlankLine(t *testing.T) {
	st := &okd.ClusterStatus{Phase: okd.PhaseUnknown}

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	if err := printClusterStatus(cmd, st); err != nil {
		t.Fatalf("printClusterStatus: %v", err)
	}
	out := buf.String()

	if !strings.HasSuffix(out, "╯\n\n") {
		t.Errorf("status box must end with exactly one blank line after it:\n%q", out)
	}
	if strings.HasSuffix(out, "╯\n\n\n") {
		t.Errorf("status box must not print more than one blank line after it:\n%q", out)
	}
}
