package setup

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/download"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/httputil"
	"github.com/qxtaiba/okdctl/internal/platform"
	"github.com/qxtaiba/okdctl/internal/system"
)

// Tool binary fetch URLs and version pins. The {arch} placeholder is
// expanded via strings.NewReplacer at install time.
const (
	helmVersion = "v3.17.3"
	sopsVersion = "v3.9.4"
	// yqVersion is a trust anchor: changing it requires updating yqChecksumsByArch.
	yqVersion = "v4.45.1"

	helmURLTemplate = "https://get.helm.sh/helm-" + helmVersion + "-linux-{arch}.tar.gz"
	sopsURLTemplate = "https://github.com/getsops/sops/releases/download/" + sopsVersion + "/sops-" + sopsVersion + ".linux.{arch}"
	yqURLTemplate   = "https://github.com/mikefarah/yq/releases/download/" + yqVersion + "/yq_linux_{arch}"
)

// yqChecksumsByArch holds the SHA-256 for the yq_linux_<arch> binary at yqVersion,
// sourced from the official checksums release asset. Must be updated with yqVersion.
var yqChecksumsByArch = map[string]string{
	"amd64": "654d2943ca1d3be2024089eb4f270f4070f491a0610481d128509b2834870049",
	"arm64": "ceea73d4c86f2e5c91926ee0639157121f5360da42beeb8357783d79c2cc6a1d",
}

//go:embed hashicorp.repo
var hashicorpRPMRepo []byte

type externalTool string

const (
	toolTerraform externalTool = "terraform"
	toolYQ        externalTool = "yq"
	toolHelm      externalTool = "helm"
	toolSops      externalTool = "sops"
)

func addonRequiredTools(cfg *config.Config) []externalTool {
	seen := make(map[string]bool)
	var tools []externalTool

	for _, a := range addon.Enabled(cfg) {
		tp, ok := a.(addon.ToolProvider)
		if !ok {
			continue
		}
		for _, spec := range tp.RequiredTools() {
			if seen[spec.Name] {
				continue
			}
			seen[spec.Name] = true
			tools = append(tools, externalTool(spec.Name))
		}
	}
	return tools
}

func isToolInstalled(tool externalTool) bool {
	return executor.CommandExists(string(tool))
}

// InstallExternalTools installs terraform, yq, and any addon-declared tools.
func (p *Phase) InstallExternalTools(ctx context.Context, cfg *config.Config) error {
	tools := append([]externalTool{toolTerraform, toolYQ}, addonRequiredTools(cfg)...)
	for _, tool := range tools {
		if err := p.installTool(ctx, tool); err != nil {
			return &errtypes.ConfigError{Msg: fmt.Sprintf("failed to install %s", tool), Err: err}
		}
	}
	return nil
}

var binaryToolMeta = map[externalTool]struct {
	name                     string
	versionFlag              string
	archiveBinary            string
	stripComponents          int
	urlTemplate              string
	checksumURLTemplate      string
	checksumFilenameTemplate string
	checksumsByArch          map[string]string
}{
	toolYQ: {name: "yq", versionFlag: "--version", urlTemplate: yqURLTemplate, checksumsByArch: yqChecksumsByArch},
	toolHelm: {
		name: "helm", versionFlag: "version", archiveBinary: "helm", stripComponents: 1,
		urlTemplate:              helmURLTemplate,
		checksumURLTemplate:      "https://get.helm.sh/helm-" + helmVersion + "-linux-{arch}.tar.gz.sha256sum",
		checksumFilenameTemplate: "helm-" + helmVersion + "-linux-{arch}.tar.gz",
	},
	toolSops: {
		name: "sops", versionFlag: "--version",
		urlTemplate:              sopsURLTemplate,
		checksumURLTemplate:      "https://github.com/getsops/sops/releases/download/" + sopsVersion + "/sops-" + sopsVersion + ".checksums.txt",
		checksumFilenameTemplate: "sops-" + sopsVersion + ".linux.{arch}",
	},
}

func (p *Phase) installTool(ctx context.Context, tool externalTool) error {
	if isToolInstalled(tool) {
		p.Log.Info(fmt.Sprintf("tools: %s already installed", tool))
		return nil
	}

	if tool == toolTerraform {
		return p.installTerraform(ctx)
	}

	meta, ok := binaryToolMeta[tool]
	if !ok {
		p.Log.Warn(fmt.Sprintf("tools: no installer for %s, skipping (install manually)", tool))
		return nil
	}

	arch := platform.DownloadArch()
	archReplacer := strings.NewReplacer("{arch}", arch)
	spec := binaryInstallSpec{
		name:             meta.name,
		url:              archReplacer.Replace(meta.urlTemplate),
		versionFlag:      meta.versionFlag,
		archiveBinary:    meta.archiveBinary,
		stripComponents:  meta.stripComponents,
		checksumURL:      archReplacer.Replace(meta.checksumURLTemplate),
		checksumFilename: archReplacer.Replace(meta.checksumFilenameTemplate),
		embeddedChecksum: meta.checksumsByArch[arch],
	}
	return p.installBinary(ctx, &spec)
}

