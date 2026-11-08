//go:build pveproof

package proxmox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	proofTag     = "okdctl-proof"
	proofTimeout = 3 * time.Minute
)

type proofEnv struct {
	endpoint   string
	token      []byte
	insecure   bool
	node       string
	storage    string
	isoStorage string
	bridge     string
	iso        string
	pool       string
	vmid       int
}

func loadProofEnv(t *testing.T) *proofEnv {
	t.Helper()
	endpoint, token := os.Getenv("PVEPROOF_ENDPOINT"), os.Getenv("PVEPROOF_TOKEN")
	if endpoint == "" || token == "" {
		t.Skip("PVEPROOF_ENDPOINT or PVEPROOF_TOKEN unset")
	}
	env := &proofEnv{
		endpoint:   endpoint,
		token:      []byte(token),
		insecure:   true,
		node:       envOr("PVEPROOF_NODE", "pve"),
		storage:    envOr("PVEPROOF_STORAGE", "local-lvm"),
		isoStorage: envOr("PVEPROOF_ISO_STORAGE", "local"),
		bridge:     envOr("PVEPROOF_BRIDGE", "vmbr0"),
		iso:        os.Getenv("PVEPROOF_ISO"),
		pool:       os.Getenv("PVEPROOF_POOL"),
		vmid:       9900,
	}
	if v := os.Getenv("PVEPROOF_INSECURE"); v != "" {
		insecure, err := strconv.ParseBool(v)
		if err != nil {
			t.Fatalf("PVEPROOF_INSECURE=%q: %v", v, err)
		}
		env.insecure = insecure
	}
	if v := os.Getenv("PVEPROOF_VMID"); v != "" {
		vmid, err := strconv.Atoi(v)
		if err != nil {
			t.Fatalf("PVEPROOF_VMID=%q: %v", v, err)
		}
		env.vmid = vmid
	}
	return env
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func TestProof(t *testing.T) {
	env := loadProofEnv(t)
	d, err := New(env.endpoint, env.token, env.insecure)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	if left := leftovers(t, d, env); len(left) > 0 {
		t.Fatalf("host carries %s before the proof; refusing to run", strings.Join(left, ", "))
	}
	t.Run("Upload", func(t *testing.T) { proofUpload(t, d, env) })
	t.Run("CreateUnderToken", func(t *testing.T) { proofCreateUnderToken(t, d, env, env.vmid+1) })
	t.Run("ShutdownSemantics", func(t *testing.T) { proofShutdownSemantics(t, d, env, env.vmid+2) })
	t.Run("HA", func(t *testing.T) { proofHA(t, d, env, env.vmid+2) })
	if left := leftovers(t, d, env); len(left) > 0 {
		t.Errorf("left on host after the proof: %s", strings.Join(left, ", "))
	} else {
		t.Logf("vms %d-%d, their ha resources and the proof iso absent", env.vmid, env.vmid+2)
	}
}

func proofUpload(t *testing.T, d Driver, env *proofEnv) {
	if env.iso == "" {
		t.Skip("PVEPROOF_ISO unset")
	}
	ctx := t.Context()
	name := fmt.Sprintf("okdctl-proof-%d.iso", env.vmid)
	volid := env.isoStorage + ":iso/" + name
	// PVE names the volume after the uploaded file, so the ISO is linked under the proof name.
	path := filepath.Join(t.TempDir(), name)
	if err := os.Symlink(env.iso, path); err != nil {
		t.Fatalf("link iso: %v", err)
	}
	sum, size := sha256File(t, env.iso)
	if v := lookupVolume(ctx, t, d, env, volid); v != nil {
		t.Fatalf("volume %s already exists (%d bytes); refusing to touch it", volid, v.Size)
	}
	t.Cleanup(func() { cleanupVolume(t, d, env, volid) })

	began := time.Now()
	if err := finish(ctx, d)(d.UploadISO(ctx, env.node, env.isoStorage, path, sum)); err != nil {
		t.Fatalf("UploadISO(): %v", err)
	}
	t.Logf("uploaded %d bytes as %s in %s", size, volid, time.Since(began).Round(time.Second))
	v := lookupVolume(ctx, t, d, env, volid)
	if v == nil {
		t.Fatalf("ListISOs() lacks %s after upload", volid)
	}
	if v.Size != size {
		t.Errorf("ListISOs() size = %d, want %d", v.Size, size)
	}
	if err := finish(ctx, d)(d.DeleteVolume(ctx, env.node, env.isoStorage, volid)); err != nil {
		t.Fatalf("DeleteVolume(): %v", err)
	}
	if v := lookupVolume(ctx, t, d, env, volid); v != nil {
		t.Fatalf("ListISOs() still lists %s after delete", volid)
	}
}

func proofCreateUnderToken(t *testing.T, d Driver, env *proofEnv, vmid int) {
	ctx := t.Context()
	ref := VMRef{Node: env.node, VMID: vmid}
	spec := proofSpec(env, vmid, 2, 2048,
		DiskSpec{Slot: "scsi0", Storage: env.storage, SizeGiB: 1, Serial: "proof-root"},
		DiskSpec{Slot: "scsi1", Storage: env.storage, SizeGiB: 1, Serial: "proof-data"},
	)
	began := time.Now()
	createVM(t, d, ref, &spec)
	t.Logf("created vm %d in %s", vmid, time.Since(began).Round(time.Second))
	cfg, err := d.VMConfig(ctx, ref)
	if err != nil {
		t.Fatalf("VMConfig(): %v", err)
	}
	t.Logf("config: %+v efi: %+v", cfg, cfg.EFI)
	if changes := cfg.Diff(spec); len(changes) != 0 {
		t.Errorf("Diff(spec) = %+v, want none", changes)
	}
	if err := finish(ctx, d)(d.DeleteVM(ctx, ref)); err != nil {
		t.Fatalf("DeleteVM(): %v", err)
	}
	if _, err := d.VMConfig(ctx, ref); !errors.Is(err, ErrNotFound) {
		t.Fatalf("VMConfig() after delete = %v, want ErrNotFound", err)
	}
}

func proofShutdownSemantics(t *testing.T, d Driver, env *proofEnv, vmid int) {
	ctx := t.Context()
	ref := VMRef{Node: env.node, VMID: vmid}
	// A blank disk gives the firmware nothing to boot, so no guest ever answers ACPI or the agent.
	spec := proofSpec(env, vmid, 1, 1024, DiskSpec{Slot: "scsi0", Storage: env.storage, SizeGiB: 1, Serial: "proof-blank"})
	createVM(t, d, ref, &spec)
	if err := finish(ctx, d)(d.Start(ctx, ref)); err != nil {
		t.Fatalf("Start(): %v", err)
	}
	if st := status(t, d, ref); st.Status != "running" {
		t.Fatalf("Status() after start = %+v, want running", st)
	}

	began := time.Now()
	err := finish(ctx, d)(d.Shutdown(ctx, ref, 30*time.Second, false))
	var failed *TaskFailed
	if !errors.As(err, &failed) {
		t.Fatalf("Shutdown(30s, false) = %v (%T), want *TaskFailed", err, err)
	}
	if !strings.Contains(failed.ExitStatus, "timeout") {
		t.Errorf("Shutdown(30s, false) exit status %q, want the timeout text", failed.ExitStatus)
	}
	t.Logf("graceful shutdown failed after %s: %s", time.Since(began).Round(time.Second), failed.ExitStatus)

	began = time.Now()
	if err := finish(ctx, d)(d.Shutdown(ctx, ref, 30*time.Second, true)); err != nil {
		t.Fatalf("Shutdown(30s, true): %v", err)
	}
	t.Logf("forced shutdown succeeded after %s", time.Since(began).Round(time.Second))
	if st := status(t, d, ref); st.Status != "stopped" {
		t.Fatalf("Status() after forced shutdown = %+v, want stopped", st)
	}
	if err := finish(ctx, d)(d.DeleteVM(ctx, ref)); err != nil {
		t.Fatalf("DeleteVM(): %v", err)
	}
}

func proofHA(t *testing.T, d Driver, env *proofEnv, vmid int) {
	ctx := t.Context()
	ref := VMRef{Node: env.node, VMID: vmid}
	spec := proofSpec(env, vmid, 1, 1024, DiskSpec{Slot: "scsi0", Storage: env.storage, SizeGiB: 1, Serial: "proof-ha"})
	createVM(t, d, ref, &spec)
	t.Cleanup(func() { cleanupHA(t, d, ref) })

	// "stopped" keeps the CRM from ever starting a disposable VM.
	if err := d.SetHA(ctx, ref, HASpec{State: "stopped"}); err != nil {
		t.Fatalf("SetHA(stopped) on an unregistered vm: %v", err)
	}
	if registered, err := d.(*driver).hasHA(ctx, ref); err != nil || !registered {
		t.Fatalf("hasHA() after SetHA = %v, %v; want registered", registered, err)
	}
	if st := status(t, d, ref); !st.HA {
		t.Errorf("Status() after SetHA = %+v, want HA managed", st)
	}
	if err := d.SetHA(ctx, ref, HASpec{State: "stopped"}); err != nil {
		t.Fatalf("SetHA(stopped) on a registered vm: %v", err)
	}
	t.Log("SetHA registered then updated vm:" + strconv.Itoa(vmid) + " as stopped")

	if err := d.ClearHA(ctx, ref); err != nil {
		t.Fatalf("ClearHA(): %v", err)
	}
	switch err := d.ClearHA(ctx, ref); {
	case errors.Is(err, ErrNotFound):
		t.Logf("second ClearHA is ErrNotFound: %v", err)
	case err == nil:
		t.Log("second ClearHA is nil: the host deletes idempotently")
	default:
		t.Fatalf("second ClearHA() = %v, want ErrNotFound or nil", err)
	}
	if registered, err := d.(*driver).hasHA(ctx, ref); err != nil || registered {
		t.Fatalf("hasHA() after ClearHA = %v, %v; want unregistered", registered, err)
	}
	if st := status(t, d, ref); st.HA {
		t.Errorf("Status() after ClearHA = %+v, want HA unmanaged", st)
	}
	if err := finish(ctx, d)(d.DeleteVM(ctx, ref)); err != nil {
		t.Fatalf("DeleteVM(): %v", err)
	}
}

func proofSpec(env *proofEnv, vmid, cores, memoryMiB int, disks ...DiskSpec) VMSpec {
	return VMSpec{
		VMID:       vmid,
		Name:       fmt.Sprintf("okdctl-proof-%d", vmid),
		CPUType:    "host",
		Cores:      cores,
		MemoryMiB:  memoryMiB,
		Disks:      disks,
		EFIStorage: env.storage,
		MAC:        fmt.Sprintf("02:4F:4B:44:%02X:%02X", (vmid>>8)&0xff, vmid&0xff),
		Bridge:     env.bridge,
		Tag:        proofTag,
		Pool:       env.pool,
	}
}

func finish(ctx context.Context, d Driver) func(Task, error) error {
	return func(task Task, err error) error {
		if err != nil {
			return err
		}
		return d.WaitTask(ctx, task)
	}
}

// createVM arms the cleanup only once PVE has accepted the POST, so a refused
// create never deletes a VM the proof did not make.
func createVM(t *testing.T, d Driver, ref VMRef, spec *VMSpec) {
	t.Helper()
	ctx := t.Context()
	requireVMAbsent(t, d, ref)
	task, err := d.CreateVM(ctx, ref.Node, *spec)
	if err != nil {
		t.Fatalf("CreateVM(): %v", err)
	}
	if task.UPID == "" {
		t.Fatalf("CreateVM() returned no task: vm %d already carries tag %s", ref.VMID, proofTag)
	}
	t.Cleanup(func() { cleanupVM(t, d, ref, task) })
	if err := d.WaitTask(ctx, task); err != nil {
		t.Fatalf("CreateVM(): %v", err)
	}
}

// requireVMAbsent checks the per-VM status endpoint as well as the listing,
// which pvestatd publishes with a lag of several seconds.
func requireVMAbsent(t *testing.T, d Driver, ref VMRef) {
	t.Helper()
	vms, err := d.ListVMs(t.Context())
	if err != nil {
		t.Fatalf("ListVMs(): %v", err)
	}
	if i := slices.IndexFunc(vms, func(vm VMSummary) bool { return vm.VMID == ref.VMID }); i >= 0 {
		t.Fatalf("vmid %d is taken by %q on %s; refusing to touch it", ref.VMID, vms[i].Name, vms[i].Node)
	}
	if st, err := d.Status(t.Context(), ref); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Status(%d) = %+v, %v; want ErrNotFound before creating", ref.VMID, st, err)
	}
}

