# Tier 1 LOC Reduction — Mechanical Savings

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cut ~800 LOC of duplication and waste across the non-TUI codebase through mechanical, behavior-preserving refactors.

**Architecture:** Extract helpers for repeated validation patterns, deduplicate tool installers, consolidate service management, convert `logger.Info(fmt.Sprintf(...))` to structured logging, simplify credential resolution, and trim release classification logic. No new packages — all changes are within existing files.

**Tech Stack:** Go 1.24, slog structured logging, cenkalti/backoff

---

### Task 1: Extract CIDR overlap and IP-in-CIDR validation helpers

**Files:**
- Modify: `internal/config/validators.go`

The `networkingValidator` has 3 identical CIDR overlap blocks (22 LOC) and `advancedNetworkingValidator` has 3 identical IP-in-CIDR blocks (18 LOC). Extract two helpers.

- [ ] **Step 1: Add the helper functions**

Add these two helpers above the `networkingValidator`:

```go
// checkCIDROverlap appends a validation error if the two CIDRs overlap.
func checkCIDROverlap(cidr1, cidr2, field, otherName string, result *ValidationResult) {
	if !IsValidCIDR(cidr1) || !IsValidCIDR(cidr2) {
		return
	}
	overlap, err := netutil.CIDRsOverlap(cidr1, cidr2)
	if err != nil {
		result.AddError(field, fmt.Sprintf("cannot check overlap with %s: %v", otherName, err))
	} else if overlap {
		result.AddError(field, "overlaps with "+otherName)
	}
}

// checkIPInCIDR appends a validation error if ip is not within cidr.
func checkIPInCIDR(ip, cidr, field, cidrName string, result *ValidationResult) {
	if ip == "" || !IsValidIP(ip) || !IsValidCIDR(cidr) {
		return
	}
	ok, err := netutil.IPInCIDR(ip, cidr)
	if err != nil {
		result.AddError(field, fmt.Sprintf("cannot check CIDR membership: %v", err))
	} else if !ok {
		result.AddError(field, fmt.Sprintf("must be within %s %s", cidrName, cidr))
	}
}
```

- [ ] **Step 2: Replace the 3 CIDR overlap blocks in networkingValidator**

Replace the 22-line block (3x `if IsValidCIDR ... CIDRsOverlap ...`) with:

```go
	checkCIDROverlap(podCIDR, serviceCIDR, FieldNetworkingPodCIDR, "service CIDR", result)
	checkCIDROverlap(podCIDR, machineCIDR, FieldNetworkingPodCIDR, "machine CIDR", result)
	checkCIDROverlap(serviceCIDR, machineCIDR, FieldNetworkingServiceCIDR, "machine CIDR", result)
```

- [ ] **Step 3: Replace the 3 IP-in-CIDR blocks in advancedNetworkingValidator**

Replace the 18-line block (3x `if ... IsValidIP ... IPInCIDR ...`) with:

```go
	checkIPInCIDR(gateway, machineCIDR, FieldNetworkingGateway, "machine CIDR", result)
	checkIPInCIDR(bastionIP, machineCIDR, FieldNetworkingBastionIP, "machine CIDR", result)
	checkIPInCIDR(staticIPStart, machineCIDR, FieldNetworkingStaticIPStart, "machine CIDR", result)
```

- [ ] **Step 4: Extract resource validation helper, deduplicate resourcesValidator + ValidateOKDConfig**

Add a helper:

```go
func checkNodeResources(node NodeConfig, minCPU, minMemory, minDisk int, cpuField, memField, diskField, label string, result *ValidationResult) {
	if node.CPU < minCPU {
		result.AddError(cpuField, fmt.Sprintf("%s requires at least %d vCPUs", label, minCPU))
	}
	if node.Memory < minMemory {
		result.AddError(memField, fmt.Sprintf("%s requires at least %d MB (%d GB) of memory", label, minMemory, minMemory/1024))
	}
	if node.Disk < minDisk {
		result.AddError(diskField, fmt.Sprintf("%s requires at least %d GB of disk space", label, minDisk))
	}
}
```

