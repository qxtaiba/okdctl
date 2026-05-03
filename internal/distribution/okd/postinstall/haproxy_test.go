package postinstall

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// TestRemoveHAProxy_EmptyVIPSkipsVerify exercises the vip=="" short-circuit
// — both verify blocks (vip + hostname) are gated behind `if vip != ""`,
// so the empty-vip path does no subprocess work beyond the (darwin-noop)
// systemctl probes and the os.RemoveAll on haproxyConfigPath. Verifies the
// new test seam (haproxyConfigPath var) works.
func TestRemoveHAProxy_EmptyVIPSkipsVerify(t *testing.T) {
	origConfig := haproxyConfigPath
	t.Cleanup(func() { haproxyConfigPath = origConfig })
	haproxyConfigPath = filepath.Join(t.TempDir(), "haproxy.cfg")

	p := New(executor.New(), logutil.NopLogger, "test")
	if err := p.RemoveHAProxy(context.Background(), "", t.TempDir()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