// leftovers reads the per-VM status and HA endpoints rather than the lagging
// listing, and names every proof VM, HA resource and ISO still on the host.
func leftovers(t *testing.T, d Driver, env *proofEnv) []string {
	t.Helper()
	ctx := t.Context()
	var left []string
	for vmid := env.vmid; vmid <= env.vmid+2; vmid++ {
		ref := VMRef{Node: env.node, VMID: vmid}
		if _, err := d.Status(ctx, ref); !errors.Is(err, ErrNotFound) {
			left = append(left, fmt.Sprintf("vm %d (%v)", vmid, err))
		}
		if registered, err := d.(*driver).hasHA(ctx, ref); err != nil || registered {
			left = append(left, fmt.Sprintf("ha resource %s (%v)", haSID(ref), err))
		}
	}
	volid := fmt.Sprintf("%s:iso/okdctl-proof-%d.iso", env.isoStorage, env.vmid)
	if v := lookupVolume(ctx, t, d, env, volid); v != nil {
		left = append(left, "volume "+volid)
	}
	return left
}

func status(t *testing.T, d Driver, ref VMRef) VMStatus {
	t.Helper()
	st, err := d.Status(t.Context(), ref)
	if err != nil {
		t.Fatalf("Status(): %v", err)
	}
	return st
}

