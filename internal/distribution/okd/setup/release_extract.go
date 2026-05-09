package setup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/download"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
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
// in from the release image. Bumping this requires updating
// bootstrapOCChecksum to match the new release's sha256sum.txt entry for
// openshift-client-linux-<v>.tar.gz.
const bootstrapOCVersion = "4.18.0-okd-scos.8"

// bootstrapOCChecksum is the SHA-256 of openshift-client-linux-<bootstrapOCVersion>.tar.gz,
// sourced from the release sha256sum.txt at pin time. Must be updated with
// bootstrapOCVersion. Pinning at compile time means a release-asset swap
// that also replaces sha256sum.txt is caught before any network-fetched
// value is consulted.
const bootstrapOCChecksum = "00c15ce878b6cfa6c93702e79374e56f93e02a0ec300d9095bc92832e207b7f3"

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
	tarballURL := baseURL + "/" + assetName

	archivePath := filepath.Join(downloadDir, assetName)
	p.Log.Info("tools: fetching bootstrap oc", "url", tarballURL)

	if err := download.Fetch(ctx, tarballURL, archivePath,
		download.WithChecksum(bootstrapOCChecksum),
		download.WithDescription("bootstrap-oc"),
		download.WithTimeout(3*time.Minute),
		download.WithLogger(p.Log),
	); err != nil {
		return "", &errtypes.NetworkError{Msg: "failed to download bootstrap oc", Err: err}
	}

	if err := download.ExtractTarGz(ctx, archivePath, downloadDir,
		download.WithCleanupArchive(true),
		download.WithExtractLogger(p.Log),
	); err != nil {
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
// through the canonical Executor so DefaultEnvAllowlist filters the child env.
// Best-effort registry-auth detection produces *errtypes.AuthError; other
// failures produce *errtypes.ClusterError.
func (p *Phase) extractReleaseImage(ctx context.Context, ocPath, ref, destDir string) error {
	extractCtx, cancel := context.WithTimeout(ctx, ocExtractTimeout)
	defer cancel()

	result, err := p.Exec.RunStreamed(extractCtx, ocPath,
		"adm", "release", "extract",
		"--tools", ref,
		"--to", destDir,
	)
	if err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("release extract failed for %s", ref), Err: err}
	}
	if result.ExitCode != 0 {
		msg := strings.TrimSpace(result.Stderr)
		p.Log.Error("tools: oc adm release extract failed", "ref", ref, "stderr", logutil.RedactableStderr(msg))
		// Exit code is the primary signal; stderr-text is a secondary lift.
		// oc exits 1 for most runtime errors including auth; 125 is the
		// container-runtime "failed to start" code. Widen this set if upstream
		// oc changes its exit-code contract (roadmap err:5013fea6).
		execErr := &executor.ExitError{Command: ocPath, ExitCode: result.ExitCode, Stderr: msg}
		if (result.ExitCode == 1 || result.ExitCode == 125) && isAuthError(msg) {
			return &errtypes.AuthError{Msg: fmt.Sprintf("release extract: registry auth failed for %s", ref), Err: execErr}
		}
		return &errtypes.ClusterError{Msg: fmt.Sprintf("release extract failed for %s", ref), Err: execErr}
	}

	return extractReleaseTarballs(ctx, destDir, p.Log)
}

func isAuthError(msg string) bool {
	lower := strings.ToLower(msg)
	return slices.ContainsFunc(authMarkers, func(m string) bool { return strings.Contains(lower, m) })
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
		if err := download.ExtractTarGz(ctx, archivePath, destDir,
			download.WithCleanupArchive(true),
		); err != nil {
			return fmt.Errorf("extract %s: %w", filepath.Base(archivePath), err)
		}
		logger.Info("tools: extracted release tarball", "file", filepath.Base(archivePath))
	}
	return nil
}
