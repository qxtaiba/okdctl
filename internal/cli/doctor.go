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
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/platform"
)

// goosDarwin is the runtime.GOOS string for macOS, hoisted out of inline
// comparisons so the doctor and diag commands can share it.
const goosDarwin = "darwin"

// doctorCmd is the user-facing 'openshitctl doctor' command. It is separate
// from main.preflight() (which is a startup guardrail) — doctor runs a
// comprehensive environment audit and reports status for each check.
var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check that your environment is ready to deploy a cluster",
	Long: `Run preflight checks on the local environment before a deploy.

Each check is reported as PASS, WARN, or FAIL:

  PASS — the check passed, no action needed
  WARN — something is suboptimal or missing but can be handled during
         deploy (e.g., 'oc' will be auto-downloaded into /usr/local/bin)
  FAIL — this must be fixed before 'openshitctl deploy' will succeed

Exit code is 0 if there are no FAIL results (WARN is tolerated), 1
otherwise. Designed to be rerun until clean.`,
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

// Severity levels ordered from least to most alarming.
type severity int

const (
	sevPass severity = iota
	sevWarn
	sevFail
)

type checkResult struct {
	sev    severity
	detail string
}

type check struct {
	name string
	fn   func(context.Context) checkResult
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	checks := []check{
		{"host OS", checkHostOS},
		{"not running as root", checkNotRoot},
		{"PATH contains /usr/local/bin", checkPath},
		{"required binaries", checkBinaries},
		{"sudo (non-interactive)", checkSudo},
		{"SSH public key", checkSSHKey},
		{"pull secret", checkPullSecret},
		{"free disk space (workdir)", checkDiskSpace},
		{"host ports available", checkPorts},
	}

	var fails, warns int
	for _, c := range checks {
		r := c.fn(ctx)
		printResult(c.name, r)
		switch r.sev {
		case sevFail:
			fails++
		case sevWarn:
			warns++
		}
	}

	fmt.Println()
	switch {
	case fails > 0:
		tui.Error(fmt.Sprintf("doctor: %d failing check(s), %d warning(s) — fix before deploying", fails, warns))
		return fmt.Errorf("preflight checks failed")
	case warns > 0:
		tui.Warn(fmt.Sprintf("doctor: %d warning(s) — deploy may proceed but review the warnings above", warns))
	default:
		tui.Info("doctor: environment looks ready")
	}
	return nil
}

func printResult(name string, r checkResult) {
	var marker string
	switch r.sev {
	case sevPass:
		marker = tui.SuccessStyle.Render("✓ PASS")
	case sevWarn:
		marker = tui.WarningStyle.Render("⚠ WARN")
	case sevFail:
		marker = tui.ErrorStyle.Render("✗ FAIL")
	}
	line := fmt.Sprintf("  %s  %s", marker, name)
	if r.detail != "" {
		line += tui.MutedStyle.Render(" — " + r.detail)
	}
	fmt.Println(line)
}

// checkHostOS verifies we can identify the host OS. On macOS (operator mode)
// we do not parse /etc/os-release and instead report darwin directly.
func checkHostOS(_ context.Context) checkResult {
	if runtime.GOOS == goosDarwin {
		return checkResult{sev: sevPass, detail: "macOS (operator mode — deploying to a remote Proxmox host)"}
	}
	host, err := platform.Detect()
	if err != nil {
		return checkResult{sev: sevFail, detail: "cannot read /etc/os-release: " + err.Error()}
	}
	return checkResult{sev: sevPass, detail: fmt.Sprintf("%s %s (%s family)", host.ID, host.Version, host.Family)}
}

// checkNotRoot is a secondary guard — main.preflight() already refuses to
// run as root, so by the time doctor runs we know we are not root. We keep
// the check so it shows up green in the output for user confidence.
func checkNotRoot(_ context.Context) checkResult {
	if os.Geteuid() == 0 {
		return checkResult{sev: sevFail, detail: "running as root; openshitctl uses sudo internally"}
	}
	return checkResult{sev: sevPass}
}

func checkPath(_ context.Context) checkResult {
	path := os.Getenv("PATH")
	if !strings.Contains(path, "/usr/local/bin") {
		return checkResult{sev: sevWarn, detail: "/usr/local/bin missing from PATH; openshitctl will prepend it at startup"}
	}
	return checkResult{sev: sevPass}
}