func lookupVolume(ctx context.Context, t *testing.T, d Driver, env *proofEnv, volid string) *Volume {
	t.Helper()
	vols, err := d.ListISOs(ctx, env.node, env.isoStorage)
	if err != nil {
		t.Fatalf("ListISOs(): %v", err)
	}
	if i := slices.IndexFunc(vols, func(v Volume) bool { return v.VolID == volid }); i >= 0 {
		return &vols[i]
	}
	return nil
}

func sha256File(t *testing.T, path string) (sum string, size int64) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open iso: %v", err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		t.Fatalf("hash iso: %v", err)
	}
	return hex.EncodeToString(h.Sum(nil)), n
}

// cleanupVM runs after t.Context is cancelled, so it carries its own deadline;
// it awaits the create task first because PVE holds a create lock until then.
func cleanupVM(t *testing.T, d Driver, ref VMRef, created Task) {
	ctx, cancel := context.WithTimeout(context.Background(), proofTimeout)
	defer cancel()
	_ = d.WaitTask(ctx, created)
	st, err := d.Status(ctx, ref)
	if errors.Is(err, ErrNotFound) {
		return
	}
	if err != nil {
		t.Errorf("cleanup vm %d: status: %v (vm may be left on host)", ref.VMID, err)
		return
	}
	if st.Status != "stopped" {
		if err := finish(ctx, d)(d.Stop(ctx, ref)); err != nil {
			t.Errorf("cleanup vm %d: stop: %v", ref.VMID, err)
		}
	}
	if err := finish(ctx, d)(d.DeleteVM(ctx, ref)); err != nil {
		t.Errorf("cleanup vm %d: delete: %v (vm left on host)", ref.VMID, err)
		return
	}
	t.Logf("cleanup removed vm %d", ref.VMID)
}

