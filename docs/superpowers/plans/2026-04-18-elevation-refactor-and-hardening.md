# Elevation refactor & hardening — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the per-op `try-unprivileged → fall back to sudo subprocess` escalation pattern with a require-root-via-re-exec model at CLI entry, then use plain stdlib calls in the single-UID body. Fold in three `system/fs.go` correctness fixes (mode-race, credentialed backups, write-path Close error swallowing) that share the edit surface. Then land nine independent follow-up cleanups (type strengthening, modern-Go stdlib migration, ctx threading, security perms, subprocess polish, dep hygiene, and an optional `httputil` trim) as separate reviewable PRs.

**Architecture:**
- Root-required subcommands (`deploy`, `destroy`, `cleanup`, `update-ingress`) self-re-exec via `syscall.Exec("sudo", …)` in a `PersistentPreRunE` gate, using sudo's default env reset (no `-E`).
- Inside the root body, filesystem and subprocess operations use plain stdlib (`os.Chmod`, `os.Chown`, `os.MkdirAll`, `os.RemoveAll`, `exec.CommandContext`). No more try-then-fallback.
- Three user-home write sites chown back to the invoking user (resolved via `SUDO_UID`/`SUDO_GID`/`SUDO_USER`) via an explicit `system.WriteAsInvokingUser` / `system.ChownToInvokingUser` helper.
- At end of `deploy`, a deferred `system.ChownTreeToInvokingUser(workDir)` restores ownership of per-deploy artifacts.
- Follow-up PRs are strictly additive or local refactors; none depend on the elevation work except the security fold-ins in Phase 1.

**Tech stack:** Go 1.25, cobra, `os`, `os/exec`, `os/user`, `syscall`, `path/filepath`, `k8s.io/api/core/v1` (for Phase 4 node readiness JSON), `net/netip` (for Phase 6).

**Repo conventions (binding):**
- Commit format: `type(scope): description` — lowercase, imperative, ≤70 chars. Types: `refactor`, `fix`, `feat`, `chore`, `ci`, `docs`, `test`.
- **No commit trailers.** No `Co-Authored-By`, no AI references in bodies.
- Comment density target ~3%. Don't narrate refactors in code — commit messages carry that context.
- `.golangci.yml` is authoritative; `funlen.lines: 120`, `gocyclo.min-complexity: 30`, `dupl.threshold: 200`.
- CI gates: `lint-go`, `test-go`, `build-go`, `security` (govulncheck), `lint-yaml`, `validate-terraform`. All green before merge.
- Target branch: `develop`.

**Testing posture:** The repo has zero `*_test.go` files. We don't introduce a test pyramid in this plan. Every phase is verified via `go build ./...` + `go vet ./...` + `golangci-lint run` + targeted manual smoke (wizard unprivileged, deploy on a disposable target where possible). Behavior preservation for subprocess and filesystem call sites is verified by code inspection against the stdlib equivalents listed below.

---

## File structure map

| File | Phase | Disposition |
|------|-------|-------------|
| `internal/cli/elevation.go` | 1 | **Create** — re-exec gate + subcommand classification |
| `internal/system/elevation.go` | 1 | **Create** — InvokingUser helpers + chown-back + migrated HasPasswordlessSudo |
| `internal/system/permissions.go` | 1 | **Delete** (36 LOC) — absorbed into re-exec model |
| `internal/system/fileops_elevated.go` | 1 | **Delete** (148 LOC) — absorbed into re-exec model |
| `internal/system/fs.go` | 1 | **Modify** — SEC-001 (CopyFile atomic mode), ERR-003 (Close error surfacing) |
| `internal/system/exec.go` | 1 | **Modify** — drop `HasPasswordlessSudo` move note; keep WaitFor/WaitForWithTimeout |
| `internal/system/systemd.go` | 1 | **Modify** — drop `RunSudo` wrapping; use plain `exec.CommandContext` |
| `internal/cli/root.go` | 1 | **Modify** — wire `ensureRoot` into `PersistentPreRunE` |
| `internal/cli/deploy.go` | 1 | **Modify** — deferred workdir chown hook in `runFullDeployment` |
| `internal/cli/destroy.go` | 1 | **Modify** — deferred workdir chown hook (if writes occur) |
| `internal/cli/doctor.go` | 1, 8 | **Modify** — Phase 1: delegate sudo check to system pkg; Phase 8: ERR-002 + CTX-001 doc |
| `internal/cli/update_ingress.go` | 1 | **Modify** — no workdir touch but needs gate classification only |
| `internal/distribution/okd/install/flux.go` | 1 | **Modify** — Category-1 chown-back (kubeconfig + .bashrc); SEC-002 CopyFileMode for backup |
| `internal/distribution/okd/releases/cache.go` | 1 | **Modify** — Category-1 chown-back on cache write |
| `internal/distribution/okd/setup/ignition.go` | 1 | **Modify** — SEC-002: install-config.yaml backup uses CopyFileMode(…,0o600) |
| `internal/distribution/okd/setup/phase.go` | 1, 3 | **Modify** — Phase 1: drop `HasPasswordlessSudo` preflight (moot after re-exec); Phase 3: NodeInfo.Role → NodeRole |
| `internal/distribution/okd/setup/apache.go` | 1 | **Modify** — collapse 6 `system.X` elevation helpers to stdlib |
| `internal/distribution/okd/setup/artifacts.go` | 1 | **Modify** — collapse 2 elevation calls |
| `internal/distribution/okd/setup/haproxy.go` | 1 | **Modify** — collapse 4 elevation calls |
| `internal/distribution/okd/setup/tools.go` | 1, 7, 8 | **Modify** — Phase 1: collapse 6 elevation calls; Phase 7: thread ctx into getToolVersion; Phase 8: SUB-003 wget stderr |
| `internal/distribution/okd/setup/nodes.go` | 3 | **Modify** — NodeRole enum propagation |
| `internal/distribution/okd/setup/terraform.go` | 1, 2, 3 | **Modify** — Phase 1: collapse elevation; Phase 2: tfvars 0o600; Phase 3: NodeRole |
| `internal/distribution/okd/postinstall/haproxy.go` | 1 | **Modify** — collapse 1 elevation call |
| `internal/distribution/okd/dns/dns.go` | 1, 3 | **Modify** — Phase 1: collapse 3 elevation calls; Phase 3: role iteration |
| `internal/distribution/okd/dns/dnsmasq.go` | 1 | **Modify** — collapse 12 elevation calls + RunSudo calls |
| `internal/distribution/okd/firewall/firewall.go` | 1 | **Modify** — collapse 6 elevation calls |
| `internal/distribution/okd/cleanup/services.go` | 1 | **Modify** — collapse 3 elevation calls |
| `internal/distribution/okd/cleanup/packages.go` | 1 | **Modify** — collapse 1 elevation call |
| `internal/distribution/okd/cleanup/artifacts.go` | 1 | **Modify** — collapse 1 elevation call |
| `internal/platform/packages.go` | 1, 7 | **Modify** — Phase 1: collapse 7 elevation calls; Phase 7: thread ctx into IsInstalled |
| `internal/netutil/iface.go` | 1 | **Modify** — collapse 2 elevation calls |
| `internal/netutil/ip.go` | 6 | **Modify** — net.ParseIP/net.ParseCIDR → netip |
| `internal/config/validators.go` | 6 | **Modify** — netip migration for validators.go:383 |
| `internal/distribution/okd/postinstall/verify.go` | 4 | **Modify** — TYPE-002: oc get nodes -o json |
| `internal/distribution/okd/types.go` | 3 | **Modify** — add NodeRole type + constants |
| `internal/config/cluster.go` | 5 | **Modify** — TYPE-003 setup (if addon-local settings change) |
| `internal/addon/catalog/flux/flux.go` | 5 | **Modify** — introduce per-addon key consts |
| `internal/addon/catalog/secretstore/secretstore.go` | 5 | **Modify** — introduce per-addon key consts |
| `internal/distribution/okd/install/monitor.go` | 8 | **Modify** — CON-001 sync.Once around Kill |
| `go.mod` | 9 | **Modify** — drop direct `gopkg.in/yaml.v3` if all callers happy with `sigs.k8s.io/yaml` |
| `internal/httputil/httputil.go` | 10 | **Modify** (optional) — trim factory funcs; keep timeout constants |

