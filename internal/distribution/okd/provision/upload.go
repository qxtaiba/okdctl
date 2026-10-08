package provision

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/download"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

type localISO struct {
	path   string
	name   string
	sha256 string
	size   int64
}

func collectISOFiles(isoDir string) ([]string, error) {
	entries, err := os.ReadDir(isoDir)
	if err != nil {
		return nil, err
	}

	var isoFiles []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".iso" {
			continue
		}
		isoFiles = append(isoFiles, filepath.Join(isoDir, entry.Name()))
	}
	return isoFiles, nil
}

func inspectISOs(ctx context.Context, files []string) ([]localISO, error) {
	isos := make([]localISO, 0, len(files))
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", f, err)
		}
		sum, err := download.CalculateChecksum(ctx, f)
		if err != nil {
			return nil, err
		}
		isos = append(isos, localISO{path: f, name: filepath.Base(f), sha256: sum, size: info.Size()})
	}
	return isos, nil
}

// isoUploadRecords maps volume ids to the sha256 Proxmox verified on their
// last upload, the only checksum source since the storage API reports sizes
// but no hashes.
type isoUploadRecords map[string]string

// loadUploadRecords treats a missing or unreadable record as empty, which
// only ever costs a re-upload.
func loadUploadRecords(path string) isoUploadRecords {
	records := isoUploadRecords{}
	data, err := os.ReadFile(path)
	if err != nil {
		return records
	}
	if json.Unmarshal(data, &records) != nil {
		return isoUploadRecords{}
	}
	return records
}

func (r isoUploadRecords) save(path string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return system.AtomicWrite(path, data, 0o600)
}

// uploadNeeded reports false only when the remote volume has the local size
// and the recorded verified upload matches the local sha256; coreos-installer
// embeds ignition in a fixed-size area, so size alone never proves content.
func uploadNeeded(iso localISO, remote map[string]uint64, recorded string) bool {
	size, ok := remote[iso.name]
	if !ok || iso.size < 0 || size != uint64(iso.size) {
		return true
	}
	return recorded != iso.sha256
}

func (p *Provisioner) isoStore(cfg *config.Config) (*proxmox.ISOStore, error) {
	if p.ProxmoxCreds == nil || !p.ProxmoxCreds.IsValid() {
		return nil, &errtypes.ConfigError{Msg: "proxmox api credentials are required for iso storage access"}
	}
	return proxmox.NewISOStore(p.ProxmoxCreds, cfg.Provider.Proxmox.ISOStorage), nil
}

func totalSizeMB(isos []localISO) float64 {
	var total int64
	for _, iso := range isos {
		total += iso.size
	}
	return math.Round(float64(total)/1024/1024*10) / 10
}

// isoUploadPlan is the comparison of the local ISOs against node's storage.
type isoUploadPlan struct {
	store      *proxmox.ISOStore
	node       string
	recordPath string
	records    isoUploadRecords
	isos       []localISO
	pending    []localISO
}

func (p *Provisioner) planISOUpload(ctx context.Context, cfg *config.Config, opts Options, files []string) (*isoUploadPlan, error) {
	store, err := p.isoStore(cfg)
	if err != nil {
		return nil, err
	}
	isos, err := inspectISOs(ctx, files)
	if err != nil {
		return nil, err
	}
	node := cfg.Provider.Proxmox.Node
	remote, err := store.Volumes(ctx, node)
	if err != nil {
		return nil, &errtypes.NetworkError{Msg: "list proxmox iso storage", Err: err}
	}
	plan := &isoUploadPlan{
		store:      store,
		node:       node,
		recordPath: workspace.ISOUploadRecordPath(opts.ProjectRoot),
		isos:       isos,
	}
	plan.records = loadUploadRecords(plan.recordPath)
	for _, iso := range isos {
		if uploadNeeded(iso, remote, plan.records[store.VolumeID(iso.name)]) {
			plan.pending = append(plan.pending, iso)
		}
	}
	return plan, nil
}

