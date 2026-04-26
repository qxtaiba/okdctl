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

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/version"
)

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
during the deploy so the bundle captures the relevant logs.`,
	Example: `  okdctl debug-bundle
  okdctl debug-bundle -o my-cluster.tgz
  okdctl debug-bundle --skip-must-gather`,
	RunE: runDebugBundle,
}

func init() {
	debugBundleCmd.Flags().StringVarP(&debugBundleOutput, "output", "o", "", "write bundle to this path (default: okdctl-debug-<ts>.tgz)")
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

type manifestEntry struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func runDebugBundle(cmd *cobra.Command, _ []string) (retErr error) {
	ctx := cmd.Context()
	bundleID := uuid.NewString()
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

	projectRoot, prErr := resolveProjectRoot()

	sections := collectSections(ctx, addFile, cfg, cfgErr, projectRoot, prErr, bundleAt, bundleID, debugBundleSkipMustGather)

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

func collectSections(ctx context.Context, addFile func(string, []byte) error, cfg *config.Config, cfgErr error, projectRoot string, prErr error, bundleAt time.Time, bundleID string, skipMustGather bool) []manifestEntry {
	secs := []manifestEntry{
		bundleConfig(addFile, cfg, cfgErr),
		bundleLogFile(addFile),
		bundleTerraformState(ctx, addFile, projectRoot, prErr, cfg),
		bundleDoctor(ctx, addFile),
		bundleSystemMeta(addFile, bundleID, bundleAt),
	}
	if skipMustGather {
		secs = append(secs, manifestEntry{Name: "must-gather", Status: "skipped", Message: "--skip-must-gather flag set"})
	} else {
		secs = append(secs, bundleMustGather(ctx, addFile, projectRoot, prErr))
	}
	return secs
}

func bundleConfig(addFile func(string, []byte) error, cfg *config.Config, cfgErr error) manifestEntry {
	if cfgErr != nil {
		return manifestEntry{Name: "config", Status: "skipped", Message: fmt.Sprintf("load config: %v", cfgErr)}
	}
	redacted := redactConfig(cfg)
	data, err := yaml.Marshal(redacted)
	if err != nil {
		return manifestEntry{Name: "config", Status: "failed", Message: fmt.Sprintf("marshal: %v", err)}
	}
	if err := addFile("config.yaml", data); err != nil {
		return manifestEntry{Name: "config", Status: "failed", Message: err.Error()}
	}
	return manifestEntry{Name: "config", Status: "ok"}
}

func bundleLogFile(addFile func(string, []byte) error) manifestEntry {
	if logFile == "" {
		return manifestEntry{
			Name:    "log-file",
			Status:  "skipped",
			Message: "no --log-file set on this invocation; re-run the failing command with --log-file to persist logs",
		}
	}
	data, err := os.ReadFile(logFile)
	if err != nil {
		return manifestEntry{Name: "log-file", Status: "failed", Message: fmt.Sprintf("read %s: %v", logFile, err)}
	}
	if err := addFile("okdctl.log", data); err != nil {
		return manifestEntry{Name: "log-file", Status: "failed", Message: err.Error()}
	}
	return manifestEntry{Name: "log-file", Status: "ok", Message: logFile}
}

func bundleTerraformState(ctx context.Context, addFile func(string, []byte) error, projectRoot string, prErr error, cfg *config.Config) manifestEntry {
	if prErr != nil {
		return manifestEntry{Name: "terraform-state", Status: "skipped", Message: fmt.Sprintf("project root: %v", prErr)}
	}
	tfEnv := "production"
	if cfg != nil {
		tfEnv = phase.GetTerraformEnv(cfg)
	}
	tfDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", tfEnv)
	if _, err := os.Stat(filepath.Join(tfDir, "terraform.tfstate")); os.IsNotExist(err) {
		return manifestEntry{Name: "terraform-state", Status: "skipped", Message: "no terraform.tfstate in " + tfDir}
	}
	tfExec := executor.New(executor.WithWorkDir(tfDir))
	result, runErr := tfExec.Run(ctx, "terraform", "state", "list")
	if runErr != nil {
		return manifestEntry{Name: "terraform-state", Status: "failed", Message: fmt.Sprintf("terraform state list: %v", runErr)}
	}
	if result.ExitCode != 0 {
		msg := strings.TrimSpace(result.Stderr)
		if msg == "" {
			msg = fmt.Sprintf("terraform state list exited %d", result.ExitCode)
		}
		return manifestEntry{Name: "terraform-state", Status: "failed", Message: msg}
	}
	if err := addFile("terraform-state-list.txt", []byte(result.Stdout)); err != nil {
		return manifestEntry{Name: "terraform-state", Status: "failed", Message: err.Error()}
	}
	return manifestEntry{Name: "terraform-state", Status: "ok"}
}

func bundleMustGather(ctx context.Context, addFile func(string, []byte) error, projectRoot string, prErr error) manifestEntry {
	if prErr != nil {
		return manifestEntry{Name: "must-gather", Status: "skipped", Message: fmt.Sprintf("project root: %v", prErr)}
	}
	if _, err := osexec.LookPath("oc"); err != nil {
		return manifestEntry{Name: "must-gather", Status: "skipped", Message: "oc not found on PATH; install oc or run okdctl deploy first"}
	}
	workDir := filepath.Join(projectRoot, "okd-install")
	kubeconfig := filepath.Join(phase.ClusterConfigDir(workDir), "auth", "kubeconfig")
	if _, err := os.Stat(kubeconfig); err != nil {
		return manifestEntry{Name: "must-gather", Status: "skipped", Message: "kubeconfig not found at " + kubeconfig}
	}
	mgDir, err := os.MkdirTemp("", "okdctl-must-gather-*")
	if err != nil {
		return manifestEntry{Name: "must-gather", Status: "failed", Message: fmt.Sprintf("create temp dir: %v", err)}
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
		return manifestEntry{Name: "must-gather", Status: "failed", Message: fmt.Sprintf("oc adm must-gather: %v", mgErr)}
	}
	if mgResult.ExitCode != 0 {
		msg := strings.TrimSpace(mgResult.Stderr)
		if msg == "" {
			msg = fmt.Sprintf("oc adm must-gather exited %d", mgResult.ExitCode)
		}
		return manifestEntry{Name: "must-gather", Status: "failed", Message: msg}
	}
	if err := tarDirInto(addFile, mgDir, "must-gather/"); err != nil {
		return manifestEntry{Name: "must-gather", Status: "failed", Message: fmt.Sprintf("archive must-gather: %v", err)}
	}
	return manifestEntry{Name: "must-gather", Status: "ok"}
}

// tarDirInto walks srcDir and calls addFile for each regular file, prefixing
// each entry name with bundlePrefix. Reads go through os.Root so symlinks
// cannot redirect reads outside srcDir (TOCTOU-safe).
func tarDirInto(addFile func(string, []byte) error, srcDir, bundlePrefix string) error {
	root, err := os.OpenRoot(srcDir)
	if err != nil {
		return fmt.Errorf("open root %s: %w", srcDir, err)
	}
	defer func() { _ = root.Close() }()
	return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, relErr := filepath.Rel(srcDir, path)
		if relErr != nil {
			return relErr
		}
		f, openErr := root.Open(rel)
		if openErr != nil {
			return fmt.Errorf("open %s: %w", path, openErr)
		}
		data, readErr := io.ReadAll(f)
		_ = f.Close()
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		return addFile(bundlePrefix+rel, data)
	})
}

func bundleDoctor(ctx context.Context, addFile func(string, []byte) error) manifestEntry {
	data, err := collectDoctorOutput(ctx)
	if err != nil {
		return manifestEntry{Name: "doctor", Status: "failed", Message: err.Error()}
	}
	if data == nil {
		return manifestEntry{Name: "doctor", Status: "skipped", Message: "doctor is only supported on linux"}
	}
	if err := addFile("doctor.txt", data); err != nil {
		return manifestEntry{Name: "doctor", Status: "failed", Message: err.Error()}
	}
	return manifestEntry{Name: "doctor", Status: "ok"}
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
		return manifestEntry{Name: "system-meta", Status: "failed", Message: fmt.Sprintf("marshal: %v", err)}
	}
	if err := addFile("system-meta.yaml", data); err != nil {
		return manifestEntry{Name: "system-meta", Status: "failed", Message: err.Error()}
	}
	return manifestEntry{Name: "system-meta", Status: "ok"}
}
