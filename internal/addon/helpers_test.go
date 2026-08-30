package addon

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

func Test_addonIsRetryable(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"nil", nil, false},
		{"generic transient", errors.New("connection refused"), true},
		{"context canceled", context.Canceled, false},
		{"context deadline", context.DeadlineExceeded, false},
		{"exec not found", exec.ErrNotFound, false},
		{"config error", &errtypes.ConfigError{Msg: "bad config"}, false},
		{"auth error", &errtypes.AuthError{Msg: "denied"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := addonIsRetryable(tc.err); got != tc.retryable {
				t.Errorf("addonIsRetryable(%v) = %v; want %v", tc.err, got, tc.retryable)
			}
		})
	}
}

func Test_RetryDefault(t *testing.T) {
	permErr := &errtypes.ConfigError{Msg: "oc binary missing"}
	sentinel := errors.New("always fails")
	cases := []struct {
		name string
		// fn receives the 1-based attempt number.
		fn func(attempt int32) error
		// wantErrIs is the errors.Is target for the returned error; nil wants
		// success. Exhaustion returns the original fn() error (lastErr
		// preservation), not the wait.ErrWaitTimeout sentinel.
		wantErrIs error
		wantCalls int
	}{
		{"non-retryable aborts immediately", func(int32) error { return permErr }, permErr, 1},
		{"succeeds on attempt n", func(n int32) error {
			if n < 3 {
				return errors.New("not yet")
			}
			return nil
		}, nil, 3},
		{"all failures returns last error", func(int32) error { return sentinel }, sentinel, system.DefaultBackoff().Steps},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var calls atomic.Int32
				err := RetryDefault(context.Background(), func() error {
					return tc.fn(calls.Add(1))
				})
				if tc.wantErrIs == nil {
					if err != nil {
						t.Errorf("err = %v; want nil", err)
					}
				} else if !errors.Is(err, tc.wantErrIs) {
					t.Errorf("err = %v; want errors.Is(_, %v)", err, tc.wantErrIs)
				}
				if int(calls.Load()) != tc.wantCalls {
					t.Errorf("calls = %d; want %d", calls.Load(), tc.wantCalls)
				}
			})
		})
	}
}

func Test_RetryDefault_CtxCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			time.Sleep(system.DefaultBackoff().Duration / 2)
			cancel()
		}()
		err := RetryDefault(ctx, func() error {
			return errors.New("fail")
		})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v; want context.Canceled", err)
		}
	})
}

func TestBuildOpaqueSecret(t *testing.T) {
	data := map[string][]byte{
		"username": []byte("root@pam"),
		"password": []byte("hunter2"),
	}
	out, err := BuildOpaqueSecret("flux-system", "creds", data)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("emitted YAML does not re-parse: %v\n%s", err, out)
	}

	if parsed["apiVersion"] != "v1" {
		t.Errorf("apiVersion = %v, want v1", parsed["apiVersion"])
	}
	if parsed["kind"] != "Secret" {
		t.Errorf("kind = %v, want Secret", parsed["kind"])
	}
	if parsed["type"] != "Opaque" {
		t.Errorf("type = %v, want Opaque", parsed["type"])
	}
	meta, _ := parsed["metadata"].(map[string]any)
	if meta == nil {
		t.Fatal("metadata missing")
	}
	if meta["name"] != "creds" || meta["namespace"] != "flux-system" {
		t.Errorf("metadata name/namespace wrong: %+v", meta)
	}

	if strings.Contains(out, "hunter2") {
		t.Errorf("secret body leaks plaintext: %s", out)
	}
	if strings.Contains(out, "root@pam") {
		t.Errorf("secret body leaks plaintext username: %s", out)
	}
}

func TestBuildOpaqueSecret_Empty(t *testing.T) {
	out, err := BuildOpaqueSecret("ns", "name", map[string][]byte{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "kind: Secret") {
		t.Errorf("empty-data secret missing kind: %s", out)
	}
}