---

## Phase ordering and PR strategy

Each phase is one PR against `develop`. Phases are written in dependency order — later phases assume earlier phases have merged.

| Phase | PR scope | Approx LOC | Risk | Dependencies |
|-------|---------|-----------|------|--------------|
| 1 | Elevation refactor + SEC-001/SEC-002/ERR-003 fold-ins | −126 | medium | none |
| 2 | SEC-003 perms hardening | 0 | low | 1 (SEC-002 already landed) |
| 3 | TYPE-001 NodeRole enum | +13 | low | 1 |
| 4 | TYPE-002 node readiness JSON | +15 | medium | 1 |
| 5 | TYPE-003 addon settings consts | +20 | low | none |
| 6 | MOD-001 netip cleanup | −8 | low | none |
| 7 | CTX-003 ctx threading | +6 | low | 1 |
| 8 | Doctor + polish bundle (ERR-002, CTX-001 doc, SUB-003, CON-001) | +10 | low | 1 |
| 9 | DEP-001 yaml.v3 direct drop | −1 line | low | none |
| 10 | **Optional** httputil trim | −28 | low | none |

---

# Phase 1 — Elevation refactor + fs.go correctness fixes

**Goal:** Delete the `try-unprivileged → sudo subprocess` escalation layer. Introduce a re-exec gate at CLI entry so root-required commands run single-UID. Collapse 55 call sites to plain stdlib. Preserve user-home and workdir ownership for the invoking user. Fix `CopyFile` mode race (SEC-001), credentialed backups (SEC-002), and Close-on-write-path error swallowing (ERR-003) while we're editing `system/fs.go`.

**Branch:** `phase1-elevation-refactor` off `develop`.

---

### Task 1.1: Create `system/elevation.go` with invoking-user helpers

**Files:**
- Create: `internal/system/elevation.go`

- [ ] **Step 1: Write `internal/system/elevation.go`**

```go
package system

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
)

// InvokingUser returns the user who invoked the command. When the process
// was re-exec'd under sudo (as okdctl deploy / destroy / cleanup /
// update-ingress do), SUDO_USER identifies the original user. Without it,
// the current user is returned. The caller uses this to chown user-home
// artifacts back after a privileged run.
func InvokingUser() (*user.User, error) {
	if name := os.Getenv("SUDO_USER"); name != "" {
		return user.Lookup(name)
	}
	return user.Current()
}

// InvokingUserHomeDir returns the home directory of the invoking user. Use
// this instead of os.UserHomeDir() at sites that write artifacts the user
// must read back (kubeconfig, releases cache, .bashrc). os.UserHomeDir()
// returns /root under sudo's default env reset, which would land files in
// the wrong place.
func InvokingUserHomeDir() (string, error) {
	u, err := InvokingUser()
	if err != nil {
		return "", err
	}
	return u.HomeDir, nil
}

// invokingUserIDs returns the SUDO_UID / SUDO_GID pair. Returns nil if the
// process was not re-exec'd under sudo, meaning chown is unnecessary.
type sudoIDs struct {
	uid, gid int
}

func invokingUserIDs() (*sudoIDs, error) {
	uidStr := os.Getenv("SUDO_UID")
	gidStr := os.Getenv("SUDO_GID")
	if uidStr == "" || gidStr == "" {
		return nil, nil
	}
	uid, err := strconv.Atoi(uidStr)
	if err != nil {
		return nil, fmt.Errorf("invalid SUDO_UID %q: %w", uidStr, err)
	}
	gid, err := strconv.Atoi(gidStr)
	if err != nil {
		return nil, fmt.Errorf("invalid SUDO_GID %q: %w", gidStr, err)
	}
	return &sudoIDs{uid: uid, gid: gid}, nil
}

// ChownToInvokingUser chowns path to SUDO_UID:SUDO_GID. Silently no-ops when
// the process was not re-exec'd under sudo — in that case the caller is
// already the invoking user and the file is already owned correctly.
func ChownToInvokingUser(path string) error {
	ids, err := invokingUserIDs()
	if err != nil || ids == nil {
		return err
	}
	return os.Chown(path, ids.uid, ids.gid)
}

// WriteAsInvokingUser atomically writes data to path with mode, then chowns
// the file to the invoking user (if under sudo). Use this for artifacts
// under the user's home that must remain user-readable after deploy.
func WriteAsInvokingUser(path string, data []byte, mode os.FileMode) error {
	if err := AtomicWrite(path, data, mode); err != nil {
		return err
	}
	return ChownToInvokingUser(path)
}

// ChownTreeToInvokingUser recursively chowns root and all descendants to the
// invoking user. No-op if the process was not re-exec'd under sudo. Errors
// on individual entries are collected; the walk does not abort.
func ChownTreeToInvokingUser(root string) error {
	ids, err := invokingUserIDs()
	if err != nil || ids == nil {
		return err
	}
	var errs []error
	walkErr := filepath.WalkDir(root, func(path string, _ os.DirEntry, walkErr error) error {
		if walkErr != nil {
			errs = append(errs, walkErr)
			return nil
		}
		if chownErr := os.Lchown(path, ids.uid, ids.gid); chownErr != nil {
			errs = append(errs, fmt.Errorf("chown %s: %w", path, chownErr))
		}
		return nil
	})
	if walkErr != nil {
		errs = append(errs, walkErr)
	}
	return errors.Join(errs...)
}

// HasPasswordlessSudo returns nil if `sudo -n true` succeeds. Callers use
// this as an advisory pre-flight to warn the user that the next sudo
// invocation may prompt for a password. Under the re-exec model this is
// only called by doctor; operational paths never call it.
func HasPasswordlessSudo(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "sudo", "-n", "true")
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./internal/system/...`
Expected: clean compile.

- [ ] **Step 3: Commit**

```bash
git add internal/system/elevation.go
git commit -m "feat(system): add invoking-user helpers for sudo re-exec model"
```

---

### Task 1.2: Create `cli/elevation.go` re-exec gate

**Files:**
- Create: `internal/cli/elevation.go`

- [ ] **Step 1: Write `internal/cli/elevation.go`**

```go
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"
)

// rootRequiredCmds lists subcommands that perform privileged operations
// (writing to /etc, /usr/local/bin, /var/www/html, managing systemd units,
// configuring firewalls). When invoked without euid=0, the CLI re-execs
// itself under sudo before cobra's RunE fires.
var rootRequiredCmds = map[string]bool{
	"deploy":         true,
	"destroy":        true,
	"cleanup":        true,
	"update-ingress": true,
}

// ensureRoot is wired into the root cobra command's PersistentPreRunE. It
// is a no-op for unprivileged commands (wizard, doctor, --help, --version)
// and for invocations that already have euid=0. For root-required commands
// invoked as a non-root user, it re-execs the same binary under sudo with
// the same args and environment. syscall.Exec replaces the current process,
// so a successful re-exec never returns. The euid=0 check prevents re-exec
// loops.
func ensureRoot(cmd *cobra.Command) error {
	if !rootRequiredCmds[cmd.Name()] {
		return nil
	}
	if os.Geteuid() == 0 {
		return nil
	}
	sudoPath, err := exec.LookPath("sudo")
	if err != nil {
		return fmt.Errorf("%s requires root and sudo is not installed; run as root", cmd.Name())
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve own binary: %w", err)
	}
	args := append([]string{"sudo", "--", self}, os.Args[1:]...)
	return syscall.Exec(sudoPath, args, os.Environ())
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./internal/cli/...`
Expected: clean compile.

- [ ] **Step 3: Commit**

```bash
git add internal/cli/elevation.go
git commit -m "feat(cli): add sudo re-exec gate for root-required commands"
```

---

### Task 1.4: Wire `ensureRoot` into root command

**Files:**
- Modify: `internal/cli/root.go`

- [ ] **Step 1: Add `PersistentPreRunE` to `rootCmd`**

Replace the `rootCmd` declaration in `internal/cli/root.go:24-50` so it includes a `PersistentPreRunE`:

```go
var rootCmd = &cobra.Command{
	Use:   "okdctl",
	Short: "Deploy production-ready Kubernetes clusters",
	Long: `Homelab K8s - Deploy production-ready Kubernetes clusters

