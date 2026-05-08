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
	"github.com/qxtaiba/okdctl/internal/httputil"
	"github.com/qxtaiba/okdctl/internal/platform"
	"github.com/qxtaiba/okdctl/internal/system"
)

// logISOFound emits "coreos: iso found" at Info for isoPath, de-duping by
// base filename across this Phase's lifetime so a single setup run never
// logs the same ISO more than once.
func (p *Phase) logISOFound(isoPath string) {
	base := filepath.Base(isoPath)
	if p.loggedISOs == nil {
		p.loggedISOs = make(map[string]bool)
	}
	if p.loggedISOs[base] {
		return
	}
	p.loggedISOs[base] = true
	p.Log.Info("coreos: iso found", "iso", base)
}

// isoResolution describes how a configured FCOSIso spec was resolved.
type isoResolution int

const (
	isoEmpty    isoResolution = iota // unconfigured: caller globs
	isoResolved                      // configured + present: return path
	isoMissing                       // configured + absent: caller errors
)

// resolveConfiguredISO maps cfg.Provider.Proxmox.FCOSIso to one of three
// states. ":iso/<file>" is resolved relative to phase.DefaultProxmoxISODir;
// a bare "local:iso" pool reference (no filename) is treated as isoEmpty
// so glob auto-detection still applies; bare paths are checked via
// system.FileExists. Returning isoMissing prevents the previous silent
// fallthrough to the glob loop on a misconfigured operator-pinned ISO.
func resolveConfiguredISO(spec string) (string, isoResolution) {
	if spec == "" {
		return "", isoEmpty
	}
	switch {
	case strings.Contains(spec, ":iso/"):
		parts := strings.SplitN(spec, ":iso/", 2)
		if len(parts) == 2 && parts[1] != "" {
			resolved := filepath.Join(phase.DefaultProxmoxISODir, parts[1])
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

func (p *Phase) findOrDownloadFCOSISO(ctx context.Context, cfg *config.Config, opts *Options) (string, error) {
	isoDir := phase.DefaultProxmoxISODir

	if cfg.Provider.Proxmox != nil {
		path, res := resolveConfiguredISO(cfg.Provider.Proxmox.FCOSIso)
		switch res {
		case isoResolved:
			return path, nil
		case isoMissing:
			return "", &errtypes.ConfigError{
				Msg: fmt.Sprintf("configured FCOS ISO not found: %s", cfg.Provider.Proxmox.FCOSIso),
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
			isoPath := slices.Max(matches) // newest by lexicographic version
			p.logISOFound(isoPath)
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
			isoPath := slices.Max(matches) // newest by lexicographic version
			p.logISOFound(isoPath)
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

// minSCOSStreamMinor is the first OKD minor that publishes scos.json
// (Stream CoreOS); earlier minors (4.15-4.18) ship Fedora CoreOS via
// fcos.json. The schema is identical at the path the parser walks.
// Verified 2026-04-20 across release-4.14 through release-4.24.
const minSCOSStreamMinor = 19

// streamRawBaseURL is the GitHub raw-content root for openshift/installer.
// Tests override this to an httptest.Server URL for hermetic mocking.
var streamRawBaseURL = "https://raw.githubusercontent.com"

// coreOSStreamData is the subset of fcos.json / scos.json DetectCoreOSVersion
// consumes. Both files share the schema at this path; the parser does not
// read the top-level stream field, so c9s/c10s and stable all work.
type coreOSStreamData struct {
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
// "4.19.0-0.okd-2025-…". The bool is true only when both major and minor
// scanned successfully; callers must refuse the request when it is false.
func parseOKDMinor(version string) (int, bool) {
	var major, minor int
	n, _ := fmt.Sscanf(version, "%d.%d", &major, &minor)
	return minor, n == 2
}

// streamFileForMinor returns the data file an OKD minor publishes:
// fcos.json for 4.15-4.18, scos.json for 4.19+.
func streamFileForMinor(minor int) string {
	if minor >= minSCOSStreamMinor {
		return "scos.json"
	}
	return "fcos.json"
}

// fetchCoreOSStream fetches and parses the CoreOS stream JSON at url.
// Trust anchor: the request is made over HTTPS to raw.githubusercontent.com;
// GitHub's TLS certificate is the sole guarantee of document authenticity.
// The JSON carries no cryptographic signature and is not pinned to a commit
// SHA. The ISO artifact URL and sha256 field within the returned data are
// validated at download time by DownloadCoreOSISO, so the ISO binary itself
// is integrity-checked — only the stream document that supplies those values
// is unverified beyond TLS.
func fetchCoreOSStream(ctx context.Context, url string) (*coreOSStreamData, error) {
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
		return nil, fmt.Errorf("coreos stream: %w", &download.HTTPStatusError{
			Status: resp.StatusCode,
			Method: http.MethodGet,
			URL:    url,
		})
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read coreos stream: %w", err)
	}
	var sd coreOSStreamData
	if err := json.Unmarshal(body, &sd); err != nil {
		return nil, fmt.Errorf("parse coreos stream: %w", err)
	}
	return &sd, nil
}

func coreOSInfoFromStream(sd *coreOSStreamData) (*CoreOSInfo, error) {
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
// for the host architecture. okdVersion picks the right upstream data file:
// 4.15-4.18 → fcos.json (Fedora CoreOS), 4.19+ → scos.json (Stream CoreOS).
// A malformed okdVersion fails fast as a typed ConfigError; callers used to
// see a 404 from release-4.0/... when minor silently fell back to 0.
func (p *Phase) DetectCoreOSVersion(ctx context.Context, okdVersion string) (*CoreOSInfo, error) {
	minor, ok := parseOKDMinor(okdVersion)
	if !ok {
		return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("invalid OKD version %q: cannot parse major.minor", okdVersion)}
	}
	streamURL := fmt.Sprintf(
		"%s/openshift/installer/release-4.%d/data/data/coreos/%s",
		streamRawBaseURL, minor, streamFileForMinor(minor),
	)
	sd, err := fetchCoreOSStream(ctx, streamURL)
	if err != nil {
		return nil, &errtypes.ClusterError{Msg: "failed to fetch CoreOS stream info", Err: err}
	}
	return coreOSInfoFromStream(sd)
}

// DownloadCoreOSISO downloads the CoreOS ISO described by info to destPath.
// An existing file with a matching checksum is reused; mismatch triggers a
// re-download.
func (p *Phase) DownloadCoreOSISO(ctx context.Context, info *CoreOSInfo, destPath string) error {
	if system.FileExists(destPath) {
		p.logISOFound(destPath)
		if info.ISOChecksum != "" {
			err := download.ValidateChecksum(destPath, info.ISOChecksum)
			if err != nil {
				p.Log.Warn("coreos: existing iso checksum mismatch, re-downloading")
			} else {
				p.Log.Info("coreos: iso checksum verified successfully")
				return nil
			}
		} else {
			return nil
		}
	}

	p.Log.Info("coreos: downloading iso", "version", info.Version)
	p.Log.Info("coreos: iso url", "url", info.ISOUrl)

	if err := system.EnsureDir(filepath.Dir(destPath)); err != nil {
		return &errtypes.ConfigError{Msg: "failed to ensure CoreOS ISO destination directory", Err: err}
	}

	if err := download.Fetch(ctx, info.ISOUrl, destPath,
		download.WithChecksum(info.ISOChecksum),
		download.WithDescription("CoreOS ISO"),
		download.WithLogger(p.Log),
	); err != nil {
		return &errtypes.NetworkError{Msg: "failed to download CoreOS ISO", Err: err}
	}

	p.Log.Info("coreos: iso downloaded", "path", destPath)

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

	p.Log.Info("coreos: detected version", "version", info.Version)

	// Separate from custom-isos directory which gets uploaded to Proxmox
	downloadsDir := filepath.Join(opts.WorkDir, "downloads")
	if err := system.EnsureDir(downloadsDir); err != nil {
		return "", &errtypes.ConfigError{Msg: "failed to create downloads directory", Err: err}
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
