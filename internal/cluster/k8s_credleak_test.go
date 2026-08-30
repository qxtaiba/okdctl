package cluster

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/executor"
)

const secretArg = "--from-literal=password=s3cr3t"

func TestClientRun_TransportErrorNoArgvLeak(t *testing.T) {
	c := New(WithCLI("okdctl-definitely-not-on-path-xyz"), WithExecutor(executor.New()))

	_, err := c.Run(context.Background(), "create", "secret", "generic", "s", secretArg)
	if err == nil {
		t.Fatal("expected transport error for missing binary")
	}
	if !strings.Contains(err.Error(), "create") {
		t.Errorf("error lost the subcommand: %q", err.Error())
	}
	if strings.Contains(err.Error(), "s3cr3t") {
		t.Errorf("secret-bearing argv leaked into transport error: %q", err.Error())
	}
}

func TestRunCheck_ExitErrorCommandBounded(t *testing.T) {
	installFakeOCGeneric(t)
	t.Setenv("OC_EXIT_CODE", "1")
	t.Setenv("OC_STDOUT", "")
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	err := c.runCheck(context.Background(), "create", "secret", "generic", "s", secretArg)
	if err == nil {
		t.Fatal("expected error for oc exit 1")
	}
	var exitErr *executor.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("err type = %T; want *executor.ExitError", err)
	}
	if exitErr.Command != "oc create" {
		t.Errorf("ExitError.Command = %q; want exactly %q", exitErr.Command, "oc create")
	}
	if strings.Contains(err.Error(), "s3cr3t") {
		t.Errorf("secret-bearing argv leaked into ExitError: %q", err.Error())
	}
}
