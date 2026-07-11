// Package doctor implements okdctl's host-prerequisite checks: OS and
// operator-mode detection, tool and package presence, sudo mode, pull-secret
// validity, disk space, and port availability. Rendering lives in the cli
// package.
package doctor

import (
	"context"
	"encoding/json"
	"errors"
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
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/platform"
	"github.com/qxtaiba/okdctl/internal/system"
)

// Severity levels ordered from least to most alarming.
type Severity int

// Pass, Warn, and Fail order check outcomes from least to most alarming;
// multi-item rollups keep the worst item severity.
const (
	Pass Severity = iota
	Warn
	Fail
)

// Result is a check's outcome. If Items is non-empty, the renderer shows a
// per-item sub-list and Detail is ignored; otherwise Detail is rendered as a
// single-line result next to the bracketed label. Sev is the aggregate
// severity (the worst item severity in the sub-list case, or set directly in
// the single-line case).
type Result struct {
	Sev    Severity
	Detail string
	Items  []Item
}

// Item is one row in a multi-item check result (e.g. per-binary status under
// the tools-and-packages check). Each item has its own severity so a rollup
// check can mix [ok] / [warn] / [fail] rows under one title.
type Item struct {
	Sev  Severity
	Name string
	Note string
}

// Check pairs a check's title and description with the probe that runs it.
type Check struct {
	Name string
	Desc string
	Fn   func(context.Context) Result
}

func (s Severity) String() string {
	switch s {
	case Pass:
		return "ok"
	case Warn:
		return "warn"
	case Fail:
		return "fail"
	default:
		// unreachable with the current three-value iota; guards future additions
		return "unknown"
	}
}

// Checks returns the full preflight check list. cfgFile is loaded lazily and
// at most once across the pull-secret and bin-dir checks.
func Checks(cfgFile string) []Check {
	loadedCfg := sync.OnceValues(func() (*config.Config, error) {
		return config.NewLoader().LoadFile(cfgFile)
	})
	effectiveBinDir := sync.OnceValue(func() binDirResolution {
		loaded, err := loadedCfg()
		if err != nil {
			return binDirResolution{Dir: config.ResolveBinDir(nil), LoadFailed: true}
		}
		return binDirResolution{Dir: config.ResolveBinDir(loaded)}
	})

	return []Check{
		{"host os", "platform and operator-mode detection", checkHostOS},
		{"root check", "guard against running as root (deploy uses sudo internally)", checkNotRoot},
		{"bin dir on path", "effective bin dir present on $PATH", func(_ context.Context) Result {
			return checkPath(effectiveBinDir())
		}},
		{"bin dir", "configured bin dir is writable by the invoking user", func(_ context.Context) Result {
			return checkBinDir(effectiveBinDir())
		}},
		{"tools and packages", "host tools, installable clis, and system packages", checkBinaries},
		{"sudo", "non-interactive (nopasswd) for long-running installs", checkSudo},
		{"ssh public key", "default key for vm provisioning", checkSSHKey},
		{"pull secret", "valid okd registry pull secret", func(_ context.Context) Result {
			return checkPullSecret(cfgFile)
		}},
		{"disk space", "at least 20 gb free in $home for install artifacts", checkDiskSpace},
		{"host ports", "53, 80, 443, 6443, 22623, 8080 available for bind", checkPorts},
	}
}

// checkHostOS identifies the host OS by parsing /etc/os-release.
func checkHostOS(_ context.Context) Result {
	host, err := platform.Detect()
	if err != nil {
		return Result{Sev: Fail, Detail: fmt.Sprintf("cannot read /etc/os-release: %v", err)}
	}
	return Result{Sev: Pass, Detail: fmt.Sprintf("%s %s (%s family)", host.ID, host.Version, host.Family)}
}

// checkNotRoot is a secondary guard — main.preflight() already refuses to
// run as root, so by the time doctor runs we know we are not root. We keep
// the check so it shows up green in the output for user confidence.
func checkNotRoot(_ context.Context) Result {
	if os.Geteuid() == 0 {
		return Result{Sev: Fail, Detail: "running as root; okdctl uses sudo internally"}
	}
	return Result{Sev: Pass, Detail: "running as unprivileged user"}
}

// binDirResolution pairs the resolved bin dir with a flag set when the
// config file failed to load. The flag demotes pass→warn and suffixes the
// detail so a malformed YAML never reads as green.
type binDirResolution struct {
	Dir        string
	LoadFailed bool
}