A delightful CLI tool for deploying OKD/OpenShift clusters
on Proxmox VE infrastructure.

Highlights:
  • Interactive setup wizard with beautiful TUI
  • OKD/OpenShift 4.15-4.21 support
  • Addon-extensible architecture (Flux, secrets, storage, cert-manager)
  • YAML configuration with sensible defaults
  • Automated preflight checks and validation
  • Single binary distribution`,
	Version:           version.Version,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error { return ensureRoot(cmd) },
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println(tui.TitleStyle.Render("homelab k8s"))
		fmt.Println()
		fmt.Println(tui.MutedStyle.Render("quick start:"))
		fmt.Println("  " + tui.HighlightStyle.Render("okdctl deploy") + "           deploy a cluster")
		fmt.Println("  " + tui.HighlightStyle.Render("okdctl destroy") + "          destroy the cluster")
		fmt.Println("  " + tui.HighlightStyle.Render("okdctl update-ingress") + "   switch ingress to loadbalancer ips")
		fmt.Println()
		fmt.Println(tui.MutedStyle.Render("run 'okdctl --help' for all commands"))
	},
}
```

- [ ] **Step 2: Verify `okdctl --help` still works (gate is a no-op for help)**

Run: `go run ./cmd/okdctl --help`
Expected: root help output, no sudo prompt.

- [ ] **Step 3: Verify `okdctl deploy` triggers re-exec (as non-root)**

