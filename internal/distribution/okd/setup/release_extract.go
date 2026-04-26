package setup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/download"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// ocExtractTimeout bounds the `oc adm release extract --tools` call. The
// release image is 5-7 GB across 200+ layers; on cold caches or residential
// links the extract regularly takes 4-8 minutes before the tools layer
// completes. 10 minutes leaves margin for the realistic worst case.
const ocExtractTimeout = 10 * time.Minute

// bootstrapOCVersion pins the okd-scos GitHub release used to fetch a
// known-good oc binary for `oc adm release extract`. Independent of the
// user-configured cluster OKD version — the cluster oc is later swapped
// in from the release image. Bumping this requires a release whose
// sha256sum.txt and openshift-client-linux-<v>.tar.gz are published.
const bootstrapOCVersion = "4.18.0-okd-scos.8"

// bootstrapOC ensures oc is available in downloadDir. If a non-empty
// cached binary is present it is reused; otherwise the openshift-client
// tarball is fetched from the pinned okd-scos GitHub release and
// verified against the SHA-256 published alongside it. A failure to
// retrieve or match the checksum is a hard error — there is no silent
// fallback to an unverified binary.
func (p *Phase) bootstrapOC(ctx context.Context, downloadDir string) (string, error) {
	ocPath := filepath.Join(downloadDir, "oc")
	if fi, statErr := os.Stat(ocPath); statErr == nil && fi.Size() > 0 {
		p.Log.Info("tools: bootstrap oc already present", "path", ocPath)
		return ocPath, nil
	}

	assetName := "openshift-client-linux-" + bootstrapOCVersion + ".tar.gz"
	baseURL := "https://github.com/okd-project/okd-scos/releases/download/" + bootstrapOCVersion
	sumsURL := baseURL + "/sha256sum.txt"
	tarballURL := baseURL + "/" + assetName

	checksum, err := download.FetchChecksum(ctx, sumsURL, assetName)
	if err != nil {
		return "", &errtypes.NetworkError{Msg: "failed to fetch bootstrap oc checksum", Err: err}
	}

	archivePath := filepath.Join(downloadDir, assetName)
	p.Log.Info("tools: fetching bootstrap oc", "url", tarballURL)

	if err := download.Download(ctx, &download.Options{
		URL:              tarballURL,
		OutputPath:       archivePath,
		ExpectedChecksum: checksum,
		Description:      "bootstrap-oc",
		Timeout:          3 * time.Minute,
		Logger:           p.Log,
	}); err != nil {
		return "", &errtypes.NetworkError{Msg: "failed to download bootstrap oc", Err: err}
	}

	if err := download.ExtractTarGz(ctx, download.ExtractOptions{
		ArchivePath:    archivePath,
		DestDir:        downloadDir,
		CleanupArchive: true,
		Logger:         p.Log,
	}); err != nil {
		return "", &errtypes.NetworkError{Msg: "failed to extract bootstrap oc", Err: err}
	}

	fi, statErr := os.Stat(ocPath)
	if statErr != nil {
		return "", &errtypes.NetworkError{Msg: "bootstrap oc binary missing after extract", Err: statErr}
	}
	if fi.Size() == 0 {
		return "", &errtypes.NetworkError{Msg: "bootstrap oc binary is empty after extract"}
	}

	if err := system.MakeExecutable(ocPath); err != nil {
		p.Log.Warn("tools: failed to chmod bootstrap oc", "err", err)
	}

	p.Log.Info("tools: bootstrap oc ready", "path", ocPath)
	return ocPath, nil
}

// authMarkers names substrings that registries and oc emit for credential
// failures. Best-effort — a registry whose error envelope drifts from these
// patterns will fall through to *errtypes.ClusterError instead of AuthError.
var authMarkers = []string{
	"unauthorized",
	"authentication",
	"denied",
	"forbidden",
	"no basic auth",
	"401",
	"403",
}

// extractReleaseImage runs `oc adm release extract --tools <ref> --to <destDir>`
// with a bounded timeout. Best-effort registry-auth detection produces
// *errtypes.AuthError; other failures produce *errtypes.ClusterError.
func (p *Phase) extractReleaseImage(ctx context.Context, ocPath, ref, destDir string) error {
	extractCtx, cancel := context.WithTimeout(ctx, ocExtractTimeout)
	defer cancel()

	cmd := exec.CommandContext(extractCtx, ocPath,
		"adm", "release", "extract",
		"--tools", ref,
		"--to", destDir,
	)

	var stderr strings.Builder
	cmd.Stderr = &stderr

	if runErr := cmd.Run(); runErr != nil {
		msg := strings.TrimSpace(stderr.String())
		p.Log.Error("tools: oc adm release extract failed", "ref", ref, "stderr", msg)
		if isAuthError(msg) {
			return &errtypes.AuthError{Msg: fmt.Sprintf("release extract: registry auth failed for %s", ref), Err: runErr}
		}
		return &errtypes.ClusterError{Msg: fmt.Sprintf("release extract failed for %s", ref), Err: runErr}
	}

	return extractReleaseTarballs(ctx, destDir, p.Log)
}

func isAuthError(msg string) bool {
	lower := strings.ToLower(msg)
	for _, marker := range authMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// extractReleaseTarballs extracts the versioned tarballs that
// `oc adm release extract --tools` places in destDir so that
// InstallToolsToSystem finds the bare binaries.
func extractReleaseTarballs(ctx context.Context, destDir string, logger *slog.Logger) error {
	matches, err := filepath.Glob(filepath.Join(destDir, "openshift-*-linux-*.tar.gz"))
	if err != nil {
		return fmt.Errorf("glob release tarballs: %w", err)
	}
	for _, archivePath := range matches {
		if err := download.ExtractTarGz(ctx, download.ExtractOptions{
			ArchivePath:    archivePath,
			DestDir:        destDir,
			CleanupArchive: true,
		}); err != nil {
			return fmt.Errorf("extract %s: %w", filepath.Base(archivePath), err)
		}
		logger.Info("tools: extracted release tarball", "file", filepath.Base(archivePath))
	}
	return nil
}
