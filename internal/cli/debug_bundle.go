package cli

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/version"
)

type bundleCategory string

const (
	categoryMustGather     bundleCategory = "must-gather"
	categoryTerraformState bundleCategory = "terraform-state"
	categoryDoctor         bundleCategory = "doctor"
	categoryConfig         bundleCategory = "config"
	categoryLogFile        bundleCategory = "log-file"
	categorySystemMeta     bundleCategory = "system-meta"
)

// maxBundleFileBytes caps individual file reads in tarDirInto.
// must-gather routinely emits multi-GB dumps; the bundle is for troubleshooting,
// not full dump retention. var (not const) so tests can lower it without
// allocating tens of MB of zeros.
var maxBundleFileBytes int64 = 50 * 1024 * 1024

var (
	debugBundleOutput         string
	debugBundleSkipMustGather bool
)

var debugBundleCmd = &cobra.Command{
	Use:   "debug-bundle",
	Short: "Collect a support bundle for troubleshooting",
	Long: `Collect a tarball containing redacted configuration, recent log
output, oc adm must-gather results, terraform state summary,
okdctl doctor results, and system metadata.

The output is safe to attach to a support ticket — credentials are
redacted and the raw terraform state file is never included.

Run this after a failed deploy, passing the same --log-file you used
during the deploy so the bundle captures the relevant logs.

Pass --quiet to suppress progress logs to stderr when only the bundle
file is needed (e.g. in scripts or CI).`,
	Example: `  okdctl debug-bundle
  okdctl debug-bundle --output-file my-cluster.tgz
  okdctl debug-bundle --skip-must-gather`,
	RunE: runDebugBundle,
}

func init() {
	debugBundleCmd.Flags().StringVar(&debugBundleOutput, flagOutputFile, "", "write bundle to this path (default: okdctl-debug-<ts>.tgz)")
	debugBundleCmd.Flags().BoolVar(&debugBundleSkipMustGather, "skip-must-gather", false, "skip oc adm must-gather (faster, omits cluster diagnostics)")
	rootCmd.AddCommand(debugBundleCmd)
}

// bundleManifest is the top-level structure written as manifest.yaml inside
// the tarball. Support engineers read this first to understand bundle contents
// and why any section was skipped or failed.
type bundleManifest struct {
	BundleID  string          `json:"bundle_id"`
	BundleAt  string          `json:"bundle_at"`
	Version   string          `json:"version"`
	GitCommit string          `json:"git_commit"`
	Platform  string          `json:"platform"`
	Sections  []manifestEntry `json:"sections"`
}

type bundleStatus string

const (
	bundleStatusOK      bundleStatus = "ok"
	bundleStatusSkipped bundleStatus = "skipped"
	bundleStatusFailed  bundleStatus = "failed"
)

type manifestEntry struct {
	Name    bundleCategory `json:"name"`
	Status  bundleStatus   `json:"status"`
	Message string         `json:"message,omitempty"`
}

