// Package doctor implements okdctl's host-prerequisite checks: OS detection,
// tool/package presence, sudo mode, pull-secret validity, disk space, and ports.
package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/platform"
	"github.com/qxtaiba/okdctl/internal/system"
)

// Severity levels ordered from least to most alarming.
type Severity int

// Pass, Warn, and Fail order check outcomes; multi-item rollups keep the worst severity.
const (
	Pass Severity = iota
	Warn
	Fail
)

// Result is a check's outcome; a non-empty Items list renders as a per-item
// sub-list (Detail ignored), else Detail renders as the single-line result.
type Result struct {
	Sev    Severity
	Detail string
	Items  []Item
}

// Item is one row in a multi-item check result, each with its own severity so a
// rollup can mix [ok]/[warn]/[fail] rows.
type Item struct {
	Sev  Severity
	Name string
	Note string
}

// Check pairs a title and description with the probe that runs it.
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
		return "unknown"
	}
}

// Checks returns the full preflight check list; cfgFile loads lazily, once,
// shared by the pull-secret and bin-dir checks.
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
		{"host ports", "53, 80, 443, 6443, 22623 available for bind", checkPorts},
	}
}

func checkHostOS(_ context.Context) Result {
	host, err := platform.Detect()
	if err != nil {
		return Result{Sev: Fail, Detail: fmt.Sprintf("cannot read /etc/os-release: %v", err)}
	}
	return Result{Sev: Pass, Detail: fmt.Sprintf("%s %s (%s family)", host.ID, host.Version, host.Family)}
}

// checkNotRoot is a secondary guard: cli.ensureRoot rejects `sudo okdctl
// doctor`, but OKDCTL_WIZARD_DEMO bypasses that.
func checkNotRoot(_ context.Context) Result {
	if os.Geteuid() == 0 {
		return Result{Sev: Fail, Detail: "running as root; okdctl uses sudo internally"}
	}
	return Result{Sev: Pass, Detail: "running as unprivileged user"}
}

// binDirResolution pairs the resolved dir with a load-failed flag so malformed
// YAML demotes pass→warn instead of reading green.
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

// checkBinDir probes the bin dir for existence/writability; an unwritable
// user-configured dir fails since setup would install root-owned binaries there.
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

// checkBinaries reports per-item status: host tools missing = fail; CLIs and
// system packages missing = warn. Mirrors setup.installSystemPackages — keep in sync.
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

	// Apache binary name varies by distro: httpd (rhel) or apache2 (debian).
	apacheFound := slices.ContainsFunc([]string{"httpd", "apache2"}, func(bin string) bool {
		_, err := exec.LookPath(bin)
		return err == nil
	})
	if apacheFound {
		items = append(items, Item{Sev: Pass, Name: "apache"})
	} else {
		items = append(items, Item{Sev: Warn, Name: "apache", Note: "will be installed via package manager"})
		worst = max(worst, Warn)
	}

	return Result{Sev: worst, Items: items}
}

// checkSudo verifies passwordless sudo; failing here only warns since deploy's
// re-exec gate still works interactively.
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

// checkPullSecret verifies cfg.Files.PullSecret: missing config warns (normal
// pre-deploy); missing file/invalid JSON/empty 'auths' fails.
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

	data, err := readNoFollow(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Result{Sev: Fail, Detail: "not found at " + path + " (download from https://console.redhat.com/openshift/install/pull-secret)"}
		}
		return Result{Sev: Fail, Detail: err.Error()}
	}
	defer system.ZeroBytes(data)

	var parsed struct {
		Auths map[string]json.RawMessage `json:"auths"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Result{Sev: Fail, Detail: fmt.Sprintf("invalid json: %v", err)}
	}
	defer func() {
		for k := range parsed.Auths {
			system.ZeroBytes(parsed.Auths[k])
		}
	}()
	if parsed.Auths == nil {
		return Result{Sev: Fail, Detail: "missing or malformed 'auths' field: not a valid okd pull secret"}
	}
	if len(parsed.Auths) == 0 {
		return Result{Sev: Fail, Detail: "'auths' is empty: pull secret has no registry entries"}
	}
	return Result{Sev: Pass, Detail: path}
}

// readNoFollow refuses to follow a symlink at the final path component,
// matching setup's ignition renderer.
func readNoFollow(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("path %q is a symlink; refusing to follow", path)
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// checkDiskSpace requires 20 GB free in home for OKD tools, ISOs, and terraform state.
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

// checkPorts connects to 127.0.0.1:<port> rather than binding (binds happen
// via sudo later); misses services bound only on a non-loopback address.
func checkPorts(ctx context.Context) Result {
	ports := []int{53, 80, 443, 6443, 22623}

	var busy []string
	for _, p := range ports {
		if isPortInUse(ctx, p) {
			busy = append(busy, strconv.Itoa(p))
		}
	}
	if len(busy) > 0 {
		return Result{Sev: Warn, Detail: "in use: " + strings.Join(busy, ", ") + " (stop the conflicting service before deploy)"}
	}
	return Result{Sev: Pass, Detail: "53, 80, 443, 6443, 22623 all free"}
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