// cleanupHA runs before cleanupVM (cleanups are LIFO) so the VM is never
// destroyed while registered.
func cleanupHA(t *testing.T, d Driver, ref VMRef) {
	ctx, cancel := context.WithTimeout(context.Background(), proofTimeout)
	defer cancel()
	if err := d.ClearHA(ctx, ref); err != nil && !errors.Is(err, ErrNotFound) {
		t.Errorf("cleanup ha %s: %v (resource may be left on host)", haSID(ref), err)
	}
}

// cleanupVolume runs after t.Context is cancelled, so it carries its own deadline.
func cleanupVolume(t *testing.T, d Driver, env *proofEnv, volid string) {
	ctx, cancel := context.WithTimeout(context.Background(), proofTimeout)
	defer cancel()
	vols, err := d.ListISOs(ctx, env.node, env.isoStorage)
	if err != nil {
		t.Errorf("cleanup %s: list isos: %v (volume may be left on host)", volid, err)
		return
	}
	if !slices.ContainsFunc(vols, func(v Volume) bool { return v.VolID == volid }) {
		return
	}
	if err := finish(ctx, d)(d.DeleteVolume(ctx, env.node, env.isoStorage, volid)); err != nil {
		t.Errorf("cleanup %s: delete: %v (volume left on host)", volid, err)
		return
	}
	t.Logf("cleanup removed %s", volid)
}