func (p *Phase) installTerraform(ctx context.Context) error {
	p.Log.Info("tools: installing terraform via hashicorp repository")

	switch p.OS.Family {
	case platform.FamilyDebian:
		if err := installHashiCorpDebianRepo(ctx); err != nil {
			return err
		}
	default: // rhel family
		// Build-time-pinned .repo content avoids trusting the .repo URL at deploy
		// time. Written root-owned (AtomicWrite) so a non-root invoking user cannot
		// later edit the gpgkey URL and poison subsequent dnf operations.
		repoPath := "/etc/yum.repos.d/hashicorp.repo"
		if err := system.AtomicWrite(repoPath, hashicorpRPMRepo, 0o644); err != nil {
			return fmt.Errorf("failed to write HashiCorp repository file: %w", err)
		}
	}

	p.Log.Info("tools: hashicorp repository added")

	if err := p.Pkg.Install(ctx, []string{"terraform"}, p.Log); err != nil {
		return fmt.Errorf("failed to install terraform: %w", err)
	}

	if !isToolInstalled(toolTerraform) {
		return fmt.Errorf("terraform installation verification failed")
	}

	version := getToolVersion(ctx, "terraform", "--version")
	p.Log.Info(fmt.Sprintf("tools: terraform installed (%s)", version))
	return nil
}

type binaryInstallSpec struct {
	name             string
	url              string
	versionFlag      string
	archiveBinary    string // if non-empty, download is a tar.gz; value is the binary path within the archive
	stripComponents  int
	checksumURL      string // if non-empty, SHA-256 is fetched and verified before the binary is installed
	checksumFilename string // filename to look up in the checksums file
	embeddedChecksum string // used when the vendor does not publish a compatible sha256sum file
}

func (p *Phase) installBinary(ctx context.Context, spec *binaryInstallSpec) error {
	p.Log.Info("tools: installing", "tool", spec.name)

	var expectedChecksum string
	switch {
	case spec.checksumURL != "":
		// Verify the binary against the vendor-published SHA-256 before
		// it lands in BinDir; prevents silent compromise on a poisoned CDN.
		var err error
		expectedChecksum, err = download.FetchChecksum(ctx, spec.checksumURL, spec.checksumFilename)
		if err != nil {
			return fmt.Errorf("failed to fetch checksum for %s: %w", spec.name, err)
		}
	case spec.embeddedChecksum != "":
		expectedChecksum = spec.embeddedChecksum
	}

	tempFile, err := system.WriteTempFile(spec.name+"-download-*", 0o600, func(f *os.File) error {
		return download.Download(ctx, &download.Options{
			URL: spec.url, OutputPath: f.Name(), ExpectedChecksum: expectedChecksum,
			Description: spec.name, Timeout: 2 * time.Minute, Logger: p.Log,
		})
	})
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", spec.name, err)
	}
	defer func() { _ = os.Remove(tempFile) }()

	srcPath := tempFile
	if spec.archiveBinary != "" {
		extractDir, err := os.MkdirTemp(os.TempDir(), spec.name+"-extract-*")
		if err != nil {
			return fmt.Errorf("failed to create extract directory: %w", err)
		}
		defer func() { _ = os.RemoveAll(extractDir) }()
		if err := download.ExtractTarGz(ctx, download.ExtractOptions{
			ArchivePath: tempFile, DestDir: extractDir,
			StripComponents: spec.stripComponents, CleanupArchive: true, Logger: p.Log,
		}); err != nil {
			return fmt.Errorf("failed to extract %s: %w", spec.name, err)
		}
		srcPath = filepath.Join(extractDir, spec.archiveBinary)
	}

	if err := p.installBinaryToPath(ctx, srcPath, spec.name); err != nil {
		return err
	}
	if !isToolInstalled(externalTool(spec.name)) {
		return fmt.Errorf("%s installation verification failed", spec.name)
	}
	p.Log.Info("tools: installed", "tool", spec.name, "version", getToolVersion(ctx, spec.name, spec.versionFlag))
	return nil
}

