package setup

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
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/download"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/httputil"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox/hostssh"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/platform"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
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
	p.Log.Info("coreos: iso found", "file", base)
}

// isoResolution describes how a configured FCOSIso spec was resolved.
type isoResolution int

const (
	isoEmpty    isoResolution = iota // unconfigured: caller globs
	isoResolved                      // configured + present: return path
	isoMissing                       // configured + absent: caller errors
)

// resolveConfiguredISO maps cfg.Provider.Proxmox.FCOSIso to one of three
// states. ":iso/<file>" is resolved relative to hostssh.DefaultProxmoxISODir;
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

func (p *Phase) findOrDownloadFCOSISO(ctx context.Context, cfg *config.Config, opts *Options) (string, error) {
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

	// nodetypes.CoreOSISONamePatterns covers the two official OKD artifact
	// shapes (fedora-coreos-*.iso, scos-*.iso); fcos-*.iso and
	// fedora-coreos.iso are additional local naming conventions for
	// manually-placed ISOs and stay setup-local since hostssh's remote
	// guard never needs to recognize them.
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

	return p.EnsureCoreOSISO(ctx, cfg, &Options{
		BaseOptions: phase.BaseOptions{
			WorkDir: opts.WorkDir,
		},
	})
}

// findNewestISO globs dir against each pattern in order and returns the
// lexicographically newest match from the first pattern with a hit.
func (p *Phase) findNewestISO(dir string, patterns []string) (string, bool) {
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

// minSCOSStreamMajor and minSCOSStreamMinor mark the first OKD release that
// publishes scos.json (Stream CoreOS); earlier 4.x minors (4.15-4.18) ship
// Fedora CoreOS via fcos.json. Every 5.x+ major ships scos.json exclusively.
// The schema is identical at the path the parser walks.
// Verified 2026-04-20 across release-4.14 through release-4.24.
const (
	minSCOSStreamMajor = 4
	minSCOSStreamMinor = 19
)

// streamRawBaseURL is the GitHub raw-content root for openshift/installer.
// Tests override this to an httptest.Server URL for hermetic mocking.
var streamRawBaseURL = "https://raw.githubusercontent.com"

// coreOSStreamPin is the compile-time anchor for one OKD release's stream
// JSON. CommitSHA pins the openshift/installer tree (immutable); JSONSHA256
// is the SHA-256 of the JSON file at that commit, verified before the body
// is parsed. Both must update together on OKD version bumps — see README
// "Bumping the CoreOS stream pin".
type coreOSStreamPin struct {
	CommitSHA  string
	JSONSHA256 string
}

// okdVersionKey identifies one supported OKD release train by (major,
// minor). Keying streamPins on minor alone would let a future 5.x version
// collide with a pinned 4.x entry of the same minor number and silently
// resolve to the wrong installer commit and SHA-256.
type okdVersionKey struct {
	Major int
	Minor int
}

// streamPins maps each supported OKD (major, minor) release train to a
// pinned openshift/installer commit SHA and the expected SHA-256 of its
// stream JSON file. Fetching from release-X.Y branch (mutable) is
// intentionally absent: an attacker who can rewrite the JSON on that branch
// can also rewrite the sha256 field, making DownloadCoreOSISO's integrity
// check meaningless.
//
// 4.15-4.18 share an identical fcos.json sha256, and 4.21-4.23 likewise
// share an identical scos.json sha256 (the file content is byte-equal across
// each of those release branches at their pinned tips — verified genuine
// upstream, not a copy-paste slip); goconst is suppressed because the
// duplication is mechanical, machine-rewritten by scripts/update-coreos-pins.sh,
// and any per-minor drift would surface as a real diff in the next bump PR.
//
// To add or update a pin:
//  1. git ls-remote https://github.com/openshift/installer release-X.Y
//  2. curl -sSfL https://raw.githubusercontent.com/openshift/installer/<SHA>/data/data/coreos/<fcos|scos>.json | sha256sum
//  3. update CommitSHA and JSONSHA256 below; run make test.
//
// Tests may override this var to inject hermetic pin entries.
//
//nolint:goconst,nolintlint // see comment above re: 4.15-4.18 / 4.21-4.23 sha-equal-by-design
var streamPins = map[okdVersionKey]coreOSStreamPin{
	{4, 10}: {CommitSHA: "62137b29c72f4303faeb325dce01bc358d68d2ad", JSONSHA256: "ba2d4f18b19d5de01261e52228d189c221f50302c4bc3b8e585a32668c4f01e5"},
	{4, 11}: {CommitSHA: "64675f82cb5be511953ef6eff2a9d76efa9cfe73", JSONSHA256: "7ed054b02d04baab3eacda3c13e060a30d6d221202be42bf38e5de1c0e155264"},
	{4, 12}: {CommitSHA: "b86064a94ccd47a4547bd16771de98dc36a4abb0", JSONSHA256: "31c97633aed443b33f9e3282b416750c9f4a43ce9fd2c8b3f716052045e0c869"},
	{4, 13}: {CommitSHA: "b3d2f7b8834666c220b88f7aee46ec9160274bcc", JSONSHA256: "7d84d832e0e8c28f52fda566318bb5afdb60829f7e6317cae9e163536e2706e4"},
	{4, 14}: {CommitSHA: "4dd5abdf12a97ef0f32f6774ab79fa8dc6482f34", JSONSHA256: "bbb0651c7363d416c7e38e1f4129345f0804913f736f842cfa2156733e3c7f41"},
	{4, 15}: {CommitSHA: "83c823bf5cb70c42dcbbc93306a570759ac6aaf8", JSONSHA256: "57f52e71f3f351bfdac77b1708e725a287e8df0239df7f6ff0b2883d73b10302"},
	{4, 16}: {CommitSHA: "441e0e5469d5698ce147c092c7c802d7c44b1557", JSONSHA256: "57f52e71f3f351bfdac77b1708e725a287e8df0239df7f6ff0b2883d73b10302"},
	{4, 17}: {CommitSHA: "b102c3acc6afdc1aed628f8d5604a467fba9b8c4", JSONSHA256: "57f52e71f3f351bfdac77b1708e725a287e8df0239df7f6ff0b2883d73b10302"},
	{4, 18}: {CommitSHA: "01812b66eb92aefc97907166e741d41961116eea", JSONSHA256: "57f52e71f3f351bfdac77b1708e725a287e8df0239df7f6ff0b2883d73b10302"},
	{4, 19}: {CommitSHA: "2493db4d14208d90f64b4d5951f5229d5116e725", JSONSHA256: "734ab37d8ac19e8b4c5535c11b1432ffefad9403032d37eda873b1168595ab2c"},
	{4, 20}: {CommitSHA: "349feecf1e7597a0a62cf93c97e0870ac7cff2e3", JSONSHA256: "cc5912af5ae98f6fed3e09e545bc8409ce83843a9fc3b11d06ef315c903d925d"},
	{4, 21}: {CommitSHA: "129c91fe1aad20fee24160703a27ef387048de88", JSONSHA256: "3bfc32f58e48880e3fb6ef56b19f8ba41411ba35416fef2d881d5adaf474600c"},
	{4, 22}: {CommitSHA: "b89b386a406430674b55f4494e828240e445ebb2", JSONSHA256: "3bfc32f58e48880e3fb6ef56b19f8ba41411ba35416fef2d881d5adaf474600c"},
	{4, 23}: {CommitSHA: "95357a6ffee4e19405f99c78277a5aa976a7b9f5", JSONSHA256: "3bfc32f58e48880e3fb6ef56b19f8ba41411ba35416fef2d881d5adaf474600c"},
}

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

// parseOKDVersion extracts major and minor from an OKD version like
// "4.19.0-0.okd-2025-…". The bool is true only when both scanned
// successfully; callers must refuse the request when it is false.
func parseOKDVersion(version string) (major, minor int, ok bool) {
	n, _ := fmt.Sscanf(version, "%d.%d", &major, &minor)
	return major, minor, n == 2
}

// streamFileForVersion returns the data file an OKD (major, minor) release
// publishes: fcos.json for 4.15-4.18, scos.json for 4.19+ and every 5.x+.
func streamFileForVersion(major, minor int) string {
	if major > minSCOSStreamMajor || (major == minSCOSStreamMajor && minor >= minSCOSStreamMinor) {
		return "scos.json"
	}
	return "fcos.json"
}

// fetchCoreOSStream fetches and parses the CoreOS stream JSON at url. When
// expectedSHA256 is non-empty the response body is verified against it before
// parsing; a mismatch is a hard error. Production callers always pass the
// compile-time constant from streamPins; tests may pass "" to skip.
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
		Version:      metal.Release,
		ISOUrl:       iso.Location,
		ISOChecksum:  iso.SHA256,
		Architecture: archKey,
	}, nil
}

