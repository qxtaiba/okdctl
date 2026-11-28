package proxmox

import (
	"net/netip"
	"reflect"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/model"
)

func TestPreflightInputFor(t *testing.T) {
	cfg := config.MinimalConfig()
	cfg.Provider.Proxmox.ISOStorage = ""
	const infraID = "minimal-a1b2c3"
	inv, err := model.Build(cfg, infraID)
	if err != nil {
		t.Fatal(err)
	}
	addr := netip.MustParseAddr

	t.Run("record absent: no tag, no macs, iso storage falls back", func(t *testing.T) {
		got := PreflightInputFor(&inv, &cfg.Provider.Proxmox, "", 0)
		want := PreflightInput{
			Nodes: []string{"pve"}, Storage: "local-lvm", ISOStorage: "local-lvm", Bridge: "vmbr0",
			VMIDs:           []int{6010, 6011, 6012},
			NodeAddresses:   map[netip.Addr]int{addr("192.168.1.150"): 6010, addr("192.168.1.151"): 6011, addr("192.168.1.152"): 6012},
			VIPs:            []netip.Addr{addr("192.168.1.158"), addr("192.168.1.159")},
			MemoryMiBByNode: map[string]int{"pve": 3 * 16384},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("PreflightInputFor() = %+v\nwant %+v", got, want)
		}
	})

	t.Run("record and iso present: tag, macs and size carried", func(t *testing.T) {
		got := PreflightInputFor(&inv, &cfg.Provider.Proxmox, infraID, 3*gibBytes)
		wantMACs := []string{model.MAC(infraID, 6010), model.MAC(infraID, 6011), model.MAC(infraID, 6012)}
		if got.Tag != infraID || got.ISOBytes != 3*gibBytes || !reflect.DeepEqual(got.MACs, wantMACs) {
			t.Fatalf("tag=%q iso=%d macs=%v, want %q, %d, %v", got.Tag, got.ISOBytes, got.MACs, infraID, 3*gibBytes, wantMACs)
		}
	})
}