func (r binDirResolution) suffix(s string) string {
	if r.LoadFailed {
		return s + " (config unavailable; using default)"
	}
	return s
}

func (r binDirResolution) demote(sev Severity) Severity {
	if r.LoadFailed && sev == Pass {
		return Warn
	}
	return sev
}

func checkPath(r binDirResolution) Result {
	if slices.Contains(filepath.SplitList(os.Getenv("PATH")), r.Dir) {
		return Result{Sev: r.demote(Pass), Detail: r.suffix(r.Dir + " found on $PATH")}
	}
	if r.Dir == config.PreflightBinDir() {
		return Result{Sev: Warn, Detail: r.suffix(r.Dir + " missing from $PATH; okdctl will prepend it at startup")}
	}
	return Result{Sev: Fail, Detail: r.suffix(r.Dir + " missing from $PATH; add it to your shell profile (okdctl cannot auto-prepend a config-only dir)")}
}

// checkBinDir probes the effective bin dir for existence and user-write
// access. User-configured dirs that are not user-writable are a fail because
// setup runs under sudo and would install root-owned binaries.
func checkBinDir(r binDirResolution) Result {
	defaultDir := r.Dir == config.DefaultBinDir
	if _, err := os.Stat(r.Dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if defaultDir {
				return Result{Sev: Warn, Detail: r.suffix(r.Dir + " does not exist; setup will create it as root via sudo")}
			}
			return Result{Sev: Fail, Detail: r.suffix(r.Dir + " does not exist; create it first (e.g. mkdir -p)")}
		}
		return Result{Sev: Fail, Detail: r.suffix(r.Dir + " stat failed: " + err.Error())}
	}
	if !system.IsDirWritable(r.Dir) {
		if defaultDir {
			return Result{Sev: Warn, Detail: r.suffix(r.Dir + " not writable by invoking user; setup will install as root via sudo")}
		}
		return Result{Sev: Fail, Detail: r.suffix(r.Dir + " not writable by invoking user; setup runs under sudo so binaries will be root-owned — chown to your user if you want to manage them later")}
	}
	return Result{Sev: r.demote(Pass), Detail: r.suffix(r.Dir + " writable")}
}

// checkBinaries reports per-item status for three categories: host tools
// that must already exist (missing = fail), installable CLIs that setup
// downloads into /usr/local/bin (missing = warn), and system packages
// that setup installs via dnf/apt (missing = warn). The system package
// list is a mirror of setup.installSystemPackages — keep in sync.
func checkBinaries(_ context.Context) Result {
	hostBinaries := []string{"curl", "ssh", "git"}
	installableTools := []string{"oc", "openshift-install", "terraform"}
	systemPackages := []string{"coreos-installer", "haproxy", "dnsmasq"}

	var items []Item
	worst := Pass

	probe := func(name string, missingSev Severity, note string) {
		if _, err := exec.LookPath(name); err != nil {
			items = append(items, Item{Sev: missingSev, Name: name, Note: note})
			worst = max(worst, missingSev)
			return
		}
		items = append(items, Item{Sev: Pass, Name: name})
	}

	for _, name := range hostBinaries {
		probe(name, Fail, "missing; required before anything else will work")
	}
	for _, name := range installableTools {
		probe(name, Warn, "will be downloaded during setup")
	}
	for _, name := range systemPackages {
		probe(name, Warn, "will be installed via package manager")
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
		items = append(items, Item{Sev: Pass, Name: "apache"})
	} else {
		items = append(items, Item{Sev: Warn, Name: "apache", Note: "will be installed via package manager"})
		worst = max(worst, Warn)
	}

	return Result{Sev: worst, Items: items}
}

// checkSudo verifies that sudo is present and can escalate without
// prompting. A failing check is a warning rather than a fail because the
// deploy re-exec gate can still succeed with an interactive password — but
// the user should know up front whether the sudo prompt will appear.
func checkSudo(ctx context.Context) Result {
	if _, err := exec.LookPath("sudo"); err != nil {
		return Result{Sev: Fail, Detail: "sudo not installed"}
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := system.HasPasswordlessSudo(cctx); err != nil {
		return Result{Sev: Warn, Detail: "sudo requires a password; deploy will prompt"}
	}
	return Result{Sev: Pass, Detail: "nopasswd enabled"}
}

func checkSSHKey(_ context.Context) Result {
	home, err := os.UserHomeDir()
	if err != nil {
		return Result{Sev: Warn, Detail: "cannot resolve home directory"}
	}
	candidates := []string{
		filepath.Join(home, ".ssh", "id_ed25519.pub"),
		filepath.Join(home, ".ssh", "id_rsa.pub"),
		filepath.Join(home, ".ssh", "id_ecdsa.pub"),
	}
	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return Result{Sev: Pass, Detail: p}
		}
	}
	return Result{Sev: Warn, Detail: "no default ssh public key found; you will need to specify one in the wizard"}
}

