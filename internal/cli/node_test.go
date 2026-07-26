package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/node"
)

func TestValidateAddFlags(t *testing.T) {
	cases := []struct {
		name    string
		count   int
		wantErr bool
	}{
		{"default count", 1, false},
		{"batch count", 3, false},
		{"zero count rejected", 0, true},
		{"negative count rejected", -1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAddFlags(tc.count)
			if tc.wantErr {
				var usageErr *errtypes.UsageError
				if !errors.As(err, &usageErr) {
					t.Fatalf("want *errtypes.UsageError, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("want nil error, got %v", err)
			}
		})
	}
}

func TestValidateResizeFlags(t *testing.T) {
	cases := []struct {
		name          string
		memoryMB, cpu int
		wantErr       bool
	}{
		{"memory only", 16384, 0, false},
		{"cpu only", 0, 8, false},
		{"both set", 16384, 8, false},
		{"neither set", 0, 0, true},
		{"negative values treated as unset", -1, -1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateResizeFlags(tc.memoryMB, tc.cpu)
			if tc.wantErr {
				var usageErr *errtypes.UsageError
				if !errors.As(err, &usageErr) {
					t.Fatalf("want *errtypes.UsageError, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("want nil error, got %v", err)
			}
		})
	}
}

func TestNodeConfirmHookYesPrintsBoxSkipsGate(t *testing.T) {
	rc := &nodeRunnerCtx{}
	var errW bytes.Buffer
	hook := nodeConfirmHook(rc, nodeConsent{yes: true, twoStage: true}, "prod", &errW)

	ok, hookErr := hook(context.Background(), &node.OpPlan{Op: node.OpRemove, Cluster: "prod"})

	if hookErr != nil || !ok {
		t.Fatalf("--yes must proceed without prompting: ok=%v err=%v", ok, hookErr)
	}
	if rc.captured == nil {
		t.Error("confirm hook must capture the plan for the completion box")
	}
	if out := errW.String(); !strings.Contains(out, "confirm worker removal") || !strings.Contains(out, "prod") {
		t.Errorf("--yes still prints the informed box; got:\n%s", out)
	}
}

func TestRunNodeGateSingleYN(t *testing.T) {
	testStdinReader = strings.NewReader("y\n")
	t.Cleanup(func() { testStdinReader = nil })

	ok, err := runNodeGate(context.Background(), false, "prod")
	if err != nil || !ok {
		t.Fatalf("single y/N gate: ok=%v err=%v", ok, err)
	}
}

func TestRunNodeGateTwoStageWrongNameDenies(t *testing.T) {
	testStdinReader = strings.NewReader("staging\n")
	t.Cleanup(func() { testStdinReader = nil })

	ok, err := runNodeGate(context.Background(), true, "prod")
	if err != nil {
		t.Fatalf("wrong name should deny without error, got %v", err)
	}
	if ok {
		t.Fatal("mistyped cluster name must deny the destroy-grade gate")
	}
}

func TestRunNodeGateTwoStageHappyPath(t *testing.T) {
	pr, pw := io.Pipe()
	testStdinReader = pr
	t.Cleanup(func() {
		_ = pr.Close()
		testStdinReader = nil
	})
	go func() {
		_, _ = pw.Write([]byte("prod\n"))
		_, _ = pw.Write([]byte("y\n"))
		_ = pw.Close()
	}()

	ok, err := runNodeGate(context.Background(), true, "prod")
	if err != nil || !ok {
		t.Fatalf("typed name + y should proceed: ok=%v err=%v", ok, err)
	}
}