func runDebugBundle(cmd *cobra.Command, _ []string) (retErr error) {
	ctx := cmd.Context()
	bundleID := system.NewUUIDv4()
	bundleAt := time.Now().UTC()

	outPath := debugBundleOutput
	if outPath == "" {
		outPath = fmt.Sprintf("okdctl-debug-%s.tgz", bundleAt.Format("20060102-150405"))
	}

	tui.Info("collecting debug bundle", tui.LF("bundle_id", bundleID), tui.LF("output", outPath))

	cfg, cfgErr := loadConfig(cfgFile)

	if info, err := os.Lstat(outPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("output path %q is a symlink; refusing to follow", outPath)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat bundle file: %w", err)
	}
	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return fmt.Errorf("create bundle file: %w", err)
	}
	defer func() { _ = f.Close() }()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	// Deferred closes run on every return path so a failure between tar writes
	// and explicit Close still finalizes the archive — without this a mid-run
	// error leaves a truncated .tgz that gunzip reports as corrupt.
	defer func() {
		if cErr := tw.Close(); cErr != nil && retErr == nil {
			retErr = fmt.Errorf("finalize tar: %w", cErr)
		}
		if cErr := gz.Close(); cErr != nil && retErr == nil {
			retErr = fmt.Errorf("finalize gzip: %w", cErr)
		}
	}()

	addFile := func(name string, data []byte) error {
		hdr := &tar.Header{
			Name:    name,
			Mode:    0o600,
			Size:    int64(len(data)),
			ModTime: bundleAt,
		}
		if wErr := tw.WriteHeader(hdr); wErr != nil {
			return fmt.Errorf("tar header %s: %w", name, wErr)
		}
		if _, wErr := tw.Write(data); wErr != nil {
			return fmt.Errorf("tar write %s: %w", name, wErr)
		}
		return nil
	}

	addStream := func(hdr *tar.Header, r io.Reader) error {
		if wErr := tw.WriteHeader(hdr); wErr != nil {
			return fmt.Errorf("tar header %s: %w", hdr.Name, wErr)
		}
		if _, wErr := io.Copy(tw, r); wErr != nil {
			return fmt.Errorf("tar write %s: %w", hdr.Name, wErr)
		}
		return nil
	}

	projectRoot, prErr := resolveProjectRoot()

	sections := collectSections(ctx, addFile, addStream, cfg, cfgErr, projectRoot, prErr, bundleAt, bundleID, debugBundleSkipMustGather)

	manifest := bundleManifest{
		BundleID:  bundleID,
		BundleAt:  bundleAt.Format(time.RFC3339),
		Version:   version.Version,
		GitCommit: version.GitCommit,
		Platform:  version.Platform,
		Sections:  sections,
	}
	manifestBytes, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := addFile("manifest.yaml", manifestBytes); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	tui.Info("debug bundle written", tui.LF("path", outPath), tui.LF("bundle_id", bundleID))
	return nil
}

func collectSections(ctx context.Context, addFile func(string, []byte) error, addStream func(*tar.Header, io.Reader) error, cfg *config.Config, cfgErr error, projectRoot string, prErr error, bundleAt time.Time, bundleID string, skipMustGather bool) []manifestEntry {
	secs := []manifestEntry{
		bundleConfig(addFile, cfg, cfgErr),
		bundleLogFile(addFile),
		bundleTerraformState(ctx, addFile, projectRoot, prErr, cfg),
		bundleDoctor(ctx, addFile),
		bundleSystemMeta(addFile, bundleID, bundleAt),
	}
	if skipMustGather {
		secs = append(secs, manifestEntry{Name: categoryMustGather, Status: bundleStatusSkipped, Message: "--skip-must-gather flag set"})
	} else {
		secs = append(secs, bundleMustGather(ctx, addStream, projectRoot, prErr))
	}
	return secs
}

// safeMessage converts err to a string safe for inclusion in the bundle
// manifest. If err implements Redacted() any the redacted form is used,
// preventing a future error type from leaking credentials into the
// operator-shared bundle. Mirrors logutil.redactAny's dispatch shape.
func safeMessage(err error) string {
	if err == nil {
		return ""
	}
	if r, ok := err.(interface{ Redacted() any }); ok {
		return fmt.Sprint(r.Redacted())
	}
	return err.Error()
}

func bundleConfig(addFile func(string, []byte) error, cfg *config.Config, cfgErr error) manifestEntry {
	if cfgErr != nil {
		return manifestEntry{Name: categoryConfig, Status: bundleStatusSkipped, Message: fmt.Sprintf("load config: %v", cfgErr)}
	}
	redacted := redactConfig(cfg)
	data, err := yaml.Marshal(redacted)
	if err != nil {
		return manifestEntry{Name: categoryConfig, Status: bundleStatusFailed, Message: fmt.Sprintf("marshal: %v", err)}
	}
	if err := addFile("config.yaml", data); err != nil {
		return manifestEntry{Name: categoryConfig, Status: bundleStatusFailed, Message: safeMessage(err)}
	}
	return manifestEntry{Name: categoryConfig, Status: bundleStatusOK}
}

func bundleLogFile(addFile func(string, []byte) error) manifestEntry {
	if logFile == "" {
		return manifestEntry{
			Name:    categoryLogFile,
			Status:  bundleStatusSkipped,
			Message: "no --log-file set on this invocation; re-run the failing command with --log-file to persist logs",
		}
	}
	data, err := os.ReadFile(logFile)
	if err != nil {
		return manifestEntry{Name: categoryLogFile, Status: bundleStatusFailed, Message: fmt.Sprintf("read %s: %v", logFile, err)}
	}
	if err := addFile("okdctl.log", data); err != nil {
		return manifestEntry{Name: categoryLogFile, Status: bundleStatusFailed, Message: safeMessage(err)}
	}
	return manifestEntry{Name: categoryLogFile, Status: bundleStatusOK, Message: logFile}
}

