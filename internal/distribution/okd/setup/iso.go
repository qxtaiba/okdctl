package setup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/system"
)

// nodeISOFingerprint hashes the coreos-installer inputs for one node ISO. The
// base ISO path (filename encodes the FCOS version) is used instead of its
// content to avoid hashing a multi-GB file on every invocation.
func nodeISOFingerprint(liveKargs, destKargs []string, sshKey, basePath string) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s",
		strings.Join(liveKargs, "\x1f"),
		strings.Join(destKargs, "\x1f"),
		sshKey,
		basePath,
	)
	return hex.EncodeToString(h.Sum(nil))
}

// BuildCustomISOs produces a per-node FCOS ISO with coreos-installer that
// embeds the node's ignition URL, role, and static-IP kernel arguments. A
// node whose output ISO and .fp-<name> fingerprint both match the current
// inputs is skipped; the fingerprint is written after a successful build.
func (p *Phase) BuildCustomISOs(ctx context.Context, cfg *config.Config, opts *Options) error {
	isoDir := filepath.Join(opts.WorkDir, "custom-isos")
	if err := system.EnsureDir(isoDir); err != nil {
		return err
	}

	if !executor.CommandExists("coreos-installer") {
		return &errtypes.ConfigError{Msg: "coreos-installer not found - please install it first"}
	}

	fcosISO, err := p.findOrDownloadFCOSISO(ctx, cfg, opts)
	if err != nil {
		return &errtypes.NetworkError{Msg: "failed to find or download FCOS ISO", Err: err}
	}

	nodes, err := p.BuildNodeList(cfg)
	if err != nil {
		return &errtypes.ConfigError{Msg: "failed to build node list", Err: err}
	}

	var sshKey string
	if cfg.Files.SSHPublicKey != "" {
		keyPath := system.ExpandPath(cfg.Files.SSHPublicKey)
		b, readErr := os.ReadFile(keyPath)
		if readErr != nil {
			return fmt.Errorf("failed to read ssh public key %s: %w", keyPath, readErr)
		}
		sshKey = strings.TrimSpace(string(b))
	}

	gateway, netmask, dns, iface := ExtractNetworkConfig(cfg)

	for _, node := range nodes {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		ignitionURL, err := BuildIgnitionURLForNode(cfg, node.Role)
		if err != nil {
			return err
		}
		kargsParams := &LiveKargsParams{
			NodeIP:      node.IP,
			Gateway:     gateway,
			Netmask:     netmask,
			DNS:         dns,
			Interface:   iface,
			IgnitionURL: ignitionURL,
		}
		fp := nodeISOFingerprint(
			BuildLiveKargs(kargsParams),
			BuildDestKargs(kargsParams),
			sshKey, fcosISO,
		)

		fpFile := filepath.Join(isoDir, ".fp-"+node.Name)
		isoOut := filepath.Join(isoDir, node.Name+".iso")
		if system.FileExists(isoOut) {
			if stored, statErr := os.ReadFile(fpFile); statErr == nil && strings.TrimSpace(string(stored)) == fp {
				p.Log.Info("iso: skipping unchanged", "node", node.Name)
				continue
			}
		}

		p.Log.Info("iso: building custom coreos iso", "node", node.Name)

		if err := p.buildNodeISO(ctx, cfg, node, fcosISO, isoDir, sshKey, fp, fpFile); err != nil {
			return &errtypes.ClusterError{Msg: fmt.Sprintf("failed to build ISO for %s", node.Name), Err: err}
		}
	}

	return nil
}

func writePreInstallScript(script string) (string, error) {
	return system.WriteTempFile("pre-install-*.sh", 0o750, func(f *os.File) error {
		if _, err := f.WriteString(script); err != nil {
			return fmt.Errorf("failed to write pre-install script: %w", err)
		}
		return nil
	})
}

// ignitionConfig is a narrow subset of the Ignition v3.3 spec — only the
// fields writeInstallerTriggerIgnition emits. See
// https://coreos.github.io/ignition/specs/v3.3.0/ for the full schema.
type ignitionConfig struct {
	Ignition ignitionMeta    `json:"ignition"`
	Storage  ignitionStorage `json:"storage"`
	Passwd   *ignitionPasswd `json:"passwd,omitempty"`
}

type ignitionMeta struct {
	Version string `json:"version"`
}

type ignitionStorage struct {
	Files []ignitionFile `json:"files"`
}

