package provision

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

func TestSharedISOPlacementRejectsMissingDestination(t *testing.T) {
	testutil.InstallFakeBin(t, "ssh-keyscan", "#!/bin/sh\nexit 0\n")
	testutil.InstallFakeBin(t, "ssh", `#!/bin/sh
case "$*" in
 *"/nodes/pve1/storage/shared/content"*) printf '%s' '[{"volid":"shared:iso/worker0.iso"}]';;
 *"/nodes/pve2/storage/shared/content"*) printf '%s' '[]';;
 *"/nodes/"*"/storage"*) printf '%s' '[{"storage":"shared","enabled":1,"active":1}]';;
 *"/storage/shared"*) printf '%s' '{"type":"nfs","content":"iso"}';;
 *) exit 1;;
esac
`)
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = "pve1"
	cfg.Provider.Proxmox.ISOStorage = "shared"
	cfg.Provider.Proxmox.WorkerNodes = []string{"pve2"}
	p := New(phase.WithExecutor(newUploadExecutor()))
	if err := p.ValidateISOPlacement(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	if err := p.verifySharedISOs(t.Context(), cfg, "pve", "", []string{"worker0.iso"}); err == nil {
		t.Fatal("ISO absent at destination accepted")
	}
}

func TestSharedISOPlacementRejectsLocalStorage(t *testing.T) {
	testutil.InstallFakeBin(t, "ssh-keyscan", "#!/bin/sh\nexit 0\n")
	testutil.InstallFakeBin(t, "ssh", `#!/bin/sh
printf '%s' '{"type":"dir","shared":1,"content":"iso"}'
`)
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.WorkerNodes = []string{"other-host"}
	p := New(phase.WithExecutor(newUploadExecutor()))
	if err := p.ValidateISOPlacement(t.Context(), cfg); err == nil {
		t.Fatal("shared flag on local directory accepted as shared storage")
	}
}
