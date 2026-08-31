package provision

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/download"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/httputil"
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

	return p.EnsureCoreOSISO(ctx, cfg, Options{WorkDir: opts.WorkDir})
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

// minSCOSStreamMajor/Minor mark the first OKD release publishing scos.json
// (Stream CoreOS); 4.15-4.18 ship fcos.json, every 5.x+ ships scos.json.
// Verified 2026-04-20 across release-4.14 through release-4.24.
const (
	minSCOSStreamMajor = 4
	minSCOSStreamMinor = 19
)

// streamRawBaseURL is the GitHub raw-content root for openshift/installer;
// tests override it to an httptest.Server URL.
var streamRawBaseURL = "https://raw.githubusercontent.com"

// coreOSStreamPin anchors one OKD release's stream JSON: CommitSHA pins the
// immutable installer tree, JSONSHA256 verifies the fetched body before
// parsing. Both update together on version bumps — see README "Bumping the
// CoreOS stream pin".
type coreOSStreamPin struct {
	CommitSHA  string
	JSONSHA256 string
}

// okdVersionKey identifies a release train by (major, minor) — keying on
// minor alone would let a future 5.x collide with a same-numbered 4.x pin
// and resolve to the wrong commit/SHA-256.
type okdVersionKey struct {
	Major int
	Minor int
}

// streamPins maps each supported (major, minor) to a pinned
// openshift/installer commit SHA and expected SHA-256, never fetched from
// the mutable release-X.Y branch — an attacker who rewrites that JSON could
// rewrite its sha256 too, defeating DownloadCoreOSISO's integrity check.
//
// 4.15-4.18 share one fcos.json sha256 and 4.21-4.23 share one scos.json
// sha256 by design (byte-identical upstream files); goconst is suppressed
// since scripts/update-coreos-pins.sh maintains this mechanically.
//
// To update: sha256sum the pinned commit's <fcos|scos>.json and set
// CommitSHA/JSONSHA256 below, or see scripts/update-coreos-pins.sh. Tests
// may override this var with hermetic pin entries.
//
//nolint:goconst,nolintlint // see comment above re: 4.15-4.18 / 4.21-4.23 sha-equal-by-design
var streamPins = map[okdVersionKey]coreOSStreamPin{
	{4, 10}: {CommitSHA: "62137b29c72f4303faeb325dce01bc358d68d2ad", JSONSHA256: "d30f4bfbda28a7d455ea4ce7c09ba1f4e5dab168b78ef870458814e30810a8de"},
	{4, 11}: {CommitSHA: "64675f82cb5be511953ef6eff2a9d76efa9cfe73", JSONSHA256: "b96821026c8db5df1a5be02e7fa5cb5c549609e72f0281c5f9fc714fbe7bbe79"},
	{4, 12}: {CommitSHA: "b86064a94ccd47a4547bd16771de98dc36a4abb0", JSONSHA256: "38c6cf1e5c058a31c4ffec6a9045aca1fbbf6d49f5c100ed39cbb46ac0ea3b0f"},
	{4, 13}: {CommitSHA: "b3d2f7b8834666c220b88f7aee46ec9160274bcc", JSONSHA256: "33c89ea1b543cc0e9eaa32656827bc57f23705bc153e4c3454e82914152f770d"},
	{4, 14}: {CommitSHA: "4dd5abdf12a97ef0f32f6774ab79fa8dc6482f34", JSONSHA256: "78705a4faee6530ccc5f4564f661cc25e0dca6bde2fb70b502e38c36e7d48a15"},
	{4, 15}: {CommitSHA: "83c823bf5cb70c42dcbbc93306a570759ac6aaf8", JSONSHA256: "35147552e46269b5baabcdef1b27b38a4afec8c326d60b718890c5846595e9c1"},
	{4, 16}: {CommitSHA: "441e0e5469d5698ce147c092c7c802d7c44b1557", JSONSHA256: "35147552e46269b5baabcdef1b27b38a4afec8c326d60b718890c5846595e9c1"},
	{4, 17}: {CommitSHA: "b102c3acc6afdc1aed628f8d5604a467fba9b8c4", JSONSHA256: "35147552e46269b5baabcdef1b27b38a4afec8c326d60b718890c5846595e9c1"},
	{4, 18}: {CommitSHA: "01812b66eb92aefc97907166e741d41961116eea", JSONSHA256: "35147552e46269b5baabcdef1b27b38a4afec8c326d60b718890c5846595e9c1"},
	{4, 19}: {CommitSHA: "3344466dfa462b4512246923af3932d885875ea0", JSONSHA256: "35816bdaff8aeb9816b08a21b65a4a837dc408d8f43099891c1e787a0ae6667c"},
	{4, 20}: {CommitSHA: "f9e3db0194c9acc504b9d046b3867fb53005c9c0", JSONSHA256: "b65c52ddcf8136e810a9c80e77055efdeaaa4358f7f8f321d7f0d7bc74def82a"},
	{4, 21}: {CommitSHA: "295daec8d3fac4d15c5b616613e50320cf209b05", JSONSHA256: "3bfc32f58e48880e3fb6ef56b19f8ba41411ba35416fef2d881d5adaf474600c"},
	{4, 22}: {CommitSHA: "32af8e768b33608072996c4a80e041d5d549b5fe", JSONSHA256: "3bfc32f58e48880e3fb6ef56b19f8ba41411ba35416fef2d881d5adaf474600c"},
	{4, 23}: {CommitSHA: "04b457821289440760611be90daf4cf33bbd29f8", JSONSHA256: "3bfc32f58e48880e3fb6ef56b19f8ba41411ba35416fef2d881d5adaf474600c"},
}