// DetectCoreOSVersion returns the CoreOS ISO location, checksum, and release
// for the host architecture. okdVersion picks the right upstream data file:
// 4.15-4.18 → fcos.json (Fedora CoreOS), 4.19+ and every 5.x → scos.json
// (Stream CoreOS). A malformed okdVersion or an unpinned (major, minor)
// fails fast as a ConfigError.
func (p *Phase) DetectCoreOSVersion(ctx context.Context, okdVersion string) (*CoreOSInfo, error) {
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

// DownloadCoreOSISO downloads the CoreOS ISO described by info to destPath.
// An existing file with a matching checksum is reused; mismatch triggers a
// re-download.
func (p *Phase) DownloadCoreOSISO(ctx context.Context, info *CoreOSInfo, destPath string) error {
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
		download.WithProgress(tui.ProgressBarsEnabled()),
	); err != nil {
		return &errtypes.NetworkError{Msg: "download CoreOS ISO", Err: err}
	}

	p.Log.Info("coreos: iso downloaded", "path", destPath)

	return nil
}

// EnsureCoreOSISO ensures the CoreOS ISO is available, downloading to the work
// directory (avoids permission issues with /var/lib/vz).
func (p *Phase) EnsureCoreOSISO(ctx context.Context, cfg *config.Config, opts *Options) (string, error) {
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
