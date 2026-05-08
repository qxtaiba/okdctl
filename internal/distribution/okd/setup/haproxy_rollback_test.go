package setup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAttemptHAProxyRollback(t *testing.T) {
	errCause := errors.New("original cause")
	errWrite := errors.New("write failed")
	errRestart := errors.New("restart failed")

	cases := []struct {
		name        string
		writeFn     func(string, string, os.FileMode) error
		restartFn   func() error
		wantCause   bool
		wantWrite   bool
		wantRestart bool
	}{
		{
			name:      "restore write fails",
			writeFn:   func(_, _ string, _ os.FileMode) error { return errWrite },
			restartFn: func() error { t.Error("restartFn must not be called when write fails"); return nil },
			wantCause: true,
			wantWrite: true,
		},
		{
			name:        "restore ok restart fails",
			writeFn:     func(_, _ string, _ os.FileMode) error { return nil },
			restartFn:   func() error { return errRestart },
			wantCause:   true,
			wantRestart: true,
		},
		{
			name:      "happy rollback returns cause",
			writeFn:   func(_, _ string, _ os.FileMode) error { return nil },
			restartFn: func() error { return nil },
			wantCause: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "haproxy.cfg")
			backupPath := filepath.Join(dir, "haproxy.cfg.backup")
			if err := os.WriteFile(backupPath, []byte("old config"), 0o644); err != nil {
				t.Fatalf("write backup: %v", err)
			}

			got := attemptHAProxyRollback(errCause, cfgPath, backupPath, tc.writeFn, tc.restartFn)

			if tc.wantCause && !errors.Is(got, errCause) {
				t.Errorf("errors.Is(got, errCause) = false; got: %v", got)
			}
			if tc.wantWrite && !errors.Is(got, errWrite) {
				t.Errorf("errors.Is(got, errWrite) = false; got: %v", got)
			}
			if tc.wantRestart && !errors.Is(got, errRestart) {
				t.Errorf("errors.Is(got, errRestart) = false; got: %v", got)
			}
			if !tc.wantWrite && errors.Is(got, errWrite) {
				t.Errorf("unexpected errWrite in result: %v", got)
			}
			if !tc.wantRestart && errors.Is(got, errRestart) {
				t.Errorf("unexpected errRestart in result: %v", got)
			}
		})
	}
}
