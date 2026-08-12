package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
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
		name                    string
		memoryMB, cpu, osDiskGB int
		wantErr                 bool
	}{
		{"memory only", 16384, 0, 0, false},
		{"cpu only", 0, 8, 0, false},
		{"os disk only", 0, 0, 100, false},
		{"all set", 16384, 8, 100, false},
		{"neither set", 0, 0, 0, true},
		{"negative values treated as unset", -1, -1, -1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateResizeFlags(tc.memoryMB, tc.cpu, tc.osDiskGB)
			if tc.wantErr {
				var usageErr *errtypes.UsageError
				if !errors.As(err, &usageErr) {
					t.Fatalf("want *errtypes.UsageError, got %v", err)
				}
				if !strings.Contains(usageErr.Msg, "--memory-mb") || !strings.Contains(usageErr.Msg, "--cpu") || !strings.Contains(usageErr.Msg, "--os-disk-gb") {
					t.Fatalf("refusal message missing a flag name: %q", usageErr.Msg)
				}
				return
			}
			if err != nil {
				t.Fatalf("want nil error, got %v", err)
			}
		})
	}
}

func TestEnsureTerraformWorkspace(t *testing.T) {
	t.Run("missing dir returns targeted config error", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "never-deployed")
		err := ensureTerraformWorkspace(dir)
		var cfgErr *errtypes.ConfigError
		if !errors.As(err, &cfgErr) {
			t.Fatalf("want *errtypes.ConfigError, got %v (%T)", err, err)
		}
		if !strings.Contains(cfgErr.Msg, "okdctl deploy") {
			t.Errorf("error must point at 'okdctl deploy', got %q", cfgErr.Msg)
		}
	})

	t.Run("existing dir passes", func(t *testing.T) {
		if err := ensureTerraformWorkspace(t.TempDir()); err != nil {
			t.Fatalf("want nil, got %v", err)
		}
	})
}

// TestNodeAddRequiresRootScoped locks the privilege scoping: only node add
// (which revives the ignition httpd server on the local host) requires root
// and elevates under sudo. The non-host-mutating verbs stay unprivileged so
// `sudo okdctl node <verb>` keeps hitting the reject-root branch.
func TestNodeAddRequiresRootScoped(t *testing.T) {
	// Guard against dry-run flag pollution from a sibling test: requiresRoot
	// short-circuits to false under --dry-run.
	_ = nodeAddCmd.Flags().Set(flagDryRun, "false")

	if nodeAddCmd.Annotations[annotationKeyRequiresRoot] != annotationValueTrue {
		t.Error("node add must carry the requiresRoot annotation")
	}
	if !requiresRoot(nodeAddCmd) {
		t.Error("node add must require root (revives the ignition httpd server)")
	}
	if requiresRoot(nodeRemoveCmd) {
		t.Error("node remove must not require root")
	}
	if requiresRoot(nodeResizeCmd) {
		t.Error("node resize must not require root")
	}
	if requiresRoot(nodeListCmd) {
		t.Error("node list must not require root")
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

// TestDestroyGradeVerbPolicy locks the single source of truth for the
// destroy-grade gate: only VM-destroying verbs escalate to the typed-name
// stage. Flipping a literal at a RunE call site can no longer silently
// downgrade the gate because every site consults this map.
func TestDestroyGradeVerbPolicy(t *testing.T) {
	want := map[string]bool{
		"remove":  true,
		"compact": true,
		"resize":  false,
		"add":     false,
		"stop":    false,
		"start":   false,
	}
	for verb, exp := range want {
		if got := destroyGradeVerb(verb); got != exp {
			t.Errorf("destroyGradeVerb(%q) = %v, want %v", verb, got, exp)
		}
	}
}

// TestDestroyGradeVerbsDenyBareYes drives the real confirm hook for each
// destroy-grade verb with only "y\n" on stdin and asserts denial: a bare
// "y" cannot satisfy the typed-cluster-name stage the destroy-grade gate
// puts ahead of the y/N prompt.
func TestDestroyGradeVerbsDenyBareYes(t *testing.T) {
	for _, verb := range []string{"remove", "compact"} {
		t.Run(verb, func(t *testing.T) {
			if !destroyGradeVerb(verb) {
				t.Fatalf("%s must be a destroy-grade verb for this test to be meaningful", verb)
			}
			testStdinReader = strings.NewReader("y\n")
			t.Cleanup(func() { testStdinReader = nil })

			rc := &nodeRunnerCtx{}
			var errW bytes.Buffer
			hook := nodeConfirmHook(rc, nodeConsent{twoStage: destroyGradeVerb(verb)}, "prod", &errW)

			ok, err := hook(context.Background(), &node.OpPlan{Op: node.OpRemove, Cluster: "prod"})
			if err != nil {
				t.Fatalf("gate error: %v", err)
			}
			if ok {
				t.Fatalf("%s: bare 'y' must not satisfy the destroy-grade typed-name gate", verb)
			}
		})
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
