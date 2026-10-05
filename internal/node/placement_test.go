package node

import (
	"context"
	"errors"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

type relocatedPower struct {
	fakePower
	host string
	err  error
}

func (p *relocatedPower) VMOwner(context.Context, int) (string, error) { return p.host, p.err }

func TestPlacementCapacityUsesEachHostAndObservedOwner(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = "pve1"
	cfg.Provider.Proxmox.WorkerNodes = []string{"pve2"}
	r := &Runner{Cfg: cfg}
	observed := make(map[string]int)
	r.Capacity = func(_ context.Context, host, _ string) (HostCapacity, error) {
		observed[host]++
		if host == "pve2" {
			return HostCapacity{TotalMiB: 16384, AllocatedMiB: 14336, AvailableDiskGB: 100}, nil
		}
		return HostCapacity{TotalMiB: 65536, AvailableDiskGB: 100}, nil
	}
	hosts, err := r.targetHosts(t.Context(), nodetypes.RoleWorker, []string{"worker0", "worker1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if hosts["pve1"] != 1 || hosts["pve2"] != 1 {
		t.Fatalf("placement=%v", hosts)
	}
	if err := r.checkPlacementCapacity(t.Context(), hosts, 4096, 10); err == nil {
		t.Fatal("full second host passed aggregate budget")
	}
	r.Power = &relocatedPower{host: "pve3"}
	hosts, err = r.targetHosts(t.Context(), nodetypes.RoleWorker, []string{"worker0"}, true)
	if err != nil || hosts["pve3"] != 1 {
		t.Fatalf("HA ownership ignored: %v %v", hosts, err)
	}
	r.Power = &relocatedPower{err: errors.New("ownership unknown")}
	if _, err := r.targetHosts(t.Context(), nodetypes.RoleWorker, []string{"worker0"}, true); err == nil {
		t.Fatal("unknown ownership accepted")
	}
}

func TestAddChecksSeparateDataDatastore(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Storage = "os"
	cfg.Provider.Proxmox.DataStorage = "data"
	cfg.Disks.WorkerDataSizeGB = 500
	r := &Runner{Cfg: cfg, Capacity: func(_ context.Context, _, storage string) (HostCapacity, error) {
		capacity := HostCapacity{TotalMiB: 65536, AvailableDiskGB: 1000}
		if storage == "data" {
			capacity.AvailableDiskGB = 100
		}
		return capacity, nil
	}}
	if err := r.checkAddCapacity(t.Context(), map[string]int{"pve1": 1}); err == nil {
		t.Fatal("full data datastore accepted")
	}
	cfg.Provider.Proxmox.DataStorage = "os"
	cfg.Topology.Workers.DiskGB = 600
	if err := r.checkAddCapacity(t.Context(), map[string]int{"pve1": 1}); err == nil {
		t.Fatal("combined disk demand accepted")
	}
}

func TestSharedDatastoreCountsAllDestinationDisks(t *testing.T) {
	cfg := config.DefaultConfig()
	r := &Runner{Cfg: cfg, Capacity: func(context.Context, string, string) (HostCapacity, error) {
		return HostCapacity{TotalMiB: 65536, AvailableDiskGB: 1000, Shared: true}, nil
	}}
	if err := r.checkPlacementCapacity(t.Context(), map[string]int{"pve1": 1, "pve2": 1}, 0, 600); err == nil {
		t.Fatal("shared capacity spent once per host")
	}
}