func (p *Phase) installBinaryToPath(_ context.Context, srcPath, name string) error {
	binDir := phase.BinDirOrDefault(p.BinDir)
	destPath := filepath.Join(binDir, name)

	if err := system.CopyFile(srcPath, destPath); err != nil {
		return fmt.Errorf("failed to copy %s to %s: %w", name, binDir, err)
	}

	if err := system.MakeExecutable(destPath); err != nil {
		return fmt.Errorf("failed to set executable permissions on %s: %w", name, err)
	}

	return nil
}

func getToolVersion(ctx context.Context, tool, flag string) string {
	output, err := system.OutputCaptured(ctx, tool, flag)
	if err != nil {
		return "unknown"
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) > 0 {
		return lines[0]
	}
	return "unknown"
}

// expectedHashiCorpGPGFingerprint is the canonical fingerprint for the
// HashiCorp release signing key. Verified against the key before it is
// installed as a system trust root; a mismatch aborts the deploy so a MITM
// during key fetch cannot plant a persistent malicious trust root.
const expectedHashiCorpGPGFingerprint = "AA16FCBCA621E70139936A4C798AEC654FA7E1A1"

func installHashiCorpDebianRepo(ctx context.Context) error {
	gpgPath := "/usr/share/keyrings/hashicorp-archive-keyring.gpg"

	gpgTmp, err := system.WriteTempFile("hashicorp-gpg", 0o600, func(f *os.File) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://apt.releases.hashicorp.com/gpg", http.NoBody)
		if err != nil {
			return err
		}
		resp, err := httputil.New(httputil.TimeoutShort).Do(req)
		if err != nil {
			return fmt.Errorf("fetch hashicorp gpg key: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("fetch hashicorp gpg key: status %d", resp.StatusCode)
		}
		if _, err := io.Copy(f, resp.Body); err != nil {
			return fmt.Errorf("copy hashicorp gpg key: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to download HashiCorp GPG key: %w", err)
	}
	defer func() { _ = os.Remove(gpgTmp) }()

	if err := verifyHashiCorpGPGFingerprint(ctx, gpgTmp); err != nil {
		return err
	}

	// Refuse to overwrite an existing keyring belonging to a different key.
	// gpg --import-options show-only accepts both armored and binary inputs,
	// so the same fingerprint helper handles the on-disk dearmored form.
	if _, statErr := os.Stat(gpgPath); statErr == nil {
		if err := verifyHashiCorpGPGFingerprint(ctx, gpgPath); err != nil {
			return &errtypes.ConfigError{
				Msg: fmt.Sprintf("existing keyring %s has an unexpected fingerprint; remove it manually to proceed", gpgPath),
				Err: err,
			}
		}
	} else if err := system.RunCaptured(ctx, "gpg", "--dearmor", "-o", gpgPath, gpgTmp); err != nil {
		return fmt.Errorf("failed to dearmor HashiCorp GPG key: %w", err)
	}

	codeCmd := exec.CommandContext(ctx, "lsb_release", "-cs")
	codeOut, err := codeCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to detect debian codename: %w", err)
	}
	codename := strings.TrimSpace(string(codeOut))

	listContent := fmt.Sprintf("deb [signed-by=%s] https://apt.releases.hashicorp.com %s main\n", gpgPath, codename)
	listPath := "/etc/apt/sources.list.d/hashicorp.list"
	listTmp, err := system.WriteTempFile("hashicorp-list", 0o600, func(f *os.File) error {
		_, err := f.WriteString(listContent)
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to write HashiCorp repo list: %w", err)
	}
	defer func() { _ = os.Remove(listTmp) }()

	if err := system.CopyFile(listTmp, listPath); err != nil {
		return fmt.Errorf("failed to install HashiCorp repo list: %w", err)
	}
	return system.RunCaptured(ctx, "apt-get", "update")
}

func verifyHashiCorpGPGFingerprint(ctx context.Context, armoredKeyPath string) error {
	out, err := system.OutputCaptured(ctx,
		"gpg", "--with-fingerprint", "--with-colons",
		"--import-options", "show-only", "--import", armoredKeyPath,
	)
	if err != nil {
		return fmt.Errorf("gpg fingerprint check: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "fpr:") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 10 {
			continue
		}
		got := strings.ToUpper(strings.ReplaceAll(fields[9], " ", ""))
		if got == expectedHashiCorpGPGFingerprint {
			return nil
		}
		return &errtypes.ConfigError{
			Msg: fmt.Sprintf("hashicorp gpg key fingerprint mismatch: got %s, want %s", got, expectedHashiCorpGPGFingerprint),
		}
	}
	return &errtypes.ConfigError{Msg: "hashicorp gpg key fingerprint not found in gpg output"}
}