// checkBinaries looks for the binaries openshitctl shells out to. Tools
// that setup auto-downloads into /usr/local/bin (oc, openshift-install,
// terraform) are warnings, not failures — they will be installed on first
// deploy if missing.
func checkBinaries(_ context.Context) checkResult {
	required := []string{"curl", "ssh", "git"}
	autoInstalled := []string{"oc", "openshift-install", "terraform"}

	var missing []string
	for _, name := range required {
		if _, err := exec.LookPath(name); err != nil {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return checkResult{sev: sevFail, detail: "missing required: " + strings.Join(missing, ", ")}
	}

	var missingAuto []string
	for _, name := range autoInstalled {
		if _, err := exec.LookPath(name); err != nil {
			missingAuto = append(missingAuto, name)
		}
	}
	if len(missingAuto) > 0 {
		return checkResult{sev: sevWarn, detail: "will be auto-downloaded during setup: " + strings.Join(missingAuto, ", ")}
	}
	return checkResult{sev: sevPass, detail: "curl, ssh, git, oc, openshift-install, terraform all present"}
}

// checkSudo verifies that sudo can escalate without prompting. A failing
// check is a warning rather than a fail because interactive sudo can still
// work if the user is present during deploy — but it is frustrating for a
// long-running bootstrap to block on a password prompt halfway through.
func checkSudo(ctx context.Context) checkResult {
	if runtime.GOOS == goosDarwin {
		return checkResult{sev: sevPass, detail: "skipped on macOS (operator mode)"}
	}
	if _, err := exec.LookPath("sudo"); err != nil {
		return checkResult{sev: sevFail, detail: "sudo not installed"}
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "sudo", "-n", "true")
	if err := cmd.Run(); err != nil {
		return checkResult{sev: sevWarn, detail: "sudo requires a password; deploy will prompt"}
	}
	return checkResult{sev: sevPass, detail: "NOPASSWD enabled"}
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
	return checkResult{sev: sevWarn, detail: "no default SSH public key found; you will need to specify one in the wizard"}
}

// checkPullSecret looks for a pull secret in the default location. The
// actual deploy-time check is more thorough (validates JSON structure,
// checks for required auth entries); here we just verify presence and
// basic JSON well-formedness.
func checkPullSecret(_ context.Context) checkResult {
	home, err := os.UserHomeDir()
	if err != nil {
		return checkResult{sev: sevWarn, detail: "cannot resolve home directory"}
	}
	path := filepath.Join(home, ".openshitctl", "pull-secret.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return checkResult{sev: sevWarn, detail: "not found at " + path + " (required for deploy; see README)"}
		}
		return checkResult{sev: sevFail, detail: err.Error()}
	}

	var js map[string]any
	if err := json.Unmarshal(data, &js); err != nil {
		return checkResult{sev: sevFail, detail: "invalid JSON: " + err.Error()}
	}
	if _, ok := js["auths"]; !ok {
		return checkResult{sev: sevFail, detail: "missing 'auths' field — not a valid OKD pull secret"}
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
		return checkResult{sev: sevWarn, detail: "statfs failed: " + err.Error()}
	}
	// uint64 cast on Bavail is redundant on darwin (already uint64) but
	// required on linux/amd64 to make the multiplication compile against
	// the signed Bsize type. Both casts kept for portability.
	freeBytes := uint64(st.Bavail) * uint64(st.Bsize) //nolint:unconvert // platform-portability
	freeGB := freeBytes / (1024 * 1024 * 1024)
	if freeGB < minGB {
		return checkResult{sev: sevFail, detail: fmt.Sprintf("%d GB free in %s (need at least %d GB)", freeGB, u.HomeDir, minGB)}
	}
	return checkResult{sev: sevPass, detail: fmt.Sprintf("%d GB free in %s", freeGB, u.HomeDir)}
}

// checkPorts probes the host ports that openshitctl binds during deploy
// (dnsmasq, HAProxy frontends, ignition server). A bound port is a warning
// rather than a fail because the user may intentionally use --skip-* flags
// to delegate one of these services to something else.
func checkPorts(ctx context.Context) checkResult {
	tcpPorts := []int{53, 80, 443, 6443, 22623, 8080}
	udpPorts := []int{53}

	var lc net.ListenConfig
	var busy []string
	for _, p := range tcpPorts {
		if !tryListenTCP(ctx, &lc, p) {
			busy = append(busy, fmt.Sprintf("tcp/%d", p))
		}
	}
	for _, p := range udpPorts {
		if !tryListenUDP(ctx, &lc, p) {
			busy = append(busy, fmt.Sprintf("udp/%d", p))
		}
	}
	if len(busy) > 0 {
		return checkResult{sev: sevWarn, detail: "in use: " + strings.Join(busy, ", ") + " — use --skip-* flags if intentional"}
	}
	return checkResult{sev: sevPass, detail: "53, 80, 443, 6443, 22623, 8080 all available"}
}

func tryListenTCP(ctx context.Context, lc *net.ListenConfig, port int) bool {
	l, err := lc.Listen(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

func tryListenUDP(ctx context.Context, lc *net.ListenConfig, port int) bool {
	c, err := lc.ListenPacket(ctx, "udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}