// coreOSStreamData is the fcos.json/scos.json subset DetectCoreOSVersion
// consumes; it never reads the top-level stream field, so c9s/c10s and
// stable all parse the same.
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

// parseOKDVersion extracts major.minor from a version like
// "4.19.0-0.okd-2025-…"; ok is false unless both scanned.
func parseOKDVersion(version string) (major, minor int, ok bool) {
	n, _ := fmt.Sscanf(version, "%d.%d", &major, &minor)
	return major, minor, n == 2
}

// streamFileForVersion returns the data file an OKD release publishes:
// fcos.json for 4.15-4.18, scos.json for 4.19+ and every 5.x+.
func streamFileForVersion(major, minor int) string {
	if major > minSCOSStreamMajor || (major == minSCOSStreamMajor && minor >= minSCOSStreamMinor) {
		return "scos.json"
	}
	return "fcos.json"
}

// fetchCoreOSStream fetches and parses the CoreOS stream JSON at url,
// verifying against expectedSHA256 before parsing when non-empty (a
// mismatch is a hard error); production callers pass the streamPins
// constant, tests may pass "" to skip.
func fetchCoreOSStream(ctx context.Context, url, expectedSHA256 string) (*coreOSStreamData, error) {
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
	if expectedSHA256 != "" {
		sum := sha256.Sum256(body)
		if got := hex.EncodeToString(sum[:]); got != expectedSHA256 {
			return nil, fmt.Errorf("coreos stream: sha256 mismatch: got %s, want %s", got, expectedSHA256)
		}
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
		Version:     metal.Release,
		ISOUrl:      iso.Location,
		ISOChecksum: iso.SHA256,
	}, nil
}

// DetectCoreOSVersion returns the CoreOS ISO location, checksum, and release
// for the host architecture, picking fcos.json for 4.15-4.18 or scos.json
// for 4.19+ via okdVersion. A malformed okdVersion or an unpinned (major,
// minor) fails fast as a ConfigError.
func (p *Provisioner) DetectCoreOSVersion(ctx context.Context, okdVersion string) (*CoreOSInfo, error) {
	major, minor, ok := parseOKDVersion(okdVersion)
	if !ok {
		return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("invalid OKD version %q: cannot parse major.minor", okdVersion)}
	}
	pin, ok := streamPins[okdVersionKey{Major: major, Minor: minor}]
	if !ok {
		return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("OKD %d.%d is not pinned; update streamPins in coreos.go", major, minor)}
	}
	streamURL := fmt.Sprintf(
		"%s/openshift/installer/%s/data/data/coreos/%s",
		streamRawBaseURL, pin.CommitSHA, streamFileForVersion(major, minor),
	)
	sd, err := fetchCoreOSStream(ctx, streamURL, pin.JSONSHA256)
	if err != nil {
		return nil, &errtypes.ClusterError{Msg: "fetch CoreOS stream info", Err: err}
	}
	return coreOSInfoFromStream(sd)
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
func (p *Provisioner) EnsureCoreOSISO(ctx context.Context, cfg *config.Config, opts Options) (string, error) {
	p.Log.Info("coreos: resolving iso from pinned installer stream metadata")

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
