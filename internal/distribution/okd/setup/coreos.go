package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/download"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/httputil"
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

// minScosDirectFetch is the first OKD minor that publishes scos.json at
// openshift/installer release-4.<minor>/data/data/coreos/scos.json. Verified
// 2026-04-20: minors 4.15–4.18 return 404; 4.19+ return 200.
const minScosDirectFetch = 19

// scosRawBaseURL is the GitHub raw-content root for openshift/installer.
// Tests override this to an httptest.Server URL for hermetic mocking.
var scosRawBaseURL = "https://raw.githubusercontent.com"

// scosStreamData is the CoreOS stream JSON subset DetectCoreOSVersion
// consumes. The schema matches both openshift-install stdout and the
// scos.json files on openshift/installer release branches.
type scosStreamData struct {
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

// parseOKDMinor extracts the minor from an OKD version like
// "4.19.0-0.okd-2025-…". Returns 0 on parse failure (0 < minScosDirectFetch
// so the shellout path is taken safely for malformed or empty input).
func parseOKDMinor(version string) int {
	var major, minor int
	_, _ = fmt.Sscanf(version, "%d.%d", &major, &minor)
	return minor
}

func fetchSCOSStream(ctx context.Context, url string) (*scosStreamData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := httputil.New(httputil.TimeoutMedium).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scos.json: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read scos.json: %w", err)
	}
	var sd scosStreamData
	if err := json.Unmarshal(body, &sd); err != nil {
		return nil, fmt.Errorf("parse scos.json: %w", err)
	}
	return &sd, nil
}

func coreOSInfoFromStream(sd *scosStreamData) (*CoreOSInfo, error) {
	archKey := platform.CoreOSArch()
	arch, ok := sd.Architectures[archKey]
	if !ok {
		return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("%s architecture not found in CoreOS stream", archKey)}
	}
	metal := arch.Artifacts.Metal
	iso := metal.Formats.ISO.Disk
	if iso.Location == "" {
		return nil, &errtypes.ConfigError{Msg: "coreos iso location not found in stream"}
	}
	return &CoreOSInfo{
		Version:      metal.Release,
		ISOUrl:       iso.Location,
		ISOChecksum:  iso.SHA256,
		Architecture: archKey,
	}, nil
}

// DetectCoreOSVersion returns the CoreOS ISO location, checksum, and release
// for the host architecture. For OKD 4.19+ it fetches scos.json directly from
// the openshift/installer release branch; on fetch failure or for older
// minors it falls back to "openshift-install coreos print-stream-json".
func (p *Phase) DetectCoreOSVersion(ctx context.Context, okdVersion string) (*CoreOSInfo, error) {
	if minor := parseOKDMinor(okdVersion); minor >= minScosDirectFetch {
		url := fmt.Sprintf(
			"%s/openshift/installer/release-4.%d/data/data/coreos/scos.json",
			scosRawBaseURL, minor,
		)
		sd, fetchErr := fetchSCOSStream(ctx, url)
		if fetchErr == nil {
			return coreOSInfoFromStream(sd)
		}
		p.Log.Warn("coreos: direct scos.json fetch failed, falling back to shellout", "err", fetchErr)
	}

	if !executor.CommandExists("openshift-install") {
		return nil, &errtypes.ConfigError{Msg: "openshift-install not found - run setup first"}
	}
	result, err := p.Exec.RunChecked(ctx, "openshift-install", "coreos", "print-stream-json")
	if err != nil {
		return nil, &errtypes.ClusterError{Msg: "failed to get CoreOS stream info", Err: err}
	}
	var sd scosStreamData
	if err := json.Unmarshal([]byte(result.Stdout), &sd); err != nil {
		return nil, &errtypes.ConfigError{Msg: "failed to parse CoreOS stream JSON", Err: err}
	}
	return coreOSInfoFromStream(&sd)
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
		return &errtypes.ConfigError{Msg: "failed to ensure CoreOS ISO destination directory", Err: err}
	}

	opts := &download.Options{
		URL:              info.ISOUrl,
		OutputPath:       destPath,
		ExpectedChecksum: info.ISOChecksum,
		Description:      "CoreOS ISO",
		Logger:           p.Log,
	}

	if err := download.Download(ctx, opts); err != nil {
		return &errtypes.NetworkError{Msg: "failed to download CoreOS ISO", Err: err}
	}

	p.Log.Info(fmt.Sprintf("coreos: iso downloaded to %s", destPath))

	return nil
}

// EnsureCoreOSISO ensures the CoreOS ISO is available, downloading to the work
// directory (avoids permission issues with /var/lib/vz).
func (p *Phase) EnsureCoreOSISO(ctx context.Context, cfg *config.Config, opts *Options) (string, error) {
	p.Log.Info("coreos: detecting version from openshift-install")

	var okdVersion string
	if cfg != nil {
		okdVersion = cfg.Distribution.Version
	}
	info, err := p.DetectCoreOSVersion(ctx, okdVersion)
	if err != nil {
		return "", err
	}

	p.Log.Info(fmt.Sprintf("coreos: detected version %s", info.Version))

	// Separate from custom-isos directory which gets uploaded to Proxmox
	downloadsDir := filepath.Join(opts.WorkDir, "downloads")
	if err := system.EnsureDir(downloadsDir); err != nil {
		return "", &errtypes.ConfigError{Msg: "failed to create downloads directory", Err: err}
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
