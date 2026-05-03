//go:build linux

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/platform"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

func init() {
	doctorCmd.RunE = runDoctor
}

// Severity levels ordered from least to most alarming.
type severity int

const (
	sevPass severity = iota
	sevWarn
	sevFail
)

// checkResult is a check's outcome. If items is non-empty, printResult
// renders it as a per-item sub-list and detail is ignored; otherwise
// detail is rendered as a single-line result next to the bracketed label.
// sev is the aggregate severity (the worst item severity in the sub-list
// case, or set directly in the single-line case).
type checkResult struct {
	sev    severity
	detail string
	items  []checkItem
}

// checkItem is one row in a multi-item check result (e.g. per-binary status
// under the tools-and-packages check). Each item has its own severity so a
// rollup check can mix [ok] / [warn] / [fail] rows under one title.
type checkItem struct {
	sev  severity
	name string
	note string
}

type check struct {
	name string
	desc string
	fn   func(context.Context) checkResult
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	defer fmt.Println()

	checks := []check{
		{"host os", "platform and operator-mode detection", checkHostOS},
		{"root check", "guard against running as root (deploy uses sudo internally)", checkNotRoot},
		{"bin dir on path", "effective bin dir present on $PATH", checkPath},
		{"bin dir", "configured bin dir is writable by the invoking user", checkBinDir},
		{"tools and packages", "host tools, installable clis, and system packages", checkBinaries},
		{"sudo", "non-interactive (nopasswd) for long-running installs", checkSudo},
		{"ssh public key", "default key for vm provisioning", checkSSHKey},
		{"pull secret", "valid okd registry pull secret", checkPullSecret},
		{"disk space", "at least 20 gb free in $home for install artifacts", checkDiskSpace},
		{"host ports", "53, 80, 443, 6443, 22623, 8080 available for bind", checkPorts},
	}

	fmt.Println()
	fmt.Println("🩺 " + tui.HighlightStyle.Render(fmt.Sprintf("doctor: running %d environment checks", len(checks))))
	fmt.Println()

	var fails, warns int
	for _, c := range checks {
		r := c.fn(ctx)
		printResult(c, r)
		switch r.sev {
		case sevFail:
			fails++
		case sevWarn:
			warns++
		}
	}

	switch {
	case fails > 0:
		tui.Error("doctor: failing checks block deploy", tui.LF("failing", fails), tui.LF("warnings", warns))
		return &errtypes.ConfigError{Msg: "preflight checks failed"}
	case warns > 0:
		tui.Warn(fmt.Sprintf("doctor: %d warning(s): deploy may proceed but review the warnings above", warns))
	default:
		tui.Info("doctor: environment looks ready")
	}
	return nil
}

// severityMarkers returns the styled icon, the styled bracketed label,
// and the raw (unstyled) label text for a given severity. Callers use the
// raw text for column-width math when rendering aligned sub-item lists.
func severityMarkers(sev severity) (icon, label, rawLabel string) {
	switch sev {
	case sevPass:
		rawLabel = "[ok]"
		icon = tui.SuccessStyle.Render("✓")
		label = tui.SuccessStyle.Render(rawLabel)
	case sevWarn:
		rawLabel = "[warn]"
		icon = tui.WarningStyle.Render("⚠")
		label = tui.WarningStyle.Render(rawLabel)
	case sevFail:
		rawLabel = "[fail]"
		icon = tui.ErrorStyle.Render("✗")
		label = tui.ErrorStyle.Render(rawLabel)
	}
	return
}

// printResult renders one check as either a two-line block (title + single
// result line) or a title followed by a per-item sub-list. A blank line
// follows either shape so check blocks remain visually distinct.
func printResult(c check, r checkResult) {
	icon, aggregateLabel, _ := severityMarkers(r.sev)

	title := c.name
	if c.desc != "" {
		title += tui.MutedStyle.Render(": " + c.desc)
	}
	fmt.Println("  " + icon + " " + title)

	if len(r.items) > 0 {
		// Sub-list: each item on its own line, labels aligned to the
		// widest possible label ("[fail]" / "[warn]" at 6 chars).
		const maxLabelWidth = 6
		for _, item := range r.items {
			_, itemLabel, itemRawLabel := severityMarkers(item.sev)
			padding := strings.Repeat(" ", maxLabelWidth-len(itemRawLabel)+2)
			line := "      " + itemLabel + padding + item.name
			if item.note != "" {
				line += tui.MutedStyle.Render(" (" + item.note + ")")
			}
			fmt.Println(line)
		}
	} else {
		fmt.Println("      " + aggregateLabel + " " + r.detail)
	}

	fmt.Println()
}

