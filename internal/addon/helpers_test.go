package addon

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/errtypes"
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
		{"wrapped context canceled", fmt.Errorf("wrap: %w", context.Canceled), false},
		{"exec not found", exec.ErrNotFound, false},
		{"wrapped exec not found", fmt.Errorf("oc: %w", exec.ErrNotFound), false},
		{"config error", &errtypes.ConfigError{Msg: "bad config"}, false},
		{"wrapped config error", fmt.Errorf("outer: %w", &errtypes.ConfigError{Msg: "bad"}), false},
		{"auth error", &errtypes.AuthError{Msg: "denied"}, false},
		{"wrapped auth error", fmt.Errorf("outer: %w", &errtypes.AuthError{Msg: "denied"}), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := addonIsRetryable(tc.err); got != tc.retryable {
				t.Errorf("addonIsRetryable(%v) = %v; want %v", tc.err, got, tc.retryable)
			}
		})
	}
}

func Test_RetryDefault_NonRetryableAbortsImmediately(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		permErr := &errtypes.ConfigError{Msg: "oc binary missing"}
		err := RetryDefault(context.Background(), func() error {
			calls.Add(1)
			return permErr
		})
		if !errors.Is(err, permErr) {
			t.Errorf("err = %v; want permErr", err)
		}
		if calls.Load() != 1 {
			t.Errorf("calls = %d; want 1 (no retry on non-retryable)", calls.Load())
		}
	})
}

func Test_RetryDefault_SucceedsOnAttemptN(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		err := RetryDefault(context.Background(), func() error {
			n := calls.Add(1)
			if n < 3 {
				return errors.New("not yet")
			}
			return nil
		})
		if err != nil {
			t.Errorf("err = %v; want nil", err)
		}
		if calls.Load() != 3 {
			t.Errorf("calls = %d; want 3", calls.Load())
		}
	})
}

func Test_RetryDefault_AllFailures(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		sentinel := errors.New("always fails")
		err := RetryDefault(context.Background(), func() error {
			calls.Add(1)
			return sentinel
		})
		// lastErr preservation: exhaustion returns the original fn() error,
		// not the wait.ErrWaitTimeout sentinel.
		if !errors.Is(err, sentinel) {
			t.Errorf("err = %v; want sentinel error %v", err, sentinel)
		}
		if calls.Load() != DefaultRetryCount {
			t.Errorf("calls = %d; want %d", calls.Load(), DefaultRetryCount)
		}
	})
}

func Test_RetryDefault_CtxCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			time.Sleep(DefaultRetryBackoff / 2)
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

	// Round-trip: parse the YAML and check structure.
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

	// Values should be base64-encoded, not plaintext.
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