func bundleTerraformState(ctx context.Context, addFile func(string, []byte) error, projectRoot string, prErr error, cfg *config.Config) manifestEntry {
	if prErr != nil {
		return manifestEntry{Name: categoryTerraformState, Status: bundleStatusSkipped, Message: fmt.Sprintf("project root: %v", prErr)}
	}
	tfEnv := "production"
	if cfg != nil {
		tfEnv = phase.GetTerraformEnv(cfg)
	}
	tfDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", tfEnv)
	if _, err := os.Stat(filepath.Join(tfDir, "terraform.tfstate")); os.IsNotExist(err) {
		return manifestEntry{Name: categoryTerraformState, Status: bundleStatusSkipped, Message: "no terraform.tfstate in " + tfDir}
	}
	tfExec := executor.New(executor.WithWorkDir(tfDir))
	result, runErr := tfExec.Run(ctx, "terraform", "state", "list")
	if runErr != nil {
		return manifestEntry{Name: categoryTerraformState, Status: bundleStatusFailed, Message: fmt.Sprintf("terraform state list: %v", runErr)}
	}
	if result.ExitCode != 0 {
		msg := strings.TrimSpace(result.Stderr)
		if msg == "" {
			msg = fmt.Sprintf("terraform state list exited %d", result.ExitCode)
		}
		return manifestEntry{Name: categoryTerraformState, Status: bundleStatusFailed, Message: msg}
	}
	if err := addFile("terraform-state-list.txt", []byte(result.Stdout)); err != nil {
		return manifestEntry{Name: categoryTerraformState, Status: bundleStatusFailed, Message: safeMessage(err)}
	}
	return manifestEntry{Name: categoryTerraformState, Status: bundleStatusOK}
}

func bundleMustGather(ctx context.Context, addStream func(*tar.Header, io.Reader) error, projectRoot string, prErr error) manifestEntry {
	if prErr != nil {
		return manifestEntry{Name: categoryMustGather, Status: bundleStatusSkipped, Message: fmt.Sprintf("project root: %v", prErr)}
	}
	if _, err := osexec.LookPath("oc"); err != nil {
		return manifestEntry{Name: categoryMustGather, Status: bundleStatusSkipped, Message: "oc not found on PATH; install oc or run okdctl deploy first"}
	}
	workDir := filepath.Join(projectRoot, "okd-install")
	kubeconfig := filepath.Join(phase.ClusterConfigDir(workDir), "auth", "kubeconfig")
	if _, err := os.Stat(kubeconfig); err != nil {
		return manifestEntry{Name: categoryMustGather, Status: bundleStatusSkipped, Message: "kubeconfig not found at " + kubeconfig}
	}
	mgDir, err := os.MkdirTemp("", "okdctl-must-gather-*")
	if err != nil {
		return manifestEntry{Name: categoryMustGather, Status: bundleStatusFailed, Message: fmt.Sprintf("create temp dir: %v", err)}
	}
	defer func() { _ = os.RemoveAll(mgDir) }()

	gctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	mgExec := executor.New()
	mgResult, mgErr := mgExec.Run(gctx, "oc", "adm", "must-gather",
		"--kubeconfig="+kubeconfig,
		"--dest-dir="+mgDir,
	)
	if mgErr != nil {
		return manifestEntry{Name: categoryMustGather, Status: bundleStatusFailed, Message: fmt.Sprintf("oc adm must-gather: %v", mgErr)}
	}
	if mgResult.ExitCode != 0 {
		msg := strings.TrimSpace(mgResult.Stderr)
		if msg == "" {
			msg = fmt.Sprintf("oc adm must-gather exited %d", mgResult.ExitCode)
		}
		return manifestEntry{Name: categoryMustGather, Status: bundleStatusFailed, Message: msg}
	}
	truncated, archErr := tarDirInto(addStream, mgDir, "must-gather/")
	if archErr != nil {
		return manifestEntry{Name: categoryMustGather, Status: bundleStatusFailed, Message: fmt.Sprintf("archive must-gather: %v", archErr)}
	}
	if len(truncated) > 0 {
		tui.Warn("must-gather files truncated to 50 MB", tui.LF("files", strings.Join(truncated, ", ")))
		return manifestEntry{Name: categoryMustGather, Status: bundleStatusOK, Message: "truncated (>50 MB): " + strings.Join(truncated, ", ")}
	}
	return manifestEntry{Name: categoryMustGather, Status: bundleStatusOK}
}