// upload drops an ISO's record before sending it, so an interrupted upload
// can never leave a stale record vouching for a half-replaced volume.
func (plan *isoUploadPlan) upload(ctx context.Context, iso localISO) error {
	volume := plan.store.VolumeID(iso.name)
	if _, ok := plan.records[volume]; ok {
		delete(plan.records, volume)
		if err := plan.records.save(plan.recordPath); err != nil {
			return fmt.Errorf("save iso upload record: %w", err)
		}
	}
	if err := plan.store.Upload(ctx, plan.node, iso.path, iso.sha256); err != nil {
		return &errtypes.NetworkError{Msg: "upload iso to proxmox", Err: err}
	}
	plan.records[volume] = iso.sha256
	return plan.records.save(plan.recordPath)
}

// UploadCustomISOsToProxmox uploads the custom ISOs that are missing or
// differ on the ISO storage through the Proxmox API, one file at a time, so
// an interrupted run resumes with only the remainder.
func (p *Provisioner) UploadCustomISOsToProxmox(ctx context.Context, cfg *config.Config, opts Options) error {
	if cfg.Provider.Proxmox == nil {
		return &errtypes.ConfigError{Msg: msgProxmoxProviderRequired}
	}

	isoDir := filepath.Join(opts.WorkDir, "custom-isos")
	if !system.DirExists(isoDir) {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("custom ISOs directory not found: %s", isoDir)}
	}

	isoFiles, err := collectISOFiles(isoDir)
	if err != nil {
		return &errtypes.ConfigError{Msg: "collect ISO files", Err: err}
	}
	if len(isoFiles) == 0 {
		p.Log.Warn("iso: no iso files found to upload")
		return nil
	}

	if err := p.ValidateISOPlacement(ctx, cfg); err != nil {
		return err
	}
	plan, err := p.planISOUpload(ctx, cfg, opts, isoFiles)
	if err != nil {
		return err
	}
	for _, iso := range plan.isos {
		if !slices.Contains(plan.pending, iso) {
			p.Log.Info("iso: skipping unchanged", "file", iso.name)
		}
	}
	if len(plan.pending) == 0 {
		p.Log.Info("iso: all isos already up to date on proxmox storage")
		return p.verifySharedISOs(ctx, cfg, isoFiles)
	}

	p.Log.Info("iso: uploading", "count", len(plan.pending), "size_mb", totalSizeMB(plan.pending), "storage", cfg.Provider.Proxmox.ISOStorage, "node", plan.node)
	for _, iso := range plan.pending {
		if err := plan.upload(ctx, iso); err != nil {
			return err
		}
		p.Log.Info("iso: uploaded", "file", iso.name)
	}

	p.Log.Info("iso: uploaded files to proxmox storage", "count", len(plan.pending))
	return p.verifySharedISOs(ctx, cfg, isoFiles)
}

// currentISOUploads returns the local ISO files when every one is already
// on the ISO storage with its verified checksum; any failure reports false.
func (p *Provisioner) currentISOUploads(ctx context.Context, cfg *config.Config, opts Options) ([]string, bool) {
	isoDir := filepath.Join(opts.WorkDir, "custom-isos")
	if !system.DirExists(isoDir) {
		return nil, false
	}
	isoFiles, err := collectISOFiles(isoDir)
	if err != nil || len(isoFiles) == 0 {
		return nil, false
	}
	plan, err := p.planISOUpload(ctx, cfg, opts, isoFiles)
	return isoFiles, err == nil && len(plan.pending) == 0
}

// ISOUploadAlreadyDone returns true when every local ISO is on the ISO
// storage with its verified checksum. Missing credentials, an unreadable
// storage, or absent Proxmox config return (false, nil), so Exec runs and
// surfaces the real failure.
func (p *Provisioner) ISOUploadAlreadyDone(ctx context.Context, cfg *config.Config, opts Options) (bool, error) {
	if cfg.Provider.Proxmox == nil {
		return false, nil
	}
	isoFiles, current := p.currentISOUploads(ctx, cfg, opts)
	if !current {
		return false, nil
	}
	if err := p.ValidateISOPlacement(ctx, cfg); err != nil {
		return false, err
	}
	if err := p.verifySharedISOs(ctx, cfg, isoFiles); err != nil {
		return false, err
	}
	return true, nil
}
