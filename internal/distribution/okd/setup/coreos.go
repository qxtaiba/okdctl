package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/download"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/platform"
	"github.com/qxtaiba/okdctl/internal/system"
)

// DefaultProxmoxISODir is the default Proxmox-managed path where downloaded
// FCOS ISOs land when no explicit Proxmox storage reference is provided.
const DefaultProxmoxISODir = "/var/lib/vz/template/iso"

func (p *Phase) findOrDownloadFCOSISO(ctx context.Context, cfg *config.Config, opts *Options) (string, error) {
	isoDir := DefaultProxmoxISODir

	if cfg.Provider.Proxmox != nil && cfg.Provider.Proxmox.FCOSIso != "" {
		fcosISO := cfg.Provider.Proxmox.FCOSIso
		// Handle Proxmox storage format like "local:iso/filename.iso"
		switch {
		case strings.Contains(fcosISO, ":iso/"):
			parts := strings.SplitN(fcosISO, ":iso/", 2)
			if len(parts) == 2 {
				resolvedPath := filepath.Join(isoDir, parts[1])
				if system.FileExists(resolvedPath) {
					return resolvedPath, nil
				}
			}
		case strings.HasPrefix(fcosISO, "local:iso"):
			// Just storage reference without specific file - search for FCOS ISO
		default:
			if system.FileExists(fcosISO) {
				return fcosISO, nil
			}
		}
	}

	patterns := []string{
		"fedora-coreos-*.iso",
		"fcos-*.iso",
		"fedora-coreos.iso",
	}

	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(isoDir, pattern))
		if err != nil {
			continue
		}
		if len(matches) > 0 {
			slices.Sort(matches)
			isoPath := matches[len(matches)-1] // newest by lexicographic version
			p.Log.Info(fmt.Sprintf("coreos: found existing iso at %s", filepath.Base(isoPath)))
			return isoPath, nil
		}
	}

	workISODir := filepath.Join(opts.WorkDir, "downloads")
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(workISODir, pattern))
		if err != nil {
			continue
		}
		if len(matches) > 0 {
			slices.Sort(matches)
			isoPath := matches[len(matches)-1]
			p.Log.Info(fmt.Sprintf("coreos: found existing iso at %s", filepath.Base(isoPath)))
			return isoPath, nil
		}
	}

	p.Log.Info("coreos: no iso found, attempting auto-download")

	return p.EnsureCoreOSISO(ctx, cfg, &Options{
		BaseOptions: phase.BaseOptions{
			WorkDir: opts.WorkDir,
		},
		AutoDownloadISO: true,
	})
}

// DetectCoreOSVersion runs "openshift-install coreos print-stream-json" and
// extracts the ISO location, checksum, and release for the host architecture.
func (p *Phase) DetectCoreOSVersion(ctx context.Context) (*CoreOSInfo, error) {
	if !executor.CommandExists("openshift-install") {
		return nil, fmt.Errorf("openshift-install not found - run setup first")
	}

	result, err := p.Exec.RunChecked(ctx, "openshift-install", "coreos", "print-stream-json")
	if err != nil {
		return nil, fmt.Errorf("failed to get CoreOS stream info: %w", err)
	}

	var streamData struct {
		Architectures map[string]struct {
			Artifacts struct {
				Metal struct {
					Release string `json:"release"`
					Formats struct {
						ISO struct {
							Disk struct {
								Location string `json:"location"`
								SHA256   string `json:"sha256"`
							} `json:"disk"`
						} `json:"iso"`
					} `json:"formats"`
				} `json:"metal"`
			} `json:"artifacts"`
		} `json:"architectures"`
	}

	if err := json.Unmarshal([]byte(result.Stdout), &streamData); err != nil {
		return nil, fmt.Errorf("failed to parse CoreOS stream JSON: %w", err)
	}

	archKey := platform.CoreOSArch()
	arch, ok := streamData.Architectures[archKey]
	if !ok {
		return nil, fmt.Errorf("%s architecture not found in CoreOS stream", archKey)
	}

	metal := arch.Artifacts.Metal
	iso := metal.Formats.ISO.Disk

	if iso.Location == "" {
		return nil, fmt.Errorf("coreos iso location not found in stream")
	}

	return &CoreOSInfo{
		Version:      metal.Release,
		ISOUrl:       iso.Location,
		ISOChecksum:  iso.SHA256,
		Architecture: archKey,
	}, nil
}

// DownloadCoreOSISO downloads the CoreOS ISO described by info to destPath.
// An existing file with a matching checksum is reused; mismatch triggers a
// re-download.
func (p *Phase) DownloadCoreOSISO(ctx context.Context, info *CoreOSInfo, destPath string) error {
	if system.FileExists(destPath) {
		p.Log.Info(fmt.Sprintf("coreos: iso already exists at %s", destPath))
		if info.ISOChecksum != "" {
			err := download.ValidateChecksum(destPath, info.ISOChecksum)
			if err != nil {
				p.Log.Warn("coreos: existing iso checksum mismatch, re-downloading")
				// Continue to download
			} else {
				p.Log.Info("coreos: iso checksum verified successfully")
				return nil
			}
		} else {
			return nil
		}
	}

	p.Log.Info(fmt.Sprintf("coreos: downloading iso version %s", info.Version))
	p.Log.Info(fmt.Sprintf("coreos: url %s", info.ISOUrl))

	if err := system.EnsureDir(filepath.Dir(destPath)); err != nil {
		return err
	}

	opts := &download.Options{
		URL:              info.ISOUrl,
		OutputPath:       destPath,
		ExpectedChecksum: info.ISOChecksum,
		Description:      "CoreOS ISO",
		Logger:           p.Log,
	}

	if err := download.Download(ctx, opts); err != nil {
		return fmt.Errorf("failed to download CoreOS ISO: %w", err)
	}

	p.Log.Info(fmt.Sprintf("coreos: iso downloaded to %s", destPath))

	return nil
}

// EnsureCoreOSISO ensures the CoreOS ISO is available, downloading to the work
// directory (avoids permission issues with /var/lib/vz).
func (p *Phase) EnsureCoreOSISO(ctx context.Context, _ *config.Config, opts *Options) (string, error) {
	p.Log.Info("coreos: detecting version from openshift-install")

	info, err := p.DetectCoreOSVersion(ctx)
	if err != nil {
		return "", err
	}

	p.Log.Info(fmt.Sprintf("coreos: detected version %s", info.Version))

	// Separate from custom-isos directory which gets uploaded to Proxmox
	downloadsDir := filepath.Join(opts.WorkDir, "downloads")
	if err := system.EnsureDir(downloadsDir); err != nil {
		return "", fmt.Errorf("failed to create downloads directory: %w", err)
	}

	isoFilename := filepath.Base(info.ISOUrl)
	fcosISO := filepath.Join(downloadsDir, isoFilename)

	if system.FileExists(fcosISO) {
		p.Log.Info(fmt.Sprintf("coreos: iso already exists at %s", isoFilename))
		return fcosISO, nil
	}

	if err := p.DownloadCoreOSISO(ctx, info, fcosISO); err != nil {
		return "", err
	}

	return fcosISO, nil
}