// tarDirInto walks srcDir and streams each regular file into addStream,
// prefixing entry names with bundlePrefix. Reads go through os.Root so
// symlinks cannot redirect reads outside srcDir (TOCTOU-safe). Files larger
// than maxBundleFileBytes are capped; their relative paths are returned in
// the truncated slice so callers can record the truncation.
func tarDirInto(addStream func(*tar.Header, io.Reader) error, srcDir, bundlePrefix string) (truncated []string, err error) {
	root, openErr := os.OpenRoot(srcDir)
	if openErr != nil {
		return nil, fmt.Errorf("open root %s: %w", srcDir, openErr)
	}
	defer func() { _ = root.Close() }()
	walkErr := filepath.WalkDir(srcDir, func(path string, d os.DirEntry, wErr error) error {
		if wErr != nil {
			return wErr
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, relErr := filepath.Rel(srcDir, path)
		if relErr != nil {
			return relErr
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return fmt.Errorf("stat %s: %w", path, infoErr)
		}
		actualSize := info.Size()
		cappedSize := actualSize
		if cappedSize > maxBundleFileBytes {
			cappedSize = maxBundleFileBytes
		}
		f, openFErr := root.Open(rel)
		if openFErr != nil {
			return fmt.Errorf("open %s: %w", path, openFErr)
		}
		hdr := &tar.Header{
			Name:    bundlePrefix + rel,
			Mode:    0o600,
			Size:    cappedSize,
			ModTime: info.ModTime(),
		}
		streamErr := addStream(hdr, io.LimitReader(f, maxBundleFileBytes))
		_ = f.Close()
		if streamErr != nil {
			return fmt.Errorf("stream %s: %w", path, streamErr)
		}
		if actualSize > maxBundleFileBytes {
			truncated = append(truncated, rel)
		}
		return nil
	})
	return truncated, walkErr
}

func bundleDoctor(ctx context.Context, addFile func(string, []byte) error) manifestEntry {
	data, err := collectDoctorOutput(ctx)
	if err != nil {
		return manifestEntry{Name: categoryDoctor, Status: bundleStatusFailed, Message: safeMessage(err)}
	}
	if data == nil {
		return manifestEntry{Name: categoryDoctor, Status: bundleStatusSkipped, Message: "doctor is only supported on linux"}
	}
	if err := addFile("doctor.json", data); err != nil {
		return manifestEntry{Name: categoryDoctor, Status: bundleStatusFailed, Message: safeMessage(err)}
	}
	return manifestEntry{Name: categoryDoctor, Status: bundleStatusOK}
}

func bundleSystemMeta(addFile func(string, []byte) error, bundleID string, bundleAt time.Time) manifestEntry {
	hostname, _ := os.Hostname()
	meta := map[string]any{
		"bundle_id":  bundleID,
		"bundle_at":  bundleAt.Format(time.RFC3339),
		"okdctl":     version.Version,
		"git_commit": version.GitCommit,
		"build_date": version.BuildDate,
		"go_version": version.GoVersion,
		"platform":   version.Platform,
		"goos":       runtime.GOOS,
		"goarch":     runtime.GOARCH,
		"num_cpu":    runtime.NumCPU(),
		"hostname":   hostname,
	}
	data, err := yaml.Marshal(meta)
	if err != nil {
		return manifestEntry{Name: categorySystemMeta, Status: bundleStatusFailed, Message: fmt.Sprintf("marshal: %v", err)}
	}
	if err := addFile("system-meta.yaml", data); err != nil {
		return manifestEntry{Name: categorySystemMeta, Status: bundleStatusFailed, Message: safeMessage(err)}
	}
	return manifestEntry{Name: categorySystemMeta, Status: bundleStatusOK}
}