Run: `go run ./cmd/okdctl deploy --config /tmp/nope.yaml --non-interactive`
Expected: `sudo` prompts for password. (Cancel with Ctrl-C; we're just confirming the gate fires.)

- [ ] **Step 4: Commit**

```bash
git add internal/cli/root.go
git commit -m "refactor(cli): wire ensureRoot gate into root PersistentPreRunE"
```

---

### Task 1.5: SEC-001 + ERR-003 fix — make `CopyFile` atomic-mode and surface Close errors

**Files:**
- Modify: `internal/system/fs.go`

- [ ] **Step 1: Rewrite `CopyFile` to delegate to `CopyFileMode` with source mode**

Replace `CopyFile` at `internal/system/fs.go:64-108` with:

```go
// CopyFile copies src to dst, preserving the source file's permission bits.
// The destination is opened with the correct mode at creation time, so
// there is no window where dst is world-readable under the umask default.
// For credential-bearing files (kubeconfig, install-config.yaml, private
// keys), prefer CopyFileMode with an explicit 0o600.
func CopyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to stat source file: %w", err)
	}
	return CopyFileMode(src, dst, info.Mode().Perm())
}
```

- [ ] **Step 2: Fix `CopyFileMode` Close error surfacing (ERR-003)**

Replace `CopyFileMode` at `internal/system/fs.go:115-155` with:

```go
// CopyFileMode copies src to dst, creating dst with the given mode applied
// at open time (before any bytes are written). This avoids the race window
// where a file created with a permissive umask is briefly world-readable
// before a follow-up chmod narrows it. Use this for anything sensitive —
// kubeconfigs, credential files, private keys.
//
// Close errors on the destination are surfaced: a failing Close can mask
// an unflushed buffer or fsync problem, and silently discarding it would
// lose a durability signal.
func CopyFileMode(src, dst string, mode os.FileMode) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer func() { _ = sourceFile.Close() }()

	if err := EnsureDirForFile(dst); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	destFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}

	closed := false
	success := false
	defer func() {
		if !closed {
			_ = destFile.Close()
		}
		if !success {
			_ = os.Remove(dst)
		}
	}()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	if err := destFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync destination file: %w", err)
	}

	if err := destFile.Close(); err != nil {
		return fmt.Errorf("failed to close destination file: %w", err)
	}
	closed = true

	// Tighten explicitly in case dst pre-existed with different permissions
	// — O_CREATE won't change them.
	if err := os.Chmod(dst, mode); err != nil {
		return fmt.Errorf("failed to set file permissions: %w", err)
	}

	success = true
	return nil
}
```

- [ ] **Step 3: Verify build and existing callers still work**

Run: `go build ./...`
Expected: clean compile.

- [ ] **Step 4: Commit**

```bash
git add internal/system/fs.go
git commit -m "fix(system): eliminate CopyFile mode-narrow race and surface Close errors"
```

---

### Task 1.6: SEC-002 fix — credentialed backups use `CopyFileMode` with explicit 0o600

**Files:**
- Modify: `internal/distribution/okd/install/flux.go`
- Modify: `internal/distribution/okd/setup/ignition.go`

- [ ] **Step 1: Update kubeconfig backup at `install/flux.go:61`**

Find the line:
```go
if err := system.CopyFile(destKubeconfig, backupPath); err != nil {
```

Replace with:
```go
if err := system.CopyFileMode(destKubeconfig, backupPath, 0o600); err != nil {
```

- [ ] **Step 2: Update install-config.yaml backup at `setup/ignition.go:74`**

Find:
```go
if err := system.CopyFile(outputPath, backupPath); err != nil {
```

Replace with:
```go
if err := system.CopyFileMode(outputPath, backupPath, 0o600); err != nil {
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: clean compile.

- [ ] **Step 4: Commit**

```bash
git add internal/distribution/okd/install/flux.go internal/distribution/okd/setup/ignition.go
git commit -m "fix(security): use CopyFileMode 0o600 for kubeconfig and install-config backups"
```

---

### Task 1.7: Category-1 chown-back — `releases/cache.go`

**Files:**
- Modify: `internal/distribution/okd/releases/cache.go`

- [ ] **Step 1: Resolve invoking-user home**

Replace `getCacheFilePath` at `releases/cache.go:19-25`:

```go
func (f *OKDVersionFetcher) getCacheFilePath() (string, error) {
	homeDir, err := system.InvokingUserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".okdctl", "cache", cacheFileName), nil
}
```

- [ ] **Step 2: Write as invoking user**

Replace `saveToDiskCache` at `releases/cache.go:51-68` — the last line changes from `system.AtomicWrite` to `system.WriteAsInvokingUser`:

```go
func (f *OKDVersionFetcher) saveToDiskCache(series []OKDReleaseSeries) {
	cachePath, err := f.getCacheFilePath()
	if err != nil {
		return
	}

	cache := diskCache{CachedAt: time.Now(), Series: series}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return
	}

	// Cache may be written during a root-mode deploy; chown back so the
	// user can still read it when they run `okdctl releases list` as
	// themselves later.
	_ = system.WriteAsInvokingUser(cachePath, data, 0o644)
}
```

- [ ] **Step 3: Chown the parent `.okdctl/cache/` dir if we create it**

The `AtomicWrite` inside `WriteAsInvokingUser` calls `EnsureDirForFile`, which may create `~/.okdctl/cache/` as root. Extend `WriteAsInvokingUser` to chown the parent dirs up to `~/.okdctl/` if we just created them. This is an extension to Task 1.1 — skip if already addressed. If not, apply the patch at the end of `system/elevation.go`:

```go
// WriteAsInvokingUser atomically writes data to path with mode, then
// chowns the file AND any ancestor directories we created (up to the
// invoking user's home) to the invoking user. Ancestor detection is best-
// effort: we chown the immediate parent unconditionally, since Atomic-
// Write's EnsureDirForFile created it if it didn't exist.
func WriteAsInvokingUser(path string, data []byte, mode os.FileMode) error {
	if err := AtomicWrite(path, data, mode); err != nil {
		return err
	}
	if err := ChownToInvokingUser(path); err != nil {
		return err
	}
	return ChownToInvokingUser(filepath.Dir(path))
}
```

- [ ] **Step 4: Verify build**

Run: `go build ./...`
Expected: clean compile.

- [ ] **Step 5: Commit**

```bash
git add internal/system/elevation.go internal/distribution/okd/releases/cache.go
git commit -m "refactor(releases): chown cache back to invoking user under sudo"
```

---

### Task 1.8: Category-1 chown-back — `install/flux.go` kubeconfig + `.bashrc`

**Files:**
- Modify: `internal/distribution/okd/install/flux.go`

- [ ] **Step 1: Resolve invoking-user home for kubeconfig**

Replace `SetupClusterAccess` at `install/flux.go:45-77` so the opening home lookup uses `system.InvokingUserHomeDir`, and the `CopyFileMode(..., 0o600)` (already in place from Task 1.6) is followed by a chown:

```go
func (p *Phase) SetupClusterAccess(_ context.Context, clusterDir string) error {
	homeDir, err := system.InvokingUserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to resolve invoking user home: %w", err)
	}

	kubeDir := filepath.Join(homeDir, ".kube")
	if err := system.EnsureDir(kubeDir); err != nil {
		return fmt.Errorf("failed to create .kube directory: %w", err)
	}
	if err := system.ChownToInvokingUser(kubeDir); err != nil {
		p.Log.Warn(fmt.Sprintf("kubeconfig: could not chown .kube dir: %v", err))
	}

	srcKubeconfig := filepath.Join(clusterDir, "auth", "kubeconfig")
	destKubeconfig := filepath.Join(kubeDir, "config")

	if system.FileExists(destKubeconfig) {
		backupPath := destKubeconfig + ".backup." + time.Now().Format("20060102-150405")
		if err := system.CopyFileMode(destKubeconfig, backupPath, 0o600); err != nil {
			p.Log.Warn(fmt.Sprintf("kubeconfig: could not backup existing file: %v", err))
		} else {
			_ = system.ChownToInvokingUser(backupPath)
			p.Log.Info(fmt.Sprintf("kubeconfig: backed up existing file to %s", backupPath))
		}
	}

	if err := system.CopyFileMode(srcKubeconfig, destKubeconfig, 0o600); err != nil {
		return fmt.Errorf("failed to copy kubeconfig: %w", err)
	}
	if err := system.ChownToInvokingUser(destKubeconfig); err != nil {
		p.Log.Warn(fmt.Sprintf("kubeconfig: could not chown config: %v", err))
	}

	if err := p.addKubeconfigToBashrc(homeDir, destKubeconfig); err != nil {
		p.Log.Warn(fmt.Sprintf("kubeconfig: could not update .bashrc: %v", err))
	}

	return nil
}
```

- [ ] **Step 2: Chown `.bashrc` if we created it fresh**

Replace `addKubeconfigToBashrc` at `install/flux.go:79-120` so that a freshly-created bashrc is chowned to the invoking user:

```go
func (p *Phase) addKubeconfigToBashrc(homeDir, kubeconfigPath string) error {
	bashrcPath := filepath.Join(homeDir, ".bashrc")
	exportLine := fmt.Sprintf("export KUBECONFIG=%s", kubeconfigPath)

	mode := os.FileMode(0o644)
	created := false
	if fi, err := os.Stat(bashrcPath); err == nil {
		mode = fi.Mode().Perm()
	} else if os.IsNotExist(err) {
		created = true
	}

	content, err := os.ReadFile(bashrcPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := system.AtomicWriteString(bashrcPath, exportLine+"\n", mode); err != nil {
				return err
			}
			if created {
				return system.ChownToInvokingUser(bashrcPath)
			}
			return nil
		}
		return err
	}

	if strings.Contains(string(content), "export KUBECONFIG=") {
		return nil
	}

	f, err := os.OpenFile(bashrcPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	if len(content) > 0 && content[len(content)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(f, "\n# Added by okdctl\n%s\n", exportLine); err != nil {
		return err
	}
	// We appended to an existing file. Its ownership was already correct
	// (preserved because we opened, not created). No chown needed.
	return nil
}
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: clean compile.

- [ ] **Step 4: Commit**

```bash
git add internal/distribution/okd/install/flux.go
git commit -m "refactor(install): resolve kubeconfig paths via invoking user and chown back"
```

---

### Task 1.9: Category-2 workdir chown hook in `cli/deploy.go`

**Files:**
- Modify: `internal/cli/helpers.go`

- [ ] **Step 1: Add deferred workdir chown-back in `executeFullDeployment`**

At `internal/cli/helpers.go:120-163`, add a deferred chown-back of the workdir right after `projectRoot` is resolved:

```go
func executeFullDeployment(ctx context.Context, cfg *config.Config, opts deploymentOptions) error {
	clusterFQDN := cfg.Cluster.Name + "." + cfg.Cluster.Domain
	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}

	// Deploy writes per-run artifacts (install-config.yaml, manifests,
	// ignition files, downloaded tools, ISOs) under <projectRoot>/okd-install.
	// Under the sudo re-exec model these are root-owned by default; restore
	// ownership to the invoking user at the end so they can inspect and
	// `rm -rf` without sudo.
	workDir := filepath.Join(projectRoot, "okd-install")
	defer func() {
		if chownErr := system.ChownTreeToInvokingUser(workDir); chownErr != nil {
			tui.Warn(fmt.Sprintf("workdir chown back to user incomplete: %v", chownErr))
		}
	}()

	p := createOKDProvisioner(cfg, opts.Credentials, projectRoot)
	// ... (rest unchanged) ...
```

Add the `system` import at the top of the file if not already present.

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: clean compile.

- [ ] **Step 3: Commit**

```bash
git add internal/cli/helpers.go
git commit -m "feat(cli): chown workdir back to invoking user after deploy"
```

---

### Task 1.10: Drop `HasPasswordlessSudo` preflight from `setup/phase.go` (moot after re-exec)

**Files:**
- Modify: `internal/distribution/okd/setup/phase.go`

- [ ] **Step 1: Remove the preflight block**

Delete lines 92-99 in `setup/phase.go`:

```go
	// Preflight: many setup steps call sudo. If passwordless sudo is not
	// configured and the user has no cached timestamp, the first sudo call
	// will stall waiting for a password read from stdin, which looks like a
	// hung deployment. Warn once up front rather than blocking; the user may
	// have primed sudo earlier in this session.
	if err := system.HasPasswordlessSudo(ctx); err != nil {
		p.Log.Warn("setup: passwordless sudo not configured; next sudo command may hang")
	}
```

Under the re-exec model, `Execute` always runs as root — there is no subsequent sudo call to stall. The check is now only useful in `doctor` as advisory info.

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: clean compile.

- [ ] **Step 3: Commit**

```bash
git add internal/distribution/okd/setup/phase.go
git commit -m "refactor(setup): drop sudo preflight — re-exec model runs setup as root"
```

---

### Task 1.11: Collapse `system.CopyFileWithElevation` / `Chmod` / `Chown` / `MkdirAll` / `RemoveAll` / `RunSudo` call sites to stdlib

This is the bulk of the mechanical work: 55 call sites across 14 files. Pattern is mechanical — each `system.X(ctx, …)` helper maps to a stdlib call.

**Conversion table (applies to every caller):**

| Before | After |
|--------|-------|
| `system.CopyFileWithElevation(ctx, src, dst, desc)` | `system.CopyFile(src, dst)` for non-sensitive; `system.CopyFileMode(src, dst, 0o600)` for credential-bearing (kubeconfig, install-config) |
| `system.Chmod(ctx, path, "0644", desc)` | `os.Chmod(path, 0o644)` — parse octal literal at call site, not a string |
| `system.Chmod(ctx, path, "+x", desc)` | Read current mode, OR with 0o111, `os.Chmod(path, newMode)` — see helper snippet below |
| `system.Chown(ctx, path, "apache:apache", desc)` | Look up user/group once, `os.Chown(path, uid, gid)` |
| `system.MkdirAll(ctx, path, desc)` | `os.MkdirAll(path, 0o755)` |
| `system.RemoveAll(ctx, path, desc)` | `os.RemoveAll(path)` |
| `system.RunSudo(ctx, cmd, args...)` | `exec.CommandContext(ctx, cmd, args...).Run()` — no `sudo` wrapping |

**Helper snippet for `+x`-style mode changes** (add to `system/fs.go` once):

```go
// MakeExecutable adds the owner/group/other execute bits to path's existing
// mode. Equivalent to `chmod +x` but without a subprocess.
func MakeExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	return os.Chmod(path, info.Mode().Perm()|0o111)
}
```

**Helper snippet for user:group resolution** (add to `system/fs.go`):

```go
// ChownByName chowns path to the given user:group string. Both parts must
// be present; empty or numeric-only forms are rejected.
func ChownByName(path, ownerSpec string) error {
	userName, groupName, ok := strings.Cut(ownerSpec, ":")
	if !ok || userName == "" || groupName == "" {
		return fmt.Errorf("invalid owner spec %q: want user:group", ownerSpec)
	}
	u, err := user.Lookup(userName)
	if err != nil {
		return fmt.Errorf("lookup user %s: %w", userName, err)
	}
	g, err := user.LookupGroup(groupName)
	if err != nil {
		return fmt.Errorf("lookup group %s: %w", groupName, err)
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(g.Gid)
	return os.Chown(path, uid, gid)
}
```

**Files to touch** (from the grep — 14 files):

- `internal/platform/packages.go` (7 hits)
- `internal/netutil/iface.go` (2 hits)
- `internal/distribution/okd/dns/dnsmasq.go` (12 hits)
- `internal/distribution/okd/firewall/firewall.go` (6 hits)
- `internal/distribution/okd/dns/dns.go` (3 hits)
- `internal/distribution/okd/cleanup/artifacts.go` (1 hit)
- `internal/distribution/okd/postinstall/haproxy.go` (1 hit)
- `internal/distribution/okd/setup/artifacts.go` (2 hits)
- `internal/distribution/okd/cleanup/packages.go` (1 hit)
- `internal/distribution/okd/setup/haproxy.go` (4 hits)
- `internal/distribution/okd/setup/apache.go` (6 hits)
- `internal/distribution/okd/setup/phase.go` (1 hit — `system.HasPasswordlessSudo`; already removed in Task 1.10)
- `internal/distribution/okd/cleanup/services.go` (3 hits)
- `internal/distribution/okd/setup/tools.go` (6 hits)

- [ ] **Step 1: Add `MakeExecutable` + `ChownByName` helpers to `system/fs.go`**

Paste the two snippets above into `internal/system/fs.go`. Add the needed imports (`os/user`, `strconv`, `strings`) if not already present.

- [ ] **Step 2: Verify build**

Run: `go build ./internal/system/...`
Expected: clean compile.

- [ ] **Step 3: Commit the helpers**

```bash
git add internal/system/fs.go
git commit -m "feat(system): add MakeExecutable and ChownByName stdlib helpers"
```

- [ ] **Step 4: Collapse call sites file-by-file**

For each file in the list above, perform the mechanical substitution per the conversion table. After each file, run `go build ./...` and stage+commit with a scoped message: `refactor(<pkg>): use stdlib for filesystem ops now that root is guaranteed`.

For example, `internal/distribution/okd/setup/apache.go` before:

```go
if err := system.MkdirAll(ctx, ignitionDir, "ignition directory"); err != nil {
    return "", fmt.Errorf("failed to create ignition directory: %w", err)
}
if err := system.Chown(ctx, ignitionDir, apacheUser+":"+apacheUser, "ignition directory ownership"); err != nil {
    p.Log.Warn(fmt.Sprintf("apache: failed to set ignition dir ownership: %v", err))
}
if err := system.Chmod(ctx, ignitionDir, "755", "ignition directory permissions"); err != nil {
    p.Log.Warn(fmt.Sprintf("apache: failed to set ignition dir permissions: %v", err))
}
```

After:

```go
if err := os.MkdirAll(ignitionDir, 0o755); err != nil {
    return "", fmt.Errorf("failed to create ignition directory: %w", err)
}
if err := system.ChownByName(ignitionDir, apacheUser+":"+apacheUser); err != nil {
    p.Log.Warn(fmt.Sprintf("apache: failed to set ignition dir ownership: %v", err))
}
// Note: ChownByName already applied the ownership; the 0o755 came through MkdirAll at creation.
// If ignitionDir pre-existed, explicitly retighten:
if err := os.Chmod(ignitionDir, 0o755); err != nil {
    p.Log.Warn(fmt.Sprintf("apache: failed to set ignition dir permissions: %v", err))
}
```

Commit for this file:
```bash
git add internal/distribution/okd/setup/apache.go
git commit -m "refactor(setup/apache): use stdlib for filesystem ops"
```

Repeat the same pattern for each of the 14 files. Commit after each file so bisect stays clean.

- [ ] **Step 5: After all 14 files, confirm no remaining `system.RunSudo` / `system.Chmod(ctx` / `system.Chown(ctx` / `system.MkdirAll(ctx` / `system.RemoveAll(ctx` / `system.CopyFileWithElevation` / `system.ExecuteFileOperation` / `system.ExecuteWithElevation` callers**

Run:
```bash
rg -n 'system\.(RunSudo|Chmod\(ctx|Chown\(ctx|MkdirAll\(ctx|RemoveAll\(ctx|CopyFileWithElevation|ExecuteFileOperation|ExecuteWithElevation)\b' internal/
```

Expected: no results.

- [ ] **Step 6: Verify full build + lint**

Run:
```bash
go build ./...
golangci-lint run
```

Expected: clean on both.

---

### Task 1.12: Delete the elevation layer

**Files:**
- Delete: `internal/system/permissions.go`
- Delete: `internal/system/fileops_elevated.go`

- [ ] **Step 1: Confirm zero remaining callers of any symbol defined in the deleted files**

Symbols to check: `ExecuteWithElevation`, `isElevationNeeded`, `ExecuteFileOperation`, `OpCopy`/`OpChmod`/`OpChown`/`OpMkdir`/`OpRemove` (the FileOperation enum), `isCriticalPath`, `criticalPaths`, `CopyFileWithElevation`, `system.Chmod` / `system.Chown` / `system.MkdirAll` / `system.RemoveAll` (the ctx-taking variants), `RunSudo`, `runSudo`, `runCommand`, `HasPasswordlessSudo` (the one in `fileops_elevated.go`).

`HasPasswordlessSudo` should survive — it's been migrated to `system/elevation.go` in Task 1.1.

Run:
```bash
rg -n 'ExecuteWithElevation|ExecuteFileOperation|CopyFileWithElevation|system\.RunSudo\b|isCriticalPath' internal/
```

Expected: no results.

- [ ] **Step 2: Relocate `isCriticalPath` to `cleanup`**

`isCriticalPath` was a fs-level guard against `rm -rf /etc` via sudo. Under the re-exec model the guard moves to the only place we actually `RemoveAll` at root privilege: the cleanup phase. Add to `internal/distribution/okd/cleanup/artifacts.go`:

```go
var criticalPaths = []string{"/", "/etc", "/var", "/usr", "/bin", "/sbin", "/lib", "/home", "/root", "/boot", "/dev", "/proc", "/sys"}

// refuseCriticalPath aborts if path resolves to a root-of-system location.
// Defense-in-depth against a config-file typo that points cleanup at the
// wrong target. Returns nil for safe paths.
func refuseCriticalPath(path string) error {
	cleaned := filepath.Clean(path)
	for _, p := range criticalPaths {
		if cleaned == p {
			return fmt.Errorf("refusing to remove critical system path: %s", path)
		}
	}
	return nil
}
```

Call `refuseCriticalPath` at the top of each `os.RemoveAll` in `cleanup/artifacts.go` where the target is derived from config.

- [ ] **Step 3: Delete the files**

```bash
rm internal/system/permissions.go internal/system/fileops_elevated.go
```

- [ ] **Step 4: Verify build + lint**

Run:
```bash
go build ./...
golangci-lint run
```

Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor(system): delete escalation layer — re-exec model makes it dead weight"
```

---

### Task 1.13: Fix `systemd.go` to use plain `exec.CommandContext`

**Files:**
- Modify: `internal/system/systemd.go`

- [ ] **Step 1: Replace `RunSudo`-wrapped systemctl calls with plain `exec.CommandContext`**

Find the file (57 LOC; read it before editing). Each `system.RunSudo(ctx, "systemctl", ...)` becomes `exec.CommandContext(ctx, "systemctl", ...).Run()`. No behavioral change at runtime since we're root.

- [ ] **Step 2: Verify build + lint**

Run:
```bash
go build ./...
golangci-lint run
```

- [ ] **Step 3: Commit**

```bash
git add internal/system/systemd.go
git commit -m "refactor(system/systemd): use plain exec.CommandContext now that root is guaranteed"
```

---

### Task 1.14: Doctor — migrate `HasPasswordlessSudo` reference

**Files:**
- Modify: `internal/cli/doctor.go`

- [ ] **Step 1: Replace inline `sudo -n true` duplicate with `system.HasPasswordlessSudo`**

At `doctor.go:271-282`, the current inline code duplicates what `system.HasPasswordlessSudo` does. Replace the body of that check function with:

```go
if err := system.HasPasswordlessSudo(ctx); err != nil {
	return checkResult{severity: sevWarn, detail: fmt.Sprintf("sudo -n true failed: %v", err)}
}
return checkResult{severity: sevOK}
```

Under the re-exec model this check is informational: it tells the user their next `sudo okdctl deploy` (or re-exec) will prompt for a password. It no longer gates any behavior.

- [ ] **Step 2: Verify build + lint**

Run:
```bash
go build ./...
golangci-lint run
```

- [ ] **Step 3: Commit**

```bash
git add internal/cli/doctor.go
git commit -m "refactor(cli/doctor): delegate sudo check to system.HasPasswordlessSudo"
```

---

### Task 1.15: Phase 1 verification

- [ ] **Step 1: Full build + lint + vet**

```bash
go build ./...
go vet ./...
golangci-lint run
```

Expected: all green.

- [ ] **Step 2: Unprivileged smoke — wizard and doctor**

```bash
go run ./cmd/okdctl --help
go run ./cmd/okdctl doctor
go run ./cmd/okdctl deploy --non-interactive --output /tmp/okdctl-wiz.yaml
```

Expected: `--help` and `doctor` run as current user; `deploy --non-interactive` triggers the sudo re-exec (since `deploy` is root-required). Cancel the sudo prompt; we're only confirming the gate fires.

- [ ] **Step 3: Privileged smoke — one deploy run to a disposable target**

If you have a disposable Proxmox target or VM, run a full `sudo okdctl deploy`. Watch for:
- Exactly one sudo prompt at start.
- After deploy: `ls ~/.okdctl/cache/` owned by you (not root).
- After deploy: `ls ~/.kube/config` owned by you.
- After deploy: `ls ./okd-install/` owned by you.
- No stalls waiting for password mid-run.

- [ ] **Step 4: Open PR**

```bash
git push -u origin phase1-elevation-refactor
gh pr create --title "refactor: replace sudo-escalation with re-exec model; harden fs.go" --body "$(cat <<'EOF'
## Summary

- Root-required commands (deploy/destroy/cleanup/update-ingress) re-exec under sudo at CLI entry; single-UID body uses plain stdlib filesystem ops.
- New `system/elevation.go` helpers: `InvokingUser`, `InvokingUserHomeDir`, `WriteAsInvokingUser`, `ChownToInvokingUser`, `ChownTreeToInvokingUser`.
- Kubeconfig, `~/.bashrc` append, and releases cache chown back to the invoking user.
- Deploy workdir chown-back on exit (success or failure).
- `system/fs.go`: `CopyFile` now atomic-mode via `CopyFileMode` delegation (SEC-001); Close errors on write paths surfaced (ERR-003); kubeconfig + install-config backups use explicit 0o600 (SEC-002).
- Deletes: `system/permissions.go`, `system/fileops_elevated.go`.

## Test plan

- [x] `go build ./...` clean
- [x] `go vet ./...` clean
- [x] `golangci-lint run` clean
- [ ] Manual smoke: wizard + doctor unprivileged
- [ ] Manual smoke: full deploy to disposable target, confirm user-home artifacts owned by invoking user after run
EOF
)"
```

---

# Phase 2 — SEC-003 perms hardening

**Goal:** Tighten mode on three write sites from 0o644 to 0o600.

**Branch:** `phase2-sec-perms` off `develop` (or off `phase1-elevation-refactor` if 1 hasn't merged).

---

### Task 2.1: 0o600 on terraform.tfvars

**Files:**
- Modify: `internal/distribution/okd/setup/terraform.go`

- [ ] **Step 1: Find the tfvars write**

At `setup/terraform.go:143` (read the file first to confirm line), the current write uses `system.AtomicWriteString(path, content, 0o644)`. Change to `0o600`.

- [ ] **Step 2: Verify build**

```bash
go build ./...
```

---

### Task 2.2: 0o600 on GPG tempfiles

**Files:**
- Modify: `internal/distribution/okd/setup/tools.go`

- [ ] **Step 1: Find `WriteTempFile` calls at tools.go:207 and :230**

Both currently use mode `0o644`. Change both to `0o600`.

- [ ] **Step 2: Verify build**

```bash
go build ./...
golangci-lint run
```

- [ ] **Step 3: Commit**

```bash
git add internal/distribution/okd/setup/terraform.go internal/distribution/okd/setup/tools.go
git commit -m "fix(security): 0o600 on tfvars and GPG tempfiles"
```

---

# Phase 3 — TYPE-001 NodeRole enum

**Goal:** Replace `NodeInfo.Role string // bootstrap, master, worker` magic-string enum with a `NodeRole` type. Propagate through `setup/` and `dns/`.

