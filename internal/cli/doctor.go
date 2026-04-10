//go:build linux || darwin

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

const goosDarwin = "darwin"

// doctorCmd is the user-facing 'openshitctl doctor' command. It is separate
// from main.preflight() (which is a startup guardrail) — doctor runs a
// comprehensive environment audit and reports status for each check.
var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check that your environment is ready to deploy a cluster",
	Long: `Run preflight checks on the local environment before a deploy.

Each check is reported as [ok], [warn], or [fail]:

  [ok]   — the check passed, no action needed
  [warn] — something is suboptimal or missing but can be handled
           during deploy (e.g., 'oc' will be auto-downloaded into
           /usr/local/bin)
  [fail] — this must be fixed before 'openshitctl deploy' will
           succeed

Exit code is 0 if there are no [fail] results ([warn] is tolerated),
1 otherwise. Designed to be rerun until clean.`,
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
		{"host os", checkHostOS},
		{"not running as root", checkNotRoot},
		{"path contains /usr/local/bin", checkPath},
		{"required binaries", checkBinaries},
		{"sudo (non-interactive)", checkSudo},
		{"ssh public key", checkSSHKey},
		{"pull secret", checkPullSecret},
		{"free disk space (workdir)", checkDiskSpace},
		{"host ports", checkPorts},
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

// labelWidth is the column width reserved for the severity label, sized
// for the longest label ("[fail]" / "[warn]" = 6 chars). Keeps check
// names aligned in a scannable column.
const labelWidth = 6

func printResult(name string, r checkResult) {
	var rawLabel, styled string
	switch r.sev {
	case sevPass:
		rawLabel = "[ok]"
		styled = tui.SuccessStyle.Render(rawLabel)
	case sevWarn:
		rawLabel = "[warn]"
		styled = tui.WarningStyle.Render(rawLabel)
	case sevFail:
		rawLabel = "[fail]"
		styled = tui.ErrorStyle.Render(rawLabel)
	}
	padding := strings.Repeat(" ", labelWidth-len(rawLabel)+2)
	line := "  " + styled + padding + name
	if r.detail != "" {
		line += tui.MutedStyle.Render(" — " + r.detail)
	}
	fmt.Println(line)
}

// checkHostOS verifies we can identify the host OS. On macOS (operator mode)
// we do not parse /etc/os-release and instead report darwin directly.
func checkHostOS(_ context.Context) checkResult {
	if runtime.GOOS == goosDarwin {
		return checkResult{sev: sevPass, detail: "macos (operator mode — deploying to a remote proxmox host)"}
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
		return checkResult{sev: sevWarn, detail: "/usr/local/bin missing from path; openshitctl will prepend it at startup"}
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
		return checkResult{sev: sevPass, detail: "skipped on macos (operator mode)"}
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
			return checkResult{sev: sevWarn, detail: "not found at " + path + " (required for deploy; see readme)"}
		}
		return checkResult{sev: sevFail, detail: err.Error()}
	}

	var js map[string]any
	if err := json.Unmarshal(data, &js); err != nil {
		return checkResult{sev: sevFail, detail: "invalid json: " + err.Error()}
	}
	auths, ok := js["auths"].(map[string]any)
	if !ok {
		return checkResult{sev: sevFail, detail: "missing or malformed 'auths' field — not a valid okd pull secret"}
	}
	if len(auths) == 0 {
		return checkResult{sev: sevFail, detail: "'auths' is empty — pull secret has no registry entries"}
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
		return checkResult{sev: sevFail, detail: fmt.Sprintf("%d gb free in %s (need at least %d gb)", freeGB, u.HomeDir, minGB)}
	}
	return checkResult{sev: sevPass, detail: fmt.Sprintf("%d gb free in %s", freeGB, u.HomeDir)}
}

// checkPorts probes each port openshitctl's deploy will bind by trying
// to connect to 127.0.0.1:<port>. Connect-probe beats bind-probe for the
// preflight use case: the real deploy binds happen via sudo (haproxy,
// dnsmasq, apache), so the relevant question is "is something already
// there?" — not "can this unprivileged user bind right now?". Catches
// the common case of services bound on 0.0.0.0 or 127.0.0.1; misses
// services bound only on a specific non-loopback address.
func checkPorts(ctx context.Context) checkResult {
	if runtime.GOOS == goosDarwin {
		return checkResult{sev: sevPass, detail: "skipped on macos (operator mode)"}
	}

	ports := []int{53, 80, 443, 6443, 22623, 8080}

	var busy []string
	for _, p := range ports {
		if isPortInUse(ctx, p) {
			busy = append(busy, fmt.Sprintf("%d", p))
		}
	}
	if len(busy) > 0 {
		return checkResult{sev: sevWarn, detail: "in use: " + strings.Join(busy, ", ") + " — use --skip-* flags if intentional"}
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
