package cluster

import (
	"context"
	"errors"
	"testing"

	"github.com/qxtaiba/okdctl/internal/executor"
)

func TestClientRawGet_Success(t *testing.T) {
	installFakeOCGeneric(t)
	t.Setenv("OC_STDOUT", "  healthz ok  \n")
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	got, err := c.RawGet(context.Background(), "/healthz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "healthz ok"; got != want {
		t.Errorf("RawGet = %q; want %q", got, want)
	}
}

func TestClientRawGet_NonZeroExit(t *testing.T) {
	installFakeOCGeneric(t)
	t.Setenv("OC_EXIT_CODE", "1")
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	_, err := c.RawGet(context.Background(), "/healthz")
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
	var exitErr *executor.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("err is %T; want *executor.ExitError", err)
	}
}

func TestClientGetJSON_Success(t *testing.T) {
	installFakeOCGeneric(t)
	t.Setenv("OC_STDOUT", `{"ok":true}`)
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	stdout, truncated, err := c.GetJSON(context.Background(), "get", "foo", "-o", "json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if truncated {
		t.Error("truncated = true; want false")
	}
	if want := `{"ok":true}`; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}
}

func TestClientGetJSON_NonZeroExit(t *testing.T) {
	installFakeOCGeneric(t)
	t.Setenv("OC_EXIT_CODE", "1")
	c := New(WithCLI("oc"), WithExecutor(executor.New()))

	_, _, err := c.GetJSON(context.Background(), "get", "foo")
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
	var exitErr *executor.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("err is %T; want *executor.ExitError", err)
	}
}