**Branch:** `phase3-noderole-enum` off `develop`.

---

### Task 3.1: Add `NodeRole` type and constants

**Files:**
- Modify: `internal/distribution/okd/types.go`

- [ ] **Step 1: Add the type**

Append to `internal/distribution/okd/types.go`:

```go
// NodeRole is the kubernetes-style role assignment for a cluster node.
// Values are lowercase strings matching what openshift-install, HAProxy
// backend templates, and ignition URLs expect verbatim.
type NodeRole string

const (
	RoleBootstrap NodeRole = "bootstrap"
	RoleMaster    NodeRole = "master"
	RoleWorker    NodeRole = "worker"
)

// ParseNodeRole converts a string to NodeRole, erroring on unknown values.
// Case-sensitive to match the exact role names openshift-install emits.
func ParseNodeRole(s string) (NodeRole, error) {
	switch NodeRole(s) {
	case RoleBootstrap, RoleMaster, RoleWorker:
		return NodeRole(s), nil
	default:
		return "", fmt.Errorf("unknown node role %q (want bootstrap|master|worker)", s)
	}
}

// String satisfies fmt.Stringer for log messages and template substitution.
func (r NodeRole) String() string { return string(r) }
```

Import `fmt` if not already.

- [ ] **Step 2: Verify build**

