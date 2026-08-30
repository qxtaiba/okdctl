package cli

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func TestPromptForConfirmation(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		wantOK bool
	}{
		{"y confirms", "y\n", true},
		{"n denies", "n\n", false},
		{"EOF treated as denial", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testStdinReader = strings.NewReader(tc.input)
			t.Cleanup(func() { testStdinReader = nil })

			ok, err := promptForConfirmation(context.Background(), "")
			if err != nil {
				t.Fatalf("want nil error, got %v", err)
			}
			if ok != tc.wantOK {
				t.Fatalf("want %v, got %v", tc.wantOK, ok)
			}
		})
	}
}

func TestPromptForConfirmation_CtxCancel(t *testing.T) {
	pr, pw := io.Pipe()
	testStdinReader = pr
	t.Cleanup(func() {
		_ = pw.Close()
		_ = pr.Close()
		testStdinReader = nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ok, err := promptForConfirmation(ctx, "")
	if ok {
		t.Fatal("want false, got true")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestIsConfirmResponse(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"y", true},
		{"Y", true},
		{"yes", true},
		{"YES", true},
		{"n", false},
		{"no", false},
		{"", false},
		{"1", false},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			if got := isConfirmResponse(tc.input); got != tc.want {
				t.Fatalf("isConfirmResponse(%q) = %v; want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestConfirmClusterMatches(t *testing.T) {
	cases := []struct {
		name    string
		force   bool
		confirm string
		cluster string
		verb    string
		wantErr bool
	}{
		{"force=false short-circuits", false, "", "prod", "cleanup", false},
		{"empty confirm with force", true, "", "prod", "cleanup", true},
		{"mismatched confirm", true, "staging", "prod", "cleanup", true},
		{"correct confirm", true, "prod", "prod", "cleanup", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := confirmClusterMatches(tc.force, tc.confirm, tc.cluster, tc.verb)
			if tc.wantErr {
				var usageErr *errtypes.UsageError
				if !errors.As(err, &usageErr) {
					t.Fatalf("want *errtypes.UsageError, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("want nil, got %v", err)
			}
		})
	}
}

func TestPromptForClusterNameConfirmation(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"exact match confirms", "prod\n", true},
		{"case mismatch denies", "PROD\n", false},
		{"empty line denies", "\n", false},
		{"EOF treated as denial", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testStdinReader = strings.NewReader(tc.input)
			t.Cleanup(func() { testStdinReader = nil })

			ok, err := promptForClusterNameConfirmation(context.Background(), "prod", "type cluster name to confirm: ")
			if err != nil {
				t.Fatalf("want nil error, got %v", err)
			}
			if ok != tc.want {
				t.Fatalf("want %v, got %v", tc.want, ok)
			}
		})
	}
}
