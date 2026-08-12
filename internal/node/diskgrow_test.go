package node

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/executor"
)

type fakeDebugRunner struct {
	args   [][]string
	result *executor.Result
	err    error
}

func (f *fakeDebugRunner) Run(_ context.Context, args ...string) (*executor.Result, error) {
	f.args = append(f.args, args)
	return f.result, f.err
}

func TestDebugNodeGrowerCommandShape(t *testing.T) {
	fr := &fakeDebugRunner{result: &executor.Result{ExitCode: 0}}
	g := &DebugNodeGrower{Runner: fr}
	if err := g.GrowOSDisk(t.Context(), "grappleberry-master1"); err != nil {
		t.Fatal(err)
	}
	got := fr.args[0]
	want := []string{
		"debug", "node/grappleberry-master1", "-q", "--",
		"chroot", "/host", "sh", "-c", growOSDiskScript,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("args = %q, want %q", got, want)
	}
}

func TestDebugNodeGrowerNonZeroExitFails(t *testing.T) {
	fr := &fakeDebugRunner{result: &executor.Result{ExitCode: 2, Stderr: "NOCHANGE impossible: bad partition"}}
	g := &DebugNodeGrower{Runner: fr}
	err := g.GrowOSDisk(t.Context(), "n")
	if err == nil || !strings.Contains(err.Error(), "bad partition") {
		t.Fatalf("exit 2 not surfaced: %v", err)
	}
}

func TestGrowOSDiskScriptToleratesNoChange(t *testing.T) {
	// growpart exit 1 (NOCHANGE) must not abort the script: the || guard
	// converts rc=1 to success while any other rc still fails under set -e.
	if !strings.Contains(growOSDiskScript, "|| [ $? -eq 1 ]") {
		t.Fatalf("script lost its NOCHANGE guard:\n%s", growOSDiskScript)
	}
}