```bash
go build ./internal/distribution/okd/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/distribution/okd/types.go
git commit -m "feat(okd): add NodeRole typed enum"
```

---

### Task 3.2: Convert `setup.NodeInfo.Role` to `NodeRole`

**Files:**
- Modify: `internal/distribution/okd/setup/phase.go`

- [ ] **Step 1: Change the struct field type**

At `setup/phase.go:61-66`:

```go
type NodeInfo struct {
	Name string
	Role okd.NodeRole
	IP   string
	MAC  string
}
```

Import `"github.com/qxtaiba/okdctl/internal/distribution/okd"` at the top.

- [ ] **Step 2: Find and fix all callers**

Run `rg -n 'NodeInfo\{|\.Role\s*=\s*"(bootstrap|master|worker)"|\.Role\s*==\s*"(bootstrap|master|worker)"'` to locate assignments and comparisons. Replace string literals with `okd.RoleBootstrap`/`RoleMaster`/`RoleWorker`. Replace switch cases with typed constants.

Key files:
- `internal/distribution/okd/setup/nodes.go` — `buildNodeNames(role string)` signature becomes `buildNodeNames(role okd.NodeRole)`.
- `internal/distribution/okd/setup/haproxy.go:27-34` — switch statement updates.
- `internal/distribution/okd/setup/terraform.go:13-27, 60-63` — `buildISOStrings(role string)` and `buildNodeNames` callers.
- `internal/distribution/okd/dns/dns.go` — role iteration uses typed constants.

- [ ] **Step 3: Verify build**

```bash
go build ./...
golangci-lint run
```

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor(okd): propagate NodeRole through setup and dns packages"
```

---

### Task 3.3: Phase 3 verification

- [ ] **Step 1: Full build + lint + vet**

```bash
go build ./...
go vet ./...
golangci-lint run
```

- [ ] **Step 2: Open PR**

```bash
git push -u origin phase3-noderole-enum
gh pr create --title "refactor(okd): introduce NodeRole typed enum" --body "$(cat <<'EOF'
## Summary