// checkHostOS identifies the host OS by parsing /etc/os-release.
func checkHostOS(_ context.Context) checkResult {
	host, err := platform.Detect()
	if err != nil {
		return checkResult{sev: sevFail, detail: fmt.Sprintf("cannot read /etc/os-release: %v", err)}
	}
	return checkResult{sev: sevPass, detail: fmt.Sprintf("%s %s (%s family)", host.ID, host.Version, host.Family)}
}

// checkNotRoot is a secondary guard — main.preflight() already refuses to
// run as root, so by the time doctor runs we know we are not root. We keep
// the check so it shows up green in the output for user confidence.
func checkNotRoot(_ context.Context) checkResult {
	if os.Geteuid() == 0 {
		return checkResult{sev: sevFail, detail: "running as root; okdctl uses sudo internally"}
	}
	return checkResult{sev: sevPass, detail: "running as unprivileged user"}
}

// binDirResolution pairs the resolved bin dir with a flag set when the
// config file failed to load. The flag demotes pass→warn and suffixes the
// detail so a malformed YAML never reads as green.
type binDirResolution struct {
	Dir        string
	LoadFailed bool
}

// doctorLoadedCfg loads cfgFile once per doctor invocation and caches the
// result so binDir resolution doesn't re-read the file.
var doctorLoadedCfg = sync.OnceValues(func() (*config.Config, error) {
	loader := config.NewLoader()
	return loader.LoadFile(cfgFile)
})

func resolveBinDirForDoctor() binDirResolution {
	loaded, err := doctorLoadedCfg()
	if err != nil {
		return binDirResolution{Dir: phase.ResolveBinDir(nil), LoadFailed: true}
	}
	return binDirResolution{Dir: phase.ResolveBinDir(loaded)}
}

var effectiveBinDir = sync.OnceValue(resolveBinDirForDoctor)

func (r binDirResolution) suffix(s string) string {
	if r.LoadFailed {
		return s + " (config unavailable; using default)"
	}
	return s
}

func (r binDirResolution) demote(sev severity) severity {
	if r.LoadFailed && sev == sevPass {
		return sevWarn
	}
	return sev
}

func checkPath(_ context.Context) checkResult {
	r := effectiveBinDir()
	if slices.Contains(filepath.SplitList(os.Getenv("PATH")), r.Dir) {
		return checkResult{sev: r.demote(sevPass), detail: r.suffix(r.Dir + " found on $PATH")}
	}
	if r.Dir == phase.PreflightBinDir() {
		return checkResult{sev: sevWarn, detail: r.suffix(r.Dir + " missing from $PATH; okdctl will prepend it at startup")}
	}
	return checkResult{sev: sevFail, detail: r.suffix(r.Dir + " missing from $PATH; add it to your shell profile (okdctl cannot auto-prepend a config-only dir)")}
}

// checkBinDir probes the effective bin dir for existence and user-write
// access. User-configured dirs that are not user-writable are a fail because
// setup runs under sudo and would install root-owned binaries.
func checkBinDir(_ context.Context) checkResult {
	r := effectiveBinDir()
	defaultDir := r.Dir == phase.DefaultBinDir
	if _, err := os.Stat(r.Dir); err != nil {
		if os.IsNotExist(err) {
			if defaultDir {
				return checkResult{sev: sevWarn, detail: r.suffix(r.Dir + " does not exist; setup will create it as root via sudo")}
			}
			return checkResult{sev: sevFail, detail: r.suffix(r.Dir + " does not exist; create it first (e.g. mkdir -p)")}
		}
		return checkResult{sev: sevFail, detail: r.suffix(r.Dir + " stat failed: " + err.Error())}
	}
	if !system.IsDirWritable(r.Dir) {
		if defaultDir {
			return checkResult{sev: sevWarn, detail: r.suffix(r.Dir + " not writable by invoking user; setup will install as root via sudo")}
		}
		return checkResult{sev: sevFail, detail: r.suffix(r.Dir + " not writable by invoking user; setup runs under sudo so binaries will be root-owned — chown to your user if you want to manage them later")}
	}
	return checkResult{sev: r.demote(sevPass), detail: r.suffix(r.Dir + " writable")}
}