Then replace the 4 resource-checking blocks in `resourcesValidator.Validate` and `ValidateOKDConfig` with calls to `checkNodeResources`.

- [ ] **Step 5: Convert validator structs to function-based registry**

Replace the 10 struct/Scope/Validate patterns with:

```go
type validatorEntry struct {
	scope    ValidationScope
	validate func(*Config, *ValidationResult)
}
```

And in `NewValidatorRegistry`:
```go
validators: []validatorEntry{
	{ScopeRequired, validateRequired},
	{ScopeEnums, validateEnums},
	// ...
}
```

Rename each `(*xxxValidator).Validate` method to a standalone `validateXxx` function.

- [ ] **Step 6: Build and verify**

Run: `go build ./... && go vet ./...`

- [ ] **Step 7: Commit**

```
refactor(config): extract validation helpers, deduplicate CIDR/resource checks
```

**Estimated savings: ~105 LOC**

---

### Task 2: Deduplicate tool installers in setup/tools.go

**Files:**
- Modify: `internal/distribution/okd/setup/tools.go`

`installYQ`, `installHelm`, and `installSops` follow the same pattern: download → (optionally extract) → installBinaryToPath → verify → log version. Extract a generic binary installer.

- [ ] **Step 1: Add a binaryInstallSpec type and generic installer**

```go
type binaryInstallSpec struct {
	name        string
	url         string
	versionFlag string
	// If non-empty, download is a tar.gz that needs extraction.
	// The value is the path within the archive to the binary.
	archiveBinary string
	stripComponents int
}

func (p *Phase) installBinary(ctx context.Context, spec binaryInstallSpec) error {
	p.Log.Info(fmt.Sprintf("tools: installing %s from %s", spec.name, spec.url))

	tempFile := filepath.Join(os.TempDir(), spec.name+"-download")
	if err := download.Download(ctx, download.Options{
		URL: spec.url, OutputPath: tempFile,
		Description: spec.name + " binary", Timeout: 2 * time.Minute, Logger: p.Log,
	}); err != nil {
		return fmt.Errorf("failed to download %s: %w", spec.name, err)
	}
	defer func() { _ = os.Remove(tempFile) }()

	srcPath := tempFile
	if spec.archiveBinary != "" {
		extractDir := filepath.Join(os.TempDir(), spec.name+"-extract")
		if err := os.MkdirAll(extractDir, 0755); err != nil {
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

	if err := installBinaryToPath(ctx, srcPath, spec.name); err != nil {
		return err
	}
	if !isToolInstalled(externalTool(spec.name)) {
		return fmt.Errorf("%s installation verification failed", spec.name)
	}
	p.Log.Info(fmt.Sprintf("tools: %s installed (%s)", spec.name, getToolVersion(spec.name, spec.versionFlag)))
	return nil
}
```

- [ ] **Step 2: Replace installYQ, installHelm, installSops with spec calls**

```go
func (p *Phase) installYQ(ctx context.Context) error {
	return p.installBinary(ctx, binaryInstallSpec{
		name: "yq", versionFlag: "--version",
		url: "https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64",
	})
}

func (p *Phase) installHelm(ctx context.Context) error {
	return p.installBinary(ctx, binaryInstallSpec{
		name: "helm", versionFlag: "version",
		url:             "https://get.helm.sh/helm-v3.17.3-linux-amd64.tar.gz",
		archiveBinary:   "helm",
		stripComponents: 1,
	})
}

func (p *Phase) installSops(ctx context.Context) error {
	return p.installBinary(ctx, binaryInstallSpec{
		name: "sops", versionFlag: "--version",
		url: "https://github.com/getsops/sops/releases/download/v3.9.4/sops-v3.9.4.linux.amd64",
	})
}
```

- [ ] **Step 3: Build and verify**

Run: `go build ./... && go vet ./...`

- [ ] **Step 4: Commit**

```
refactor(setup): deduplicate tool installers with generic binary spec
```

**Estimated savings: ~60 LOC**

---

