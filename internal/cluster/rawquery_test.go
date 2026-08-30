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

func TestRawQuery_NonZeroExitWrapsExitError(t *testing.T) {
	tests := []struct {
		name string
		call func(ctx context.Context, c *Client) error
	}{
		{"RawGet", func(ctx context.Context, c *Client) error {
			_, err := c.RawGet(ctx, "/healthz")
			return err
		}},
		{"GetJSON", func(ctx context.Context, c *Client) error {
			_, _, err := c.GetJSON(ctx, "get", "foo")
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			installFakeOCGeneric(t)
			t.Setenv("OC_EXIT_CODE", "1")
			c := New(WithCLI("oc"), WithExecutor(executor.New()))

			err := tc.call(context.Background(), c)
			if err == nil {
				t.Fatal("expected error on non-zero exit")
			}
			var exitErr *executor.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("err is %T; want *executor.ExitError", err)
			}
		})
	}
}
