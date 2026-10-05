package provision

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/download"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox/hostssh"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/platform"
	"github.com/qxtaiba/okdctl/internal/system"
)

// logISOFound logs "coreos: iso found" for isoPath, deduped by base filename
// across the Provisioner's lifetime.
func (p *Provisioner) logISOFound(isoPath string) {
	base := filepath.Base(isoPath)
	if p.loggedISOs == nil {
		p.loggedISOs = make(map[string]bool)
	}
	if p.loggedISOs[base] {
		return
	}
	p.loggedISOs[base] = true
	p.Log.Info("coreos: iso found", "file", base)
}

// isoResolution describes how a configured FCOSIso spec was resolved.
type isoResolution int

const (
	isoEmpty    isoResolution = iota // unconfigured: caller globs
	isoResolved                      // configured + present: return path
	isoMissing                       // configured + absent: caller errors
)

// resolveConfiguredISO maps FCOSIso to isoEmpty/isoResolved/isoMissing:
// ":iso/<file>" resolves under hostssh.DefaultProxmoxISODir, bare "local:iso"
// (no filename) is isoEmpty for glob auto-detection, and isoMissing (rather
// than falling through to glob) surfaces a misconfigured pinned ISO instead
// of silently ignoring it.
func resolveConfiguredISO(spec string) (string, isoResolution) {
	if spec == "" {
		return "", isoEmpty
	}
	switch {
	case strings.Contains(spec, ":iso/"):
		_, filename, ok := strings.Cut(spec, ":iso/")
		if ok && filename != "" {
			resolved := filepath.Join(hostssh.DefaultProxmoxISODir, filename)
			if system.FileExists(resolved) {
				return resolved, isoResolved
			}
			return resolved, isoMissing
		}
		return "", isoEmpty
	case strings.HasPrefix(spec, "local:iso"):
		return "", isoEmpty
	default:
		if system.FileExists(spec) {
			return spec, isoResolved
		}
		return spec, isoMissing
	}
}

func (p *Provisioner) findOrDownloadFCOSISO(ctx context.Context, cfg *config.Config, opts Options) (string, error) {
	isoDir := hostssh.DefaultProxmoxISODir

	if cfg.Provider.Proxmox != nil {
		path, res := resolveConfiguredISO(cfg.Provider.Proxmox.FCOSIso)
		switch res {
		case isoResolved:
			return path, nil
		case isoMissing:
			return "", &errtypes.ConfigError{
				Msg: fmt.Sprintf("configured coreos iso not found: %s", cfg.Provider.Proxmox.FCOSIso),
			}
		}
	}

	// CoreOSISONamePatterns covers official shapes (fedora-coreos-*.iso,
	// scos-*.iso); fcos-*.iso/fedora-coreos.iso are local-only conventions
	// hostssh's remote guard never needs to recognize.
	patterns := slices.Concat(nodetypes.CoreOSISONamePatterns, []string{
		"fcos-*.iso",
		"fedora-coreos.iso",
	})

	if isoPath, ok := p.findNewestISO(isoDir, patterns); ok {
		return isoPath, nil
	}

	workISODir := filepath.Join(opts.WorkDir, "downloads")
	if isoPath, ok := p.findNewestISO(workISODir, patterns); ok {
		return isoPath, nil
	}

	p.Log.Info("coreos: no iso found, attempting auto-download")

	return p.EnsureCoreOSISO(ctx, Options{WorkDir: opts.WorkDir})
}

// findNewestISO globs dir against each pattern in order, returning the
// lexicographically newest match from the first pattern with a hit.
func (p *Provisioner) findNewestISO(dir string, patterns []string) (string, bool) {
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			continue
		}
		if len(matches) > 0 {
			isoPath := slices.Max(matches) // newest by lexicographic version
			p.logISOFound(isoPath)
			return isoPath, true
		}
	}
	return "", false
}

// installerBin supplies the stream metadata: setup's download-tools step puts
// the release's own openshift-install on PATH before any ISO is resolved.
const installerBin = "openshift-install"

// coreOSStreamData is the subset of the CoreOS stream document
// DetectCoreOSVersion consumes.
type coreOSStreamData struct {
	Stream        string `json:"stream"`
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

func coreOSInfoFromStream(sd *coreOSStreamData) (*CoreOSInfo, error) {
	archKey := platform.ClusterCoreOSArch
	arch, ok := sd.Architectures[archKey]
	if !ok {
		return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("%s architecture not found in CoreOS stream", archKey)}
	}
	metal := arch.Artifacts.Metal
	iso := metal.Formats.ISO.Disk
	if iso.Location == "" {
		return nil, &errtypes.ConfigError{Msg: "coreos iso location not found in stream"}
	}
	// An empty checksum would make download.Fetch skip verification.
	if iso.SHA256 == "" {
		return nil, &errtypes.ConfigError{Msg: "coreos iso sha256 not found in stream"}
	}
	return &CoreOSInfo{
		Version:     metal.Release,
		ISOUrl:      iso.Location,
		ISOChecksum: iso.SHA256,
	}, nil
}