// checkBinaries reports per-item status for three categories: host tools
// that must already exist (missing = fail), installable CLIs that setup
// downloads into /usr/local/bin (missing = warn), and system packages
// that setup installs via dnf/apt (missing = warn). The system package
// list is a mirror of setup.installSystemPackages — keep in sync.
func checkBinaries(_ context.Context) checkResult {
	hostBinaries := []string{"curl", "ssh", "git"}
	installableTools := []string{"oc", "openshift-install", "terraform"}
	systemPackages := []string{"coreos-installer", "haproxy", "dnsmasq"}

	var items []checkItem
	worst := sevPass

	probe := func(name string, missingSev severity, note string) {
		if _, err := exec.LookPath(name); err != nil {
			items = append(items, checkItem{sev: missingSev, name: name, note: note})
			worst = max(worst, missingSev)
			return
		}
		items = append(items, checkItem{sev: sevPass, name: name})
	}

	for _, name := range hostBinaries {
		probe(name, sevFail, "missing; required before anything else will work")
	}
	for _, name := range installableTools {
		probe(name, sevWarn, "will be downloaded during setup")
	}
	for _, name := range systemPackages {
		probe(name, sevWarn, "will be installed via package manager")
	}

	// Apache binary name varies by distro: httpd on rhel-family, apache2
	// on debian-family. If either is on PATH, treat apache as installed.
	apacheFound := false
	for _, bin := range []string{"httpd", "apache2"} {
		if _, err := exec.LookPath(bin); err == nil {
			apacheFound = true
			break
		}
	}
	if apacheFound {
		items = append(items, checkItem{sev: sevPass, name: "apache"})
	} else {
		items = append(items, checkItem{sev: sevWarn, name: "apache", note: "will be installed via package manager"})
		worst = max(worst, sevWarn)
	}

	return checkResult{sev: worst, items: items}
}

// checkSudo verifies that sudo is present and can escalate without
// prompting. A failing check is a warning rather than a fail because the
// deploy re-exec gate can still succeed with an interactive password — but
// the user should know up front whether the sudo prompt will appear.
func checkSudo(ctx context.Context) checkResult {
	if _, err := exec.LookPath("sudo"); err != nil {
		return checkResult{sev: sevFail, detail: "sudo not installed"}
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := system.HasPasswordlessSudo(cctx); err != nil {
		return checkResult{sev: sevWarn, detail: "sudo requires a password; deploy will prompt"}
	}
	return checkResult{sev: sevPass, detail: "nopasswd enabled"}
}

func checkSSHKey(_ context.Context) checkResult {
	home, err := os.UserHomeDir()
	if err != nil {
		return checkResult{sev: sevWarn, detail: "cannot resolve home directory"}
	}
	candidates := []string{
		filepath.Join(home, ".ssh", "id_ed25519.pub"),
		filepath.Join(home, ".ssh", "id_rsa.pub"),
		filepath.Join(home, ".ssh", "id_ecdsa.pub"),
	}
	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return checkResult{sev: sevPass, detail: p}
		}
	}
	return checkResult{sev: sevWarn, detail: "no default ssh public key found; you will need to specify one in the wizard"}
}

