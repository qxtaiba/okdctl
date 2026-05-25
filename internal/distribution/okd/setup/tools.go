package setup

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
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

// archAMD64 / archARM64 are the two arch keys okdctl ships (Linux amd64,
// Linux arm64). They appear as map keys in every per-tool checksum table —
// extracted as constants so a typo in any one site is a compile error
// rather than a silent miss.
const (
	archAMD64 = "amd64"
	archARM64 = "arm64"
)

// yqChecksumsByArch holds the SHA-256 for the yq_linux_<arch> binary at yqVersion,
// sourced from the official checksums release asset. Must be updated with yqVersion.
var yqChecksumsByArch = map[string]string{
	archAMD64: "654d2943ca1d3be2024089eb4f270f4070f491a0610481d128509b2834870049",
	archARM64: "ceea73d4c86f2e5c91926ee0639157121f5360da42beeb8357783d79c2cc6a1d",
}

// helmChecksumsByArch holds the SHA-256 for the helm-<helmVersion>-linux-<arch>.tar.gz
// archive, sourced from get.helm.sh/<archive>.sha256sum. Must be updated when
// helmVersion changes — pinning the checksum locally removes the runtime
// FetchChecksum dependency on the same origin as the artifact.
var helmChecksumsByArch = map[string]string{
	archAMD64: "ee88b3c851ae6466a3de507f7be73fe94d54cbf2987cbaa3d1a3832ea331f2cd",
	archARM64: "7944e3defd386c76fd92d9e6fec5c2d65a323f6fadc19bfb5e704e3eee10348e",
}

// sopsChecksumsByArch holds the SHA-256 for the sops-<sopsVersion>.linux.<arch>
// binary, sourced from github.com/getsops/sops/releases/download/<v>/sops-<v>.checksums.txt.
// Must be updated when sopsVersion changes.
var sopsChecksumsByArch = map[string]string{
	archAMD64: "5488e32bc471de7982ad895dd054bbab3ab91c417a118426134551e9626e4e85",
	archARM64: "16564c6b181d88505d9e0dfef62771894293d85cde5884d9b1a843859eee174b",
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

func (p *Phase) installTool(ctx context.Context, tool externalTool) error {
	if isToolInstalled(tool) {
		p.Log.Info("tools: already installed", "tool", string(tool))
		return nil
	}

	if tool == toolTerraform {
		return p.installTerraform(ctx)
	}

	arch := platform.DownloadArch()
	archReplacer := strings.NewReplacer("{arch}", arch)

	var spec binaryInstallSpec
	switch tool {
	case toolYQ:
		spec = binaryInstallSpec{
			name:             "yq",
			versionFlag:      "--version",
			url:              archReplacer.Replace(yqURLTemplate),
			embeddedChecksum: yqChecksumsByArch[arch],
		}
	case toolHelm:
		spec = binaryInstallSpec{
			name:             "helm",
			versionFlag:      "version",
			archiveBinary:    "helm",
			stripComponents:  1,
			url:              archReplacer.Replace(helmURLTemplate),
			embeddedChecksum: helmChecksumsByArch[arch],
		}
	case toolSops:
		spec = binaryInstallSpec{
			name:             "sops",
			versionFlag:      "--version",
			url:              archReplacer.Replace(sopsURLTemplate),
			embeddedChecksum: sopsChecksumsByArch[arch],
		}
	default:
		return fmt.Errorf("tools: no installer for %s (install manually)", tool)
	}

	return p.installBinary(ctx, &spec)
}

func (p *Phase) installTerraform(ctx context.Context) error {
	p.Log.Info("tools: installing terraform via hashicorp repository")

	switch p.OS.Family {
	case platform.FamilyDebian:
		if err := installHashiCorpDebianRepo(ctx, p.OS.Codename); err != nil {
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
	p.Log.Info("tools: terraform installed", "version", version)
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
		return download.Fetch(ctx, spec.url, f.Name(),
			download.WithFetchChecksum(expectedChecksum),
			download.WithDescription(spec.name),
			download.WithTimeout(2*time.Minute),
			download.WithLogger(p.Log),
		)
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
		if err := download.ExtractTarGz(ctx, tempFile, extractDir,
			download.WithStripComponents(spec.stripComponents),
			download.WithCleanupArchive(true),
			download.WithExtractLogger(p.Log),
		); err != nil {
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

func (p *Phase) installBinaryToPath(ctx context.Context, srcPath, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
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
	for line := range strings.Lines(string(output)) {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return "unknown"
}

// expectedHashiCorpGPGFingerprint is the canonical fingerprint for the
// HashiCorp release signing key. Verified against the key before it is
// installed as a system trust root; a mismatch aborts the deploy so a MITM
// during key fetch cannot plant a persistent malicious trust root.
const expectedHashiCorpGPGFingerprint = "798AEC654E5C15428C8E42EEAA16FCBCA621E701"

func installHashiCorpDebianRepo(ctx context.Context, codename string) error {
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

	if codename == "" {
		return fmt.Errorf("failed to detect debian codename: VERSION_CODENAME not set in /etc/os-release")
	}

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
	for line := range strings.Lines(string(out)) {
		line = strings.TrimRight(line, "\n")
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