// installerStream returns the stream document openshift-install carries for its own release.
func (p *Provisioner) installerStream(ctx context.Context) (*coreOSStreamData, error) {
	result, err := p.Exec.RunOutputChecked(ctx, 0, installerBin, "coreos", "print-stream-json")
	if err != nil {
		return nil, err
	}
	if result.Truncated {
		return nil, fmt.Errorf("stream document truncated after %d bytes", len(result.Stdout))
	}
	var sd coreOSStreamData
	if err := json.Unmarshal([]byte(result.Stdout), &sd); err != nil {
		return nil, fmt.Errorf("parse coreos stream: %w", err)
	}
	return &sd, nil
}

// DetectCoreOSVersion returns the CoreOS ISO location, checksum, and release
// the openshift-install on PATH publishes for its own OKD release. An
// installer that cannot print its stream fails as a ConfigError.
func (p *Provisioner) DetectCoreOSVersion(ctx context.Context) (*CoreOSInfo, error) {
	sd, err := p.installerStream(ctx)
	if err != nil {
		return nil, (&errtypes.ConfigError{Msg: "openshift-install could not print the coreos stream for this release", Err: err}).
			WithHint("run 'openshift-install coreos print-stream-json' to see why")
	}
	info, err := coreOSInfoFromStream(sd)
	if err != nil {
		return nil, err
	}
	p.Log.Info("coreos: resolved iso from installer stream", "stream", sd.Stream, "version", info.Version)
	return info, nil
}

// DownloadCoreOSISO downloads the CoreOS ISO described by info to destPath,
// reusing an existing file with a matching checksum or re-downloading on mismatch.
func (p *Provisioner) DownloadCoreOSISO(ctx context.Context, info *CoreOSInfo, destPath string) error {
	if system.FileExists(destPath) {
		p.logISOFound(destPath)
		if info.ISOChecksum != "" {
			err := download.ValidateChecksum(ctx, destPath, info.ISOChecksum)
			if err != nil {
				p.Log.Warn("coreos: existing iso checksum mismatch, re-downloading", "path", destPath, "err", err)
			} else {
				p.Log.Info("coreos: iso checksum verified")
				return nil
			}
		} else {
			return nil
		}
	}

	p.Log.Info("coreos: downloading iso", "version", info.Version, "url", info.ISOUrl)

	if err := system.EnsureDir(filepath.Dir(destPath)); err != nil {
		return &errtypes.ConfigError{Msg: "ensure CoreOS ISO destination directory", Err: err}
	}

	if err := download.Fetch(
		ctx, info.ISOUrl, destPath,
		download.WithFetchChecksum(info.ISOChecksum),
		download.WithDescription("CoreOS ISO"),
		download.WithLogger(p.Log),
		download.WithProgress(logutil.ProgressBarsEnabled()),
	); err != nil {
		return &errtypes.NetworkError{Msg: "download CoreOS ISO", Err: err}
	}

	p.Log.Info("coreos: iso downloaded", "path", destPath)

	return nil
}

// EnsureCoreOSISO ensures the CoreOS ISO is available, downloading to the
// work directory (avoiding /var/lib/vz permission issues) when absent. An
// ISO already at the download path is reused on filename existence alone —
// unlike DownloadCoreOSISO, no checksum is re-verified, so a corrupt cache
// must be deleted manually.
func (p *Provisioner) EnsureCoreOSISO(ctx context.Context, opts Options) (string, error) {
	info, err := p.DetectCoreOSVersion(ctx)
	if err != nil {
		return "", err
	}

	// Separate from custom-isos directory which gets uploaded to Proxmox
	downloadsDir := filepath.Join(opts.WorkDir, "downloads")
	if err := system.EnsureDir(downloadsDir); err != nil {
		return "", &errtypes.ConfigError{Msg: "create downloads directory", Err: err}
	}

	isoFilename := filepath.Base(info.ISOUrl)
	fcosISO := filepath.Join(downloadsDir, isoFilename)

	if system.FileExists(fcosISO) {
		p.logISOFound(fcosISO)
		return fcosISO, nil
	}

	if err := p.DownloadCoreOSISO(ctx, info, fcosISO); err != nil {
		return "", err
	}

	return fcosISO, nil
}
