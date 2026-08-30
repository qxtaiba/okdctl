package setup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/download"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

// ocExtractTimeout bounds oc adm release extract; the 5-7GB image can take 4-8
// min cold, so 10 min covers the worst case.
const ocExtractTimeout = 10 * time.Minute

// bootstrapOCVersion pins the okd-scos release for bootstrap oc, independent of
// the cluster's configured version; bump bootstrapOCChecksum together with it.
const bootstrapOCVersion = "4.18.0-okd-scos.8"

// bootstrapOCChecksum is bootstrapOCVersion's tarball SHA-256; overridable in
// tests, never reassigned in production.
var bootstrapOCChecksum = "00c15ce878b6cfa6c93702e79374e56f93e02a0ec300d9095bc92832e207b7f3"

// bootstrapOCBaseURL is the GitHub releases base URL, overridable in tests via httptest.Server.
var bootstrapOCBaseURL = "https://github.com/okd-project/okd-scos/releases/download/" + bootstrapOCVersion

// bootstrapOC fetches and verifies the pinned okd-scos oc tarball into
// downloadDir (reusing a cached binary); a checksum mismatch or fetch failure
// is a hard error, never silent.
func (p *Phase) bootstrapOC(ctx context.Context, downloadDir string) (string, error) {
	ocPath := filepath.Join(downloadDir, "oc")
	if fi, statErr := os.Stat(ocPath); statErr == nil && fi.Size() > 0 {
		p.Log.Info("tools: bootstrap oc already present", "path", ocPath)
		return ocPath, nil
	}

	assetName := "openshift-client-linux-" + bootstrapOCVersion + ".tar.gz"
	tarballURL := bootstrapOCBaseURL + "/" + assetName

	archivePath := filepath.Join(downloadDir, assetName)
	p.Log.Info("tools: fetching bootstrap oc", "url", tarballURL)

	if err := download.Fetch(
		ctx, tarballURL, archivePath,
		download.WithFetchChecksum(bootstrapOCChecksum),
		download.WithDescription("bootstrap-oc"),
		download.WithTimeout(3*time.Minute),
		download.WithLogger(p.Log),
		download.WithProgress(logutil.ProgressBarsEnabled()),
	); err != nil {
		return "", &errtypes.NetworkError{Msg: "download bootstrap oc", Err: err}
	}

	if err := download.ExtractTarGz(
		ctx, archivePath, downloadDir,
		download.WithExtractCleanupArchive(true),
		download.WithExtractLogger(p.Log),
	); err != nil {
		return "", &errtypes.NetworkError{Msg: "extract bootstrap oc", Err: err}
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

// authTextMarkers are substrings registries/oc emit for auth failures;
// best-effort, unmatched falls through to ClusterError.
var authTextMarkers = []string{
	"unauthorized",
	"forbidden",
	"no basic auth",
}

// authStatusRegex matches 401/403 only as standalone tokens so a digest or byte
// count containing them isn't misclassified.
var authStatusRegex = regexp.MustCompile(`\b(401|403)\b`)

// extractReleaseImage runs oc adm release extract --tools via the canonical
// Executor; auth failures map to AuthError, others to ClusterError.
func (p *Phase) extractReleaseImage(ctx context.Context, ocPath, ref, destDir string) error {
	extractCtx, cancel := context.WithTimeout(ctx, ocExtractTimeout)
	defer cancel()

	result, err := p.Exec.RunStreamed(
		extractCtx, ocPath,
		"adm", "release", "extract",
		"--tools", ref,
		"--to", destDir,
	)
	// Checked first: RunStreamed returns ctx.Err() on a ctx-killed process, so this
	// distinguishes user Ctrl-C (→130) from the 10m timeout (ClusterError).
	if cerr := extractCtx.Err(); cerr != nil {
		if errors.Is(cerr, context.Canceled) {
			return fmt.Errorf("release extract cancelled: %w", cerr)
		}
		return &errtypes.ClusterError{Msg: fmt.Sprintf("release extract timed out after %s", ocExtractTimeout), Err: cerr}
	}
	if err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("release extract failed for %s", ref), Err: err}
	}
	if result.ExitCode != 0 {
		msg := strings.TrimSpace(result.Stderr)
		p.Log.Warn("tools: oc adm release extract failed", "ref", ref, "stderr", logutil.RedactableStderr(msg))
		// Exit code is primary, stderr secondary: oc exits 1 for most errors
		// (incl. auth), 125 for runtime start failures.
		execErr := &executor.ExitError{Command: ocPath, ExitCode: result.ExitCode, Stderr: msg}
		if (result.ExitCode == 1 || result.ExitCode == 125) && isAuthError(msg) {
			return &errtypes.AuthError{Msg: fmt.Sprintf("release extract: registry auth failed for %s", ref), Err: execErr}
		}
		return &errtypes.ClusterError{Msg: fmt.Sprintf("release extract failed for %s", ref), Err: execErr}
	}

	p.Log.Info("tools: release extract completed", "ref", ref, "duration", result.Duration)

	return extractReleaseTarballs(ctx, destDir, p.Log)
}

func isAuthError(msg string) bool {
	lower := strings.ToLower(msg)
	if slices.ContainsFunc(authTextMarkers, func(m string) bool { return strings.Contains(lower, m) }) {
		return true
	}
	return authStatusRegex.MatchString(lower)
}

// extractReleaseTarballs unpacks the tarballs oc adm release extract leaves in
// destDir, so InstallToolsToSystem finds bare binaries.
func extractReleaseTarballs(ctx context.Context, destDir string, logger *slog.Logger) error {
	matches, err := filepath.Glob(filepath.Join(destDir, "openshift-*-linux-*.tar.gz"))
	if err != nil {
		return fmt.Errorf("glob release tarballs: %w", err)
	}
	for _, archivePath := range matches {
		if err := download.ExtractTarGz(
			ctx, archivePath, destDir,
			download.WithExtractCleanupArchive(true),
		); err != nil {
			return fmt.Errorf("extract %s: %w", filepath.Base(archivePath), err)
		}
		logger.Info("tools: extracted release tarball", "file", filepath.Base(archivePath))
	}
	return nil
}
