package provision

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/download"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/platform"
	"github.com/qxtaiba/okdctl/internal/system"
)

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
		p.Log.Info("coreos: iso found", "file", filepath.Base(destPath))
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

// EnsureCoreOSISO returns the work-directory path of the CoreOS ISO named by
// the installer's stream, downloading it unless a copy with the stream's
// checksum is already there.
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

	isoPath := filepath.Join(downloadsDir, filepath.Base(info.ISOUrl))
	if err := p.DownloadCoreOSISO(ctx, info, isoPath); err != nil {
		return "", err
	}
	return isoPath, nil
}
