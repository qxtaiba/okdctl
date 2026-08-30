package dns

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

func TestRestoreSystemResolver(t *testing.T) {
	cases := []struct {
		name      string
		seed      bool
		removeErr error
		wantGone  bool
	}{
		{name: "missing drop-in is a no-op", wantGone: true},
		{name: "present drop-in is removed", seed: true, wantGone: true},
		{name: "RemoveAll error logged not propagated", seed: true, removeErr: errors.New("injected remove error")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			confPath := redirectResolvedConf(t)
			if tc.seed {
				if err := os.MkdirAll(filepath.Dir(confPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(confPath, []byte("[Resolve]\nDNS=127.0.0.1\n"), 0o644); err != nil {
					t.Fatalf("seed drop-in: %v", err)
				}
			}
			if tc.removeErr != nil {
				origFn := removeAllFn
				removeAllFn = func(_ string) error { return tc.removeErr }
				t.Cleanup(func() { removeAllFn = origFn })
			}

			if err := RestoreSystemResolver(context.Background(), logutil.NopLogger); err != nil {
				t.Fatalf("RestoreSystemResolver must not propagate the failure, got: %v", err)
			}

			if tc.wantGone {
				if _, err := os.Stat(confPath); !os.IsNotExist(err) {
					t.Errorf("expected drop-in absent, got stat err: %v", err)
				}
			}
		})
	}
}