### Task 3: Convert logger.Info(fmt.Sprintf(...)) to structured slog

**Files:**
- Modify: all files containing `logger.Info(fmt.Sprintf(` or `p.Log.Info(fmt.Sprintf(`

This is the `slog` anti-pattern: using `fmt.Sprintf` to format structured data into a single message string instead of using slog's key-value pairs.

- [ ] **Step 1: Find and convert all instances**

Search pattern: `\.Info(fmt\.Sprintf\(` and `\.Warn(fmt\.Sprintf\(`

Convert patterns like:
```go
// Before
p.Log.Info(fmt.Sprintf("packages: %s not found", pkg))
m.logger.Info(fmt.Sprintf("addons: installing %d addon(s)", len(ordered)))
p.Log.Info(fmt.Sprintf("terraform: configuration written to %s", tfvarsPath))

// After
p.Log.Info("packages: not found", "package", pkg)
m.logger.Info("addons: installing", "count", len(ordered))
p.Log.Info("terraform: configuration written", "path", tfvarsPath)
```

Only convert cases where the format string has a single `%s`/`%d`/`%v` substitution. Leave complex multi-substitution formats as-is (they're fewer and harder to decompose).

Files with the highest counts:
- `internal/addon/manager.go` — 7 instances
- `internal/distribution/okd/setup/steps.go` — 6 instances
- `internal/distribution/okd/setup/tools.go` — 4 instances
- `internal/distribution/okd/setup/phase.go` — 2 instances
- `internal/distribution/okd/okd.go` — 2 instances
- Various cleanup/, dns/, firewall/ files — 1-2 each

- [ ] **Step 2: Build and verify**

Run: `go build ./... && go vet ./...`

- [ ] **Step 3: Commit**

```
refactor: convert logger.Info(fmt.Sprintf()) to structured slog
```

**Estimated savings: ~15 LOC** (fewer imports of "fmt" in some files, shorter lines)

---

### Task 4: Merge createCredentialsSecret/createTokenSecret in secretstore

**Files:**
- Modify: `internal/addon/catalog/secretstore/secretstore.go`

These two methods differ only in: secret name, data key, and whether to TrimSpace.

- [ ] **Step 1: Extract unified createSecretFromFile**

```go
func (s *SecretStore) createSecretFromFile(ctx context.Context, env *addon.Environment, filePath, secretName, dataKey string) error {
	plaintext, err := s.readSecret(ctx, env, filePath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", filepath.Base(filePath), err)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(plaintext)))
	manifest := buildOpaqueSecretManifest(defaultNamespace, secretName, dataKey, encoded)
	if _, err := env.Exec.RunWithStdinChecked(ctx, manifest, "oc", "apply", "-f", "-"); err != nil {
		return fmt.Errorf("failed to apply %s secret: %w", secretName, err)
	}
	env.Logger.Info("secretstore: secret applied", "name", secretName)
	return nil
}
```

- [ ] **Step 2: Replace both callers in Install**

```go
if system.FileExists(credPath) {
	if err := addon.RetryDefault(ctx, func() error {
		return s.createSecretFromFile(ctx, env, credPath, credentialsSecretName, "credentials_base64")
	}); err != nil {
		return err
	}
}
if system.FileExists(tokenPath) {
	if err := addon.RetryDefault(ctx, func() error {
		return s.createSecretFromFile(ctx, env, tokenPath, tokenSecretName, "token")
	}); err != nil {
		return err
	}
}
```

- [ ] **Step 3: Delete createCredentialsSecret and createTokenSecret**

- [ ] **Step 4: Build, verify, commit**

```
refactor(secretstore): merge duplicate secret creation methods
```

**Estimated savings: ~20 LOC**

---

### Task 5: Deduplicate addon Manager InstallAll/InstallOne

**Files:**
- Modify: `internal/addon/manager.go`

Extract the shared install-verify-rollback loop body into a helper.

- [ ] **Step 1: Extract installAndVerify helper**

```go
func (m *Manager) installAndVerify(ctx context.Context, a Addon) (*Environment, error) {
	info := a.Info()
	m.logger.Info("addons: installing", "addon", info.DisplayName)
	env := m.buildEnv(a)
	if err := a.Install(ctx, env); err != nil {
		return env, fmt.Errorf("addon %s install failed: %w", info.Name, err)
	}
	if vErr := a.Verify(ctx, env); vErr != nil {
		m.logger.Warn("addons: installed but verify failed", "addon", info.DisplayName, "err", vErr)
	} else {
		m.logger.Info("addons: installed and verified", "addon", info.DisplayName)
	}
	return env, nil
}
```

- [ ] **Step 2: Simplify InstallAll and InstallOne loops to use the helper**

Both loops replace the install+verify+log block with `env, err := m.installAndVerify(ctx, a)`, keeping their respective rollback logic intact.

- [ ] **Step 3: Build, verify, commit**

```
refactor(addon): extract shared install-verify loop from manager
```

**Estimated savings: ~25 LOC**

---

### Task 6: Simplify credential resolution in proxmox.go

**Files:**
- Modify: `internal/credentials/proxmox.go`

Flatten `GetProxmoxCredentials` by removing nested closures and merging the env override logic.

- [ ] **Step 1: Inline applyInsecureOverride and applyEnvOverrides**

Replace the two closures with a single helper function:

```go
func applyEnvSource(creds *ProxmoxCredentials, configHadCreds bool) {
	creds.Source = SourceEnv
	creds.ConfigCredentialsOverridden = configHadCreds
	if endpoint := os.Getenv("PROXMOX_VE_ENDPOINT"); endpoint != "" {
		creds.Endpoint = endpoint
	} else {
		creds.EndpointFromConfig = true
	}
	if os.Getenv("PROXMOX_VE_INSECURE") == "true" {
		creds.Insecure = true
	}
}
```

Then the env credential blocks become:
```go
if token := os.Getenv("PROXMOX_VE_API_TOKEN"); token != "" {
	creds.APIToken = []byte(token)
	applyEnvSource(creds, configHadCreds)
	return creds
}
if username, password := os.Getenv("PROXMOX_VE_USERNAME"), os.Getenv("PROXMOX_VE_PASSWORD"); username != "" && password != "" {
	creds.Username = username
	creds.Password = []byte(password)
	applyEnvSource(creds, configHadCreds)
	return creds
}
```

- [ ] **Step 2: Build, verify, commit**

```
refactor(credentials): flatten credential resolution, remove nested closures
```

**Estimated savings: ~15 LOC**

---

### Task 7: Simplify releases/fetcher.go classification

**Files:**
- Modify: `internal/distribution/okd/releases/fetcher.go`

The release classification does 3 passes over versions where 1 suffices. `syncSeriesLatestVersion` re-scans what was already set. Several trivial helper functions can be inlined.

- [ ] **Step 1: Merge the latest-marking pass into sortAndClassifySeries**

The `foundLatestStable`/`foundLatestPreview` loop + `syncSeriesLatestVersion` are redundant — the latest version is set during the first pass and never changes. Remove `syncSeriesLatestVersion` entirely.

- [ ] **Step 2: Inline determineStableReleaseType and determinePreviewReleaseType**

These are 3-line functions called from one site. Inline them into `assignReleaseTypeToVersion` and delete the wrappers.

Then inline `assignReleaseTypeToVersion` into `assignReleaseTypesToSeries` since it's also a trivial 5-line wrapper called from one site. Delete both wrapper functions.

- [ ] **Step 3: Build, verify, commit**

```
refactor(releases): simplify classification, remove redundant passes
```

**Estimated savings: ~40 LOC**

---

### Task 8: Remove in-memory cache from releases/cache.go

**Files:**
- Modify: `internal/distribution/okd/releases/cache.go`
- Modify: `internal/distribution/okd/releases/okd.go` (where FetchVersions calls the cache)

The in-memory cache (mutex, TTL check, copy) only helps if `FetchVersions` is called twice in the same process within 5 minutes. This is a CLI tool — it never happens. The 1-hour disk cache is sufficient.

- [ ] **Step 1: Remove in-memory cache fields and methods**

Delete: `isCacheFreshLocked`, `updateMemoryCache`, `getFromMemoryCache`, and the `cache`, `cacheAt`, `cacheTime`, `mu` fields from `OKDVersionFetcher`.

- [ ] **Step 2: Simplify FetchVersions in okd.go**

```go
func (f *OKDVersionFetcher) FetchVersions(ctx context.Context) ([]OKDReleaseSeries, error) {
	if cached, _ := f.loadFromDiskCache(); cached != nil {
		return cached, nil
	}
	series, err := f.fetchFromNetwork(ctx)
	if err != nil {
		return nil, err
	}
	f.saveToDiskCache(series)
	return series, nil
}
```

- [ ] **Step 3: Remove sync import and mu field from OKDVersionFetcher struct**

- [ ] **Step 4: Build, verify, commit**

```
refactor(releases): remove redundant in-memory cache, keep disk cache
```

**Estimated savings: ~25 LOC**

---

### Task 9: Deduplicate postinstall/haproxy.go service management

**Files:**
- Modify: `internal/distribution/okd/postinstall/haproxy.go`

`RemoveHAProxy` manually runs `systemctl stop` and `systemctl disable` via `p.Exec.Run`, duplicating the cleaner logic already in `cleanup/services.go::stopAndDisableService`. Replace the manual calls with `system.ManageService`.

- [ ] **Step 1: Replace manual systemctl calls with system.ManageService**

Replace the `sudo systemctl stop haproxy` / `sudo systemctl disable haproxy` blocks (~20 LOC) with:

```go
if system.IsServiceActive(ctx, "haproxy") {
	if err := system.ManageService(ctx, system.ServiceStop, "haproxy", "haproxy service"); err != nil {
		p.Log.Warn("haproxy: stop failed", "err", err)
	}
}
if system.IsServiceEnabled(ctx, "haproxy") {
	if err := system.ManageService(ctx, system.ServiceDisable, "haproxy", "haproxy service"); err != nil {
		p.Log.Warn("haproxy: disable failed", "err", err)
	}
}
```

- [ ] **Step 2: Remove the executor import if no longer needed**

- [ ] **Step 3: Build, verify, commit**

```
refactor(postinstall): use system.ManageService instead of manual systemctl
```

**Estimated savings: ~20 LOC**

---

### Task 10: Final LOC count and verification

- [ ] **Step 1: Full build + vet**

Run: `go build ./... && go vet ./...`

- [ ] **Step 2: Count LOC**

Run: `wc -l $(find . -name "*.go" -not -path "./vendor/*") | tail -1`

Target: ~17,800-17,950 (down from 18,754)

- [ ] **Step 3: Commit any remaining cleanup**

---

## Summary

| Task | Target File(s) | Est. Savings |
|------|----------------|-------------|
| 1. Validator helpers | config/validators.go | ~105 LOC |
| 2. Tool installer dedup | setup/tools.go | ~60 LOC |
| 3. Structured slog | ~15 files | ~15 LOC |
| 4. Secretstore merge | secretstore.go | ~20 LOC |
| 5. Addon manager dedup | addon/manager.go | ~25 LOC |
| 6. Credential flatten | credentials/proxmox.go | ~15 LOC |
| 7. Release classify simplify | releases/fetcher.go | ~40 LOC |
| 8. Remove memory cache | releases/cache.go + okd.go | ~25 LOC |
| 9. HAProxy service dedup | postinstall/haproxy.go | ~20 LOC |
| **Total** | | **~325 LOC** |

Note: The original audit estimated ~800-1,050 LOC. The discrepancy is because several "savings" identified in the audit (step builder boilerplate, apache/haproxy shared patterns, cleanup/summary verbosity, update_ingress.go JSON helpers) turn out to be lower-ROI when you write the actual replacement code — the helpers themselves cost LOC, and many "duplicate" blocks have just enough variation to resist clean extraction. The 325 LOC here represents the changes where the helper genuinely costs less than the duplication.