- New `NodeRole` type in `internal/distribution/okd/types.go` with `RoleBootstrap`/`Master`/`Worker` constants and `ParseNodeRole`.
- Propagated through `NodeInfo.Role`, `buildNodeNames`, `buildISOStrings`, and the HAProxy backend switch.
- String representation unchanged — openshift-install, ignition URLs, and HAProxy templates see identical literals.

## Test plan

- [x] `go build ./...` clean
- [x] `golangci-lint run` clean
- [ ] Manual: smoke a wizard run, confirm role assignments render correctly
EOF
)"
```

---

# Phase 4 — TYPE-002 node readiness JSON parse

**Goal:** Replace `strings.Contains(line, "Ready") && !strings.Contains(line, "NotReady")` on kubectl text output with a structured `-o json` parse.

**Branch:** `phase4-node-readiness-json` off `develop`.

---

### Task 4.1: Define a local readiness parser

**Files:**
- Modify: `internal/distribution/okd/postinstall/verify.go`

- [ ] **Step 1: Add a local struct + parser**

Full `k8s.io/api/core/v1.NodeStatus` is overkill here. Add a minimal local type above the current `verifyNodesReady`:

```go
type nodeCondition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

type nodeReadiness struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Status struct {
			Conditions []nodeCondition `json:"conditions"`
		} `json:"status"`
	} `json:"items"`
}

