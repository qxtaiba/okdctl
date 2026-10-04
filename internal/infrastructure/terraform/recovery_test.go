package terraform

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type recoverySource struct {
	err      error
	backup   string
	snapshot bool
}

func (s *recoverySource) SnapshotState(context.Context) (string, error) {
	s.snapshot = true
	return s.backup, s.err
}
func (*recoverySource) WithLockHint(err error) error { return err }

func TestRecoveryBackupFailurePreventsMutation(t *testing.T) {
	failure := errors.New("backup unavailable")
	source := &recoverySource{err: failure}
	called := false
	err := WithStateRecovery(t.Context(), source, "apply", func() error { called = true; return nil })
	if called || !errors.Is(err, failure) {
		t.Fatalf("mutation=%v err=%v", called, err)
	}
}

func TestRecoveryPreservesBackupAndCancellation(t *testing.T) {
	source := &recoverySource{backup: "/workspace/state.bak"}
	err := WithStateRecovery(t.Context(), source, "destroy", func() error {
		if !source.snapshot {
			t.Fatal("mutation preceded backup")
		}
		return context.Canceled
	})
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), source.backup) {
		t.Fatalf("lost recovery context: %v", err)
	}
}