// checkPullSecret reads the effective config file and verifies the path
// at cfg.Files.PullSecret. If no config exists yet (normal pre-deploy
// state), warns and directs the user to the wizard. If the config
// points at a file that does not exist, is not valid JSON, or has an
// empty 'auths' map, fails.
func checkPullSecret(_ context.Context) checkResult {
	configPath := cfgFile
	if configPath == "" {
		configPath = "okdctl.yaml"
	}

	if _, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			return checkResult{
				sev:    sevWarn,
				detail: "no config yet at " + configPath + "; run 'okdctl deploy' to set the pull secret path in the wizard",
			}
		}
		return checkResult{sev: sevFail, detail: fmt.Sprintf("cannot stat config: %v", err)}
	}

	loader := config.NewLoader()
	cfg, err := loader.LoadFile(configPath)
	if err != nil {
		return checkResult{sev: sevFail, detail: fmt.Sprintf("cannot load config: %v", err)}
	}

	if cfg.Files.PullSecret == "" {
		return checkResult{sev: sevFail, detail: "files.pull_secret not set in " + configPath + "; run 'okdctl deploy' to configure"}
	}

	path := system.ExpandPath(cfg.Files.PullSecret)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return checkResult{sev: sevFail, detail: "not found at " + path + " (download from https://console.redhat.com/openshift/install/pull-secret)"}
		}
		return checkResult{sev: sevFail, detail: err.Error()}
	}

	var parsed struct {
		Auths map[string]json.RawMessage `json:"auths"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return checkResult{sev: sevFail, detail: fmt.Sprintf("invalid json: %v", err)}
	}
	if parsed.Auths == nil {
		return checkResult{sev: sevFail, detail: "missing or malformed 'auths' field: not a valid okd pull secret"}
	}
	if len(parsed.Auths) == 0 {
		return checkResult{sev: sevFail, detail: "'auths' is empty: pull secret has no registry entries"}
	}
	return checkResult{sev: sevPass, detail: path}
}

// checkDiskSpace checks that the home directory has at least 20 GB free.
// The deploy process downloads OKD tools, builds custom ISOs, and holds
// terraform state, all of which live under ~/okd-install by default.
func checkDiskSpace(_ context.Context) checkResult {
	const minGB = 20

	u, err := user.Current()
	if err != nil {
		return checkResult{sev: sevWarn, detail: "cannot resolve user home"}
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(u.HomeDir, &st); err != nil {
		return checkResult{sev: sevWarn, detail: fmt.Sprintf("statfs failed: %v", err)}
	}
	// Bsize is int64 on linux but a filesystem block size is always
	// positive in practice; the bound check exists to satisfy gosec
	// G115 without a nolint directive.
	if st.Bsize <= 0 {
		return checkResult{sev: sevWarn, detail: "statfs returned a non-positive block size"}
	}
	freeBytes := st.Bavail * uint64(st.Bsize)
	freeGB := freeBytes / (1024 * 1024 * 1024)
	if freeGB < minGB {
		return checkResult{sev: sevFail, detail: fmt.Sprintf("%d gb free in %s (need at least %d gb)", freeGB, u.HomeDir, minGB)}
	}
	return checkResult{sev: sevPass, detail: fmt.Sprintf("%d gb free in %s", freeGB, u.HomeDir)}
}

// checkPorts probes each port okdctl's deploy will bind by trying
// to connect to 127.0.0.1:<port>. Connect-probe beats bind-probe for the
// preflight use case: the real deploy binds happen via sudo (haproxy,
// dnsmasq, apache), so the relevant question is "is something already
// there?" — not "can this unprivileged user bind right now?". Catches
// the common case of services bound on 0.0.0.0 or 127.0.0.1; misses
// services bound only on a specific non-loopback address.
func checkPorts(ctx context.Context) checkResult {
	ports := []int{53, 80, 443, 6443, 22623, 8080}

	var busy []string
	for _, p := range ports {
		if isPortInUse(ctx, p) {
			busy = append(busy, strconv.Itoa(p))
		}
	}
	if len(busy) > 0 {
		return checkResult{sev: sevWarn, detail: "in use: " + strings.Join(busy, ", ") + " (stop the conflicting service before deploy)"}
	}
	return checkResult{sev: sevPass, detail: "53, 80, 443, 6443, 22623, 8080 all free"}
}

func isPortInUse(ctx context.Context, port int) bool {
	d := net.Dialer{Timeout: 200 * time.Millisecond}
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