// parseNodeReadiness returns (ready, total) from a `oc get nodes -o json`
// payload. A node is ready when it has a condition with type=Ready and
// status=True.
func parseNodeReadiness(payload []byte) (ready, total int, err error) {
	var n nodeReadiness
	if err := json.Unmarshal(payload, &n); err != nil {
		return 0, 0, fmt.Errorf("parse node list json: %w", err)
	}
	for _, node := range n.Items {
		total++
		for _, cond := range node.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == "True" {
				ready++
				break
			}
		}
	}
	return ready, total, nil
}
```

- [ ] **Step 2: Replace the string-contains check**

At `verify.go:62` (read the file to locate the exact block), swap the `oc get nodes` subprocess from text-output to `-o json` and call `parseNodeReadiness`.

- [ ] **Step 3: Verify build**

```bash
go build ./...
golangci-lint run
```

- [ ] **Step 4: Commit**

```bash
git add internal/distribution/okd/postinstall/verify.go
git commit -m "refactor(postinstall): parse node readiness from oc -o json instead of text"
```

---

# Phase 5 — TYPE-003 addon settings key consts

**Goal:** Replace magic string keys in `AddonConfig.Settings` map with per-addon named constants.

**Branch:** `phase5-addon-settings-consts` off `develop`.

---

### Task 5.1: Flux addon key consts

**Files:**
- Modify: `internal/addon/catalog/flux/flux.go`

- [ ] **Step 1: Add top-of-file consts**

Locate the file and near the top, after imports, add:

```go
const (
	SettingRepository = "repository"
	SettingBranch     = "branch"
	SettingPath       = "path"
	SettingSecretsDir = "secrets_dir"
)
```

- [ ] **Step 2: Replace all `settings["repository"]` etc. with `settings[SettingRepository]`**

Run `rg -n 'settings\["(repository|branch|path|secrets_dir)"\]'` within `internal/addon/catalog/flux/`, replace each.

- [ ] **Step 3: Same for DefaultSettings and ValidateSettings**

- [ ] **Step 4: Verify build**

```bash
go build ./...
golangci-lint run
```

---

### Task 5.2: Secretstore addon key consts

**Files:**
- Modify: `internal/addon/catalog/secretstore/secretstore.go`

- [ ] **Step 1: Add consts + replace string literals** (same pattern as Task 5.1 for whatever keys secretstore uses — read the file to enumerate).

- [ ] **Step 2: Verify build + commit**

```bash
go build ./...
golangci-lint run
git add -A
git commit -m "refactor(addon): replace magic settings keys with per-addon consts"
```

---

# Phase 6 — MOD-001 netip cleanup

**Goal:** Remove the mixed `net` + `net/netip` imports in `internal/netutil/ip.go`. Migrate the stragglers (`ResolveVIP`, `DeriveVIPFromStaticIP`, `CIDRToNetmask`, `isValidNetmask`) to `netip`.

**Branch:** `phase6-netip-cleanup` off `develop`.

---

### Task 6.1: Migrate straggler functions

**Files:**
- Modify: `internal/netutil/ip.go`
- Modify: `internal/config/validators.go` (validators.go:383 — `isValidNetmask`)

- [ ] **Step 1: Read `internal/netutil/ip.go` in full**

- [ ] **Step 2: Rewrite the stragglers using `netip.Addr` / `netip.ParsePrefix`**

Example replacement for `ResolveVIP` using `netip`:

```go
func ResolveVIP(cfg *config.Networking) (string, error) {
	if cfg.VIP != "" {
		if _, err := netip.ParseAddr(cfg.VIP); err != nil {
			return "", fmt.Errorf("invalid VIP %q: %w", cfg.VIP, err)
		}
		return cfg.VIP, nil
	}
	if cfg.StaticIP == "" {
		return "", errors.New("either VIP or StaticIP must be set")
	}
	return DeriveVIPFromStaticIP(cfg.StaticIP)
}
```

`CIDRToNetmask` keeps its `fmt.Sprintf("%d.%d.%d.%d", …)` output format (HAProxy templates depend on it). Inside, use `netip.ParsePrefix(cidr).Bits()` to get the prefix length, then synthesize the four octets manually.

`isValidNetmask` in `config/validators.go:383` — convert to `netip.ParseAddr` + octet arithmetic.

- [ ] **Step 3: Drop the `net` import**

Once all four stragglers are migrated, remove `"net"` from the imports block in `ip.go`. If anything else still uses `net`, keep the import and reconsider the scope.

- [ ] **Step 4: Verify build + template rendering unchanged**

```bash
go build ./...
golangci-lint run
```

Grep for any template that consumes `CIDRToNetmask` output and confirm the format string didn't change.

- [ ] **Step 5: Commit**

```bash
git add internal/netutil/ip.go internal/config/validators.go
git commit -m "refactor(netutil): migrate remaining stragglers from net to net/netip"
```

---

# Phase 7 — CTX-003 context threading

**Goal:** Thread `context.Context` through `PackageManager.IsInstalled` and `setup/tools.go:getToolVersion` so `Ctrl-C` mid-deploy actually cancels the rpm/dpkg lookup.

**Branch:** `phase7-ctx-threading` off `develop`.

---

### Task 7.1: Thread ctx through `PackageManager`

**Files:**
- Modify: `internal/platform/platform.go` (interface definition)
- Modify: `internal/platform/packages.go`
- Modify: every caller of `PackageManager.IsInstalled` / `Install` (grep to find)

- [ ] **Step 1: Change the interface signature**

In `platform/platform.go`, the `PackageManager` interface: add `context.Context` as the first parameter to `IsInstalled` and `Install` (if not already present).

- [ ] **Step 2: Update the implementations at `packages.go:62,103`**

`exec.CommandContext(context.Background(), …)` becomes `exec.CommandContext(ctx, …)`.

- [ ] **Step 3: Update all callers**

Run `rg -n '\.IsInstalled\(|\.Install\(' internal/` and thread the ctx through.

- [ ] **Step 4: Same for `setup/tools.go:getToolVersion`**

Current signature: `func getToolVersion(tool, flag string) string`. New signature: `func getToolVersion(ctx context.Context, tool, flag string) string`. Update the two callers.

- [ ] **Step 5: Verify build + lint**

```bash
go build ./...
golangci-lint run
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor(platform): thread ctx through PackageManager and tool version lookup"
```

---

# Phase 8 — Doctor + polish bundle

**Goal:** Land four small independent cleanups: ERR-002 (doctor error concat), CTX-001 doc comment (registry-of-checks pattern), SUB-003 (wget stderr capture), CON-001 (sync.Once around Kill).

**Branch:** `phase8-polish-bundle` off `develop`.

---

### Task 8.1: ERR-002 — doctor.go error concatenation

**Files:**
- Modify: `internal/cli/doctor.go`

- [ ] **Step 1: Replace `"cannot X: " + err.Error()` with `fmt.Sprintf("cannot X: %v", err)`**

At `doctor.go:189,320,326,340,347,370` (confirm via grep). The `%v` form is the idiomatic Go stringify-for-humans; it also composes correctly if a future wrap adds structure.

- [ ] **Step 2: Verify build**

```bash
go build ./...
```

---

### Task 8.2: CTX-001 — document the registry-of-checks pattern

**Files:**
- Modify: `internal/cli/doctor.go`

- [ ] **Step 1: Add a package-scope or top-of-file doc comment**

At the top of `doctor.go`, above the first `checkXxx` function, add:

```go
// Doctor checks share a uniform signature `func(context.Context) checkResult`
// so they can be stored in a registry slice and invoked in a loop. Some
// checks (checkHostOS, checkPath, checkSSHKey, checkBinaries) genuinely
// have no cancellable work — the blank-named ctx parameter is deliberate,
// kept for signature uniformity. Checks that call into subprocesses
// (sudo -n true, rpm queries) do use the context.
```

- [ ] **Step 2: Commit**

```bash
git add internal/cli/doctor.go
git commit -m "docs(doctor): document uniform-signature registry-of-checks pattern"
```

---

### Task 8.3: SUB-003 — capture wget stderr

**Files:**
- Modify: `internal/distribution/okd/setup/tools.go`

- [ ] **Step 1: At tools.go:207-213, capture stderr**

Before:

```go
gpgTmp, err := system.WriteTempFile("hashicorp-gpg", 0o600, func(f *os.File) error {
	cmd := exec.CommandContext(ctx, "wget", "-qO-", "https://apt.releases.hashicorp.com/gpg")
	cmd.Stdout = f
	return cmd.Run()
})
```

After:

```go
gpgTmp, err := system.WriteTempFile("hashicorp-gpg", 0o600, func(f *os.File) error {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "wget", "-qO-", "https://apt.releases.hashicorp.com/gpg")
	cmd.Stdout = f
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wget failed: %w (stderr: %s)", err, stderr.String())
	}
	return nil
})
```

Add `"bytes"` import.

- [ ] **Step 2: Verify build + commit**

```bash
go build ./...
git add internal/distribution/okd/setup/tools.go
git commit -m "fix(setup): capture wget stderr on GPG key fetch"
```

---

### Task 8.4: CON-001 — `sync.Once` around Kill in monitor.go

**Files:**
- Modify: `internal/distribution/okd/install/monitor.go`

- [ ] **Step 1: Read the file at monitor.go:57-123**

- [ ] **Step 2: Wrap the `installCmd.Process.Kill()` call in a `sync.Once`**

Add `var killOnce sync.Once` at the appropriate scope and change `_ = installCmd.Process.Kill()` to `killOnce.Do(func() { _ = installCmd.Process.Kill() })`.

- [ ] **Step 3: Verify build + commit**

```bash
go build ./...
git add internal/distribution/okd/install/monitor.go
git commit -m "refactor(install/monitor): guard Kill with sync.Once for intent clarity"
```

---

# Phase 9 — DEP-001 drop direct yaml.v3 if unused

**Goal:** Audit direct `gopkg.in/yaml.v3` callers. If every direct caller can be served by `sigs.k8s.io/yaml` (which wraps yaml.v3 under the hood and is already a direct dep), drop the explicit `gopkg.in/yaml.v3` require line.

**Branch:** `phase9-yaml-direct-drop` off `develop`.

---

### Task 9.1: Enumerate callers

- [ ] **Step 1: Grep**

```bash
rg -n '"gopkg.in/yaml.v3"' internal/ --glob '*.go' -l
```

List each file.

- [ ] **Step 2: For each caller, check whether `sigs.k8s.io/yaml`'s public API covers it**

`sigs.k8s.io/yaml` uses JSON struct tags (what k8s types use) and may not handle all yaml.v3-specific tags/comments. If a caller uses yaml.v3-specific features (node tree, custom markers, line-column diagnostics), it needs to stay on yaml.v3.

If ALL callers work with sigs.k8s.io/yaml, proceed. Otherwise abort and keep the direct dep.

- [ ] **Step 3: If proceeding, migrate each caller**

Replace `import "gopkg.in/yaml.v3"` with `import "sigs.k8s.io/yaml"`. The functions `Marshal` / `Unmarshal` have compatible signatures for the common case.

- [ ] **Step 4: `go mod tidy`**

```bash
go mod tidy
go build ./...
golangci-lint run
```

- [ ] **Step 5: Confirm `gopkg.in/yaml.v3` dropped from direct requires (it will still appear as indirect)**

```bash
grep 'gopkg.in/yaml.v3' go.mod
```

Expected: appears only under `// indirect` or not at all.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/
git commit -m "chore(deps): drop direct gopkg.in/yaml.v3 (covered by sigs.k8s.io/yaml)"
```

---

# Phase 10 — Optional httputil trim

**Goal:** Delete the pure-wrapper factories `httputil.NewClient` / `httputil.NewAPIClient`; keep the `TimeoutShort`/`Medium`/`Long` constants as shared timeout policy. Callers use `&http.Client{Timeout: httputil.TimeoutMedium}` directly.

**This phase is optional — ship only if you've confirmed with the codeowner that removing the factory indirection is preferable to keeping the helper package shape.**

**Branch:** `phase10-httputil-trim` off `develop`.

---

### Task 10.1: Delete factory funcs

**Files:**
- Modify: `internal/httputil/httputil.go`

- [ ] **Step 1: Enumerate callers**

```bash
rg -n 'httputil\.(New|NewAPIClient|NewClient)\b' internal/
```

- [ ] **Step 2: Rewrite each caller inline**

For each `httputil.NewClient(timeout)` call, replace with `&http.Client{Timeout: timeout}`. For `httputil.NewAPIClient()`, replace with `&http.Client{Timeout: httputil.TimeoutShort}` (or whichever timeout it used).

- [ ] **Step 3: Delete the factory funcs from `httputil/httputil.go`, keep only the constants**

The file shrinks to ~9 LOC: package doc + three timeout constants.

- [ ] **Step 4: Verify build + lint**

```bash
go build ./...
golangci-lint run
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor(httputil): drop pure-wrapper factories; keep shared timeout policy as consts"
```

---

# End-to-end verification after all phases

- [ ] Full `go build ./...` clean
- [ ] Full `golangci-lint run` clean
- [ ] Full `go vet ./...` clean
- [ ] Full-deploy smoke to a disposable Proxmox target, confirm:
  - Exactly one sudo prompt at start
  - Wizard remains unprivileged
  - `~/.okdctl/cache/okd-versions.json` owned by invoking user after run
  - `~/.kube/config` + `.backup.TIMESTAMP` owned by invoking user
  - `~/.bashrc` ownership preserved (or owned by invoking user if freshly created)
  - `./okd-install/` tree owned by invoking user
  - Node role strings render correctly in HAProxy config and ignition URLs (Phase 3)
  - `oc get nodes -o json` path returns correct readiness count (Phase 4)
  - Deploy interrupt via Ctrl-C actually cancels package queries (Phase 7)
- [ ] `govulncheck` passes CI
- [ ] `lint-yaml` and `validate-terraform` pass CI

---

# Self-review checklist (completed before handoff)

- **Spec coverage:** Every item from the consolidated audit is assigned to a phase: elevation (SUB pattern) → Phase 1; SEC-001/002/ERR-003 fold-ins → Phase 1; SEC-003 → Phase 2; TYPE-001 → Phase 3; TYPE-002 → Phase 4; TYPE-003 → Phase 5; MOD-001 → Phase 6; CTX-003 → Phase 7; ERR-002/CTX-001/SUB-003/CON-001 → Phase 8; DEP-001 → Phase 9; httputil challenge → Phase 10 (optional).
- **Placeholder scan:** No "TBD", "TODO", "implement later", or "similar to Task N" in this document.
- **Type consistency:** `NodeRole` constants named `RoleBootstrap`/`RoleMaster`/`RoleWorker` throughout Phase 3. `system.InvokingUser` / `InvokingUserHomeDir` / `ChownToInvokingUser` / `WriteAsInvokingUser` / `ChownTreeToInvokingUser` / `HasPasswordlessSudo` naming consistent across Phase 1 tasks.
- **Commit style:** Every commit uses `type(scope): description`, lowercase, imperative, no trailers.