type ignitionFile struct {
	Path     string           `json:"path"`
	Mode     int              `json:"mode"`
	Contents ignitionContents `json:"contents"`
}

type ignitionContents struct {
	Source string `json:"source"`
}

type ignitionPasswd struct {
	Users []ignitionUser `json:"users"`
}

type ignitionUser struct {
	Name              string   `json:"name"`
	SSHAuthorizedKeys []string `json:"sshAuthorizedKeys"`
}

// writeInstallerTriggerIgnition creates a temp Ignition config that seeds
// /etc/coreos/installer.d/ so coreos-installer.service's
// ConditionDirectoryNotEmpty is satisfied before systemd evaluates it.
func writeInstallerTriggerIgnition(sshKey string) (string, error) {
	ign := ignitionConfig{
		Ignition: ignitionMeta{Version: "3.3.0"},
		Storage: ignitionStorage{
			Files: []ignitionFile{{
				Path:     "/etc/coreos/installer.d/00-install-trigger.yaml",
				Mode:     420,
				Contents: ignitionContents{Source: "data:,fetch-retries%3A%200%0A"},
			}},
		},
	}
	if sshKey != "" {
		ign.Passwd = &ignitionPasswd{
			Users: []ignitionUser{{Name: "core", SSHAuthorizedKeys: []string{sshKey}}},
		}
	}

	data, err := json.Marshal(ign)
	if err != nil {
		return "", fmt.Errorf("failed to marshal installer trigger ignition: %w", err)
	}

	return system.WriteTempFile("installer-trigger-*.ign", 0o600, func(f *os.File) error {
		if _, err := f.Write(data); err != nil {
			return fmt.Errorf("failed to write installer trigger ignition: %w", err)
		}
		return nil
	})
}

func (p *Phase) buildNodeISO(ctx context.Context, cfg *config.Config, node NodeInfo, fcosISO, outputDir, sshKey, fp, fpFile string) error {
	isoName := fmt.Sprintf("%s.iso", node.Name)
	outputPath := filepath.Join(outputDir, isoName)

	if system.FileExists(outputPath) {
		if err := os.Remove(outputPath); err != nil {
			return fmt.Errorf("failed to remove existing ISO %s: %w", outputPath, err)
		}
	}

	gateway, netmask, dns, iface := ExtractNetworkConfig(cfg)
	ignitionURL, err := BuildIgnitionURLForNode(cfg, node.Role)
	if err != nil {
		return err
	}

	kargsParams := &LiveKargsParams{
		NodeIP:      node.IP,
		Gateway:     gateway,
		Netmask:     netmask,
		DNS:         dns,
		Interface:   iface,
		IgnitionURL: ignitionURL,
	}

	args := []string{"iso", "customize"}
	for _, karg := range BuildLiveKargs(kargsParams) {
		args = append(args, "--live-karg-append", karg)
	}
	for _, karg := range BuildDestKargs(kargsParams) {
		args = append(args, "--dest-karg-append", karg)
	}

	// All nodes use a pre-install script that discovers the OS disk by serial
	// via lsblk. Workers also get the data disk wiped; for bootstrap/masters
	// the data serial is absent so the wipe is safely skipped.
	script, err := templates.RenderPreInstall(templates.PreInstallData{
		OSSerial:   "OS-DISK",
		DataSerial: "CEPH-DATA",
	})
	if err != nil {
		return err
	}
	scriptPath, err := writePreInstallScript(script)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(scriptPath) }()

	// Live ignition seeds /etc/coreos/installer.d/ so coreos-installer.service
	// starts (its ConditionDirectoryNotEmpty fires before the pre-install
	// script can populate it). Using a karg like coreos.inst.install_dev
	// would override the serial-based disk discovery in the pre-install script.
	triggerPath, err := writeInstallerTriggerIgnition(sshKey)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(triggerPath) }()
	args = append(args,
		"--live-ignition", triggerPath,
		"--pre-install", scriptPath,
		"-o", outputPath, fcosISO,
	)

	_, err = p.Exec.RunChecked(ctx, "coreos-installer", args...)
	if err != nil {
		return &errtypes.ClusterError{Msg: "coreos-installer failed", Err: err}
	}

	if writeErr := system.AtomicWriteString(fpFile, fp, 0o644); writeErr != nil {
		p.Log.Warn("iso: failed to write build fingerprint", "node", node.Name, "err", writeErr)
	}

	return nil
}
