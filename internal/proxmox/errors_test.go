package proxmox

import (
	"errors"
	"fmt"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func TestErrPermissionIsATypedAuthError(t *testing.T) {
	err := fmt.Errorf("create vm: %w", ErrPermission)
	d, ok := errtypes.Describe(err)
	if !ok || d.Kind != errtypes.KindAuth {
		t.Fatalf("Describe(%v) = %+v ok=%v, want kind auth", err, d, ok)
	}
	if !errors.Is(err, ErrPermission) {
		t.Fatalf("errors.Is(%v, ErrPermission) = false", err)
	}
}