// checkPullSecret reads the effective config file and verifies the path
// at cfg.Files.PullSecret. If no config exists yet (normal pre-deploy
// state), warns and directs the user to the wizard. If the config
// points at a file that does not exist, is not valid JSON, or has an
// empty 'auths' map, fails.
func checkPullSecret(cfgFile string) Result {
	configPath := cfgFile
	if configPath == "" {
		configPath = "okdctl.yaml"
	}

	if _, err := os.Stat(configPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Result{
				Sev:    Warn,
				Detail: "no config yet at " + configPath + "; run 'okdctl deploy' to set the pull secret path in the wizard",
			}
		}
		return Result{Sev: Fail, Detail: fmt.Sprintf("cannot stat config: %v", err)}
	}

	loader := config.NewLoader()
	cfg, err := loader.LoadFile(configPath)
	if err != nil {
		return Result{Sev: Fail, Detail: fmt.Sprintf("cannot load config: %v", err)}
	}

	if cfg.Files.PullSecret == "" {
		return Result{Sev: Fail, Detail: "files.pull_secret not set in " + configPath + "; run 'okdctl deploy' to configure"}
	}

	path := system.ExpandPath(cfg.Files.PullSecret)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Result{Sev: Fail, Detail: "not found at " + path + " (download from https://console.redhat.com/openshift/install/pull-secret)"}
		}
		return Result{Sev: Fail, Detail: err.Error()}
	}

	var parsed struct {
		Auths map[string]json.RawMessage `json:"auths"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Result{Sev: Fail, Detail: fmt.Sprintf("invalid json: %v", err)}
	}
	if parsed.Auths == nil {
		return Result{Sev: Fail, Detail: "missing or malformed 'auths' field: not a valid okd pull secret"}
	}
	if len(parsed.Auths) == 0 {
		return Result{Sev: Fail, Detail: "'auths' is empty: pull secret has no registry entries"}
	}
	return Result{Sev: Pass, Detail: path}
}

// checkDiskSpace checks that the home directory has at least 20 GB free.
// The deploy process downloads OKD tools, builds custom ISOs, and holds
// terraform state, all of which live under ~/okd-install by default.
func checkDiskSpace(_ context.Context) Result {
	const minGB = 20

	u, err := user.Current()
	if err != nil {
		return Result{Sev: Warn, Detail: "cannot resolve user home"}
	}
	freeBytes, err := fsFreeBytes(u.HomeDir)
	if err != nil {
		return Result{Sev: Warn, Detail: err.Error()}
	}
	freeGB := freeBytes / (1024 * 1024 * 1024)
	if freeGB < minGB {
		return Result{Sev: Fail, Detail: fmt.Sprintf("%d gb free in %s (need at least %d gb)", freeGB, u.HomeDir, minGB)}
	}
	return Result{Sev: Pass, Detail: fmt.Sprintf("%d gb free in %s", freeGB, u.HomeDir)}
}

// checkPorts probes each port okdctl's deploy will bind by trying
// to connect to 127.0.0.1:<port>. Connect-probe beats bind-probe for the
// preflight use case: the real deploy binds happen via sudo (haproxy,
// dnsmasq, apache), so the relevant question is "is something already
// there?" — not "can this unprivileged user bind right now?". Catches
// the common case of services bound on 0.0.0.0 or 127.0.0.1; misses
// services bound only on a specific non-loopback address.
func checkPorts(ctx context.Context) Result {
	ports := []int{53, 80, 443, 6443, 22623, 8080}

	var busy []string
	for _, p := range ports {
		if isPortInUse(ctx, p) {
			busy = append(busy, strconv.Itoa(p))
		}
	}
	if len(busy) > 0 {
		return Result{Sev: Warn, Detail: "in use: " + strings.Join(busy, ", ") + " (stop the conflicting service before deploy)"}
	}
	return Result{Sev: Pass, Detail: "53, 80, 443, 6443, 22623, 8080 all free"}
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
