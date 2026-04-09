# General-Purpose OKD-on-Proxmox Installer

Design spec for making openshitctl work across diverse Proxmox environments, OS families, and architectures.

## Scope

12 findings from codebase audit, ordered by priority:

| ID | Summary | Category |
|----|---------|----------|
| B5 | VIP configurable | Config |
| B7 | CIDR validation fix | Config |
| B6 | Plumb TF advanced features | Terraform |
| F1 | Resource capacity warning in wizard | Wizard |
| B2 | Multi-arch support | Platform |
| B3 | Per-VM multi-node Proxmox | Terraform + Wizard |
| F3 | Insecure default to false | Config |
| F4 | CPU type configurable | Config + Terraform |
| F5 | Data disk optional for workers and masters | Config + Terraform |
| F7 | Addon prerequisite validation | Wizard |
| F2 | ens18 help text | Wizard |
| B1 | Full OS abstraction (RHEL + Debian) | Platform |

---

## B5: VIP Configurable

**Problem:** VIP hardcoded to `.10` last octet (`internal/utils/netutil/ip.go:10`). Collides with existing services on many networks.

**Changes:**

- Add `VIP string` to `BastionConfig` in `internal/config/cluster.go`
- Add VIP field to wizard networking step, pre-populated by deriving from static IP start (current logic as suggestion)
- If empty after wizard, derive automatically (backward compatibility)
- `DefaultVIPLastOctet` constant stays, used only for derivation fallback
- Plumb VIP through to: dnsmasq templates, haproxy config, kube-vip manifest
- Validator: VIP must be valid IPv4, within machine CIDR, not equal to gateway or bastion IP

## B7: CIDR Validation Fix

**Problem:** `ValidateIPRangeWithin24()` only checks last octet overflow, ignores actual CIDR bounds (`internal/utils/netutil/ip.go:24-42`).

**Changes:**

- Replace `ValidateIPRangeWithin24()` with `ValidateIPRangeInCIDR(startIP string, count int, cidr string) error`
- Use `net.ParseCIDR` to get network bounds, check start and end IPs are contained
- Update all callers in `internal/config/validators.go` and `internal/tui/wizard/steps/validators.go` to pass machine CIDR
- Delete old function
- Add test cases for /25, /16, and edge cases

## B6: Plumb Terraform Advanced Features

**Problem:** TF module supports additional_networks, NUMA, per-role disk sizes, but Go code doesn't pass them through (`internal/distribution/okd/setup/terraform.go`, `internal/distribution/okd/templates/terraform.tfvars.tmpl`).

**Config additions:**

```go
// ProxmoxConfig
AdditionalNetworks []AdditionalNetwork  // Bridge, Model, VLANTag

// ProxmoxConfig
NUMAEnabled bool  // default false

// AdditionalNetwork (new type)
type AdditionalNetwork struct {
    Bridge  string
    Model   string // default "virtio"
    VLANTag int    // 0 = no tag
}
```

**Changes:**

- Add fields to `TerraformVarsData` struct
- Update `terraform.tfvars.tmpl` to emit `additional_networks`, `numa_enabled`
- Per-role disk sizes already handled via `NodeConfig.Disk` — ensure tfvars maps `master_os_disk_size_gb` and `worker_os_disk_size_gb` separately
- TF module: add `numa { enabled = var.numa_enabled }` block to all VM resources
- Wizard: additional networks and NUMA toggle added to advanced step

## F1: Resource Capacity Warning

**Problem:** no resource summary in wizard review step. users can configure 100+ GB RAM without knowing total.

**Changes:**

In `internal/tui/wizard/steps/review.go`, add a resource summary block after settings display:

```
── resource summary ──────────────────────────────
  control plane:  3 x 4 vcpu, 12 gb ram   =  12 vcpu,  36 gb ram
  workers:        3 x 8 vcpu, 20 gb ram   =  24 vcpu,  60 gb ram
  bootstrap:      1 x 4 vcpu,  8 gb ram   =   4 vcpu,   8 gb ram
  data disks:     3 x 500 gb (workers)     =          1500 gb
  total:          40 vcpu, 104 gb ram, 1850 gb disk
```

Warning thresholds (yellow text, non-blocking):
- Total RAM > 64 GB: `"total ram exceeds 64 gb — verify your proxmox host has sufficient memory"`
- Total vCPU > 32: `"total vcpu exceeds 32 — verify your proxmox host has sufficient cores"`

Uses `ExtraContent` callback on the review step definition.

## B2: Multi-Arch Support

**Problem:** x86_64 hardcoded in tool downloads (`tools.go:173,180,188`), CoreOS detection (`coreos.go:116`), and install-config template.

**Changes:**

Architecture detection via `runtime.GOARCH`, mapped:

| GOARCH | download suffix | CoreOS key | OKD arch |
|--------|----------------|------------|----------|
| amd64 | `linux_amd64` / `linux-amd64` | `x86_64` | `amd64` |
| arm64 | `linux_arm64` / `linux-arm64` | `aarch64` | `arm64` |

- `tools.go`: each `binaryInstallSpec` builds URL using detected arch
- `coreos.go:116`: replace hardcoded `"x86_64"` with arch lookup
- `install-config.yaml.tmpl`: template `architecture` field from config
- Add `Architecture string` to config (auto-detected at startup, not user-configurable)

## B3: Per-VM Multi-Node Proxmox

**Problem:** all VMs deployed to single `var.target_node` (`main.tf:38,140,244`). no HA at infrastructure layer.

**Config:**

```go
type ProxmoxConfig struct {
    Node        string   // default node (required)
    MasterNodes []string // optional per-master node assignment (index-based)
    WorkerNodes []string // optional per-worker node assignment (index-based)
    // bootstrap always uses Node
}
```

**Terraform:**

- Add `var.master_target_nodes` (list of strings, default `[]`)
- Add `var.worker_target_nodes` (list of strings, default `[]`)
- Master resource: `node_name = length(var.master_target_nodes) > count.index ? var.master_target_nodes[count.index] : var.target_node`
- Same for workers. Bootstrap stays on `var.target_node`.

**Wizard — dedicated "node placement" step:**

Custom step (not DataDrivenStep) placed after proxmox + basics steps.

1. Reads config to get master/worker counts and default node
2. "available nodes" text field — comma-separated, pre-filled with default node
3. Per-VM Selector dropdowns populated from available nodes list
4. All fields default to primary node
5. `ShouldShow` returns false if only one node entered (zero friction for single-node users)

```
── available proxmox nodes ───────────────────────
  nodes:  pve1, pve2, pve3

── control plane placement ───────────────────────
  master-0:   [pve1 ▾]
  master-1:   [pve2 ▾]
  master-2:   [pve3 ▾]

── worker placement ──────────────────────────────
  worker-0:   [pve1 ▾]
  worker-1:   [pve2 ▾]
  worker-2:   [pve1 ▾]
```

Reuses `components.Selector` for each VM's node picker.

**Step registration:**

- Add `StepTypeNodePlacement` and `StepIDNodePlacement` constants
- Add to `DefaultConfig()` step order: welcome, distribution, proxmox, basics, **node-placement**, networking, resources, addons, files, advanced, review
- Register factory in `defaultStepRegistrations`

## F3: Insecure Default to False

**Problem:** `Insecure: true` in defaults (`defaults.go:23`). security risk for production use.

**Changes:**

- `defaults.go:23`: `Insecure: true` → `Insecure: false`
- Wizard proxmox step: flip default from `"yes"` to `"no"`
- Update help text: `"skip tls certificate verification — set to yes only for self-signed certs"`

## F4: CPU Type Configurable

**Problem:** `type = "host"` hardcoded in TF module (`main.tf:46,148,254`). prevents live migration between different CPU generations.

**Changes:**

- Add `CPUType string` to `ProxmoxConfig` (default `"host"`)
- Add to wizard advanced step: field with help text `"cpu type for vms — host gives best performance, x86-64-v2 or kvm64 allow live migration"`
- Add to `TerraformVarsData`, update tfvars template
- TF module: replace hardcoded `type = "host"` with `type = var.cpu_type`
- Add `var.cpu_type` with default `"host"` and validation for known types

## F5: Data Disk Optional (Workers and Masters)

**Problem:** workers always get 500 GB data disk (`main.tf:287-295`), masters never get one. no flexibility.

**Changes:**

- Split `DisksConfig.DataSizeGB` into:
  - `WorkerDataSizeGB int` (default `500`, `0` = no disk)
  - `MasterDataSizeGB int` (default `0`, `0` = no disk)
- Migration: existing configs with `DataSizeGB` map to `WorkerDataSizeGB` (keep old field as alias with deprecation comment, remove in next major)
- TF module: replace static worker data disk with `dynamic "disk"` block conditioned on `var.data_disk_size_gb > 0`
- Add matching `dynamic "disk"` block to master resource conditioned on `var.master_data_disk_size_gb > 0`
- Add `var.master_data_disk_size_gb` (default 0) to TF variables
- Wizard resources step: two toggle+size pairs — "attach data disk to workers?" / "attach data disk to masters?"
- Update tfvars template to emit both values

## F7: Addon Prerequisite Validation

**Problem:** addons (Flux, SecretStore) have prerequisites that aren't validated until deploy fails.

**Changes:**

Validate when addon is toggled ON in wizard addons step:

- **Flux**: `os.Stat("~/.ssh/flux-deploy-key")` — if missing, show inline warning: `"flux requires ssh deploy key at ~/.ssh/flux-deploy-key — create it before deploying"`
- **SecretStore**: `exec.LookPath("sops")` — if missing, show: `"secretstore requires sops — install before deploying"`

Warnings, not hard blocks. User can proceed (may create files before running deploy). Surfaced via `ExtraContent` callback or inline note below the toggle field.

## F2: ens18 Help Text

**Problem:** `ens18` default undocumented. users with different interface names don't know why it's wrong.

**Changes:**

- Wizard networking step: update interface field help to `"network interface inside vms — ens18 is the proxmox/virtio default; use ip link in a vm to verify"`

## B1: Full OS Abstraction

**Problem:** entire bastion setup hardcoded for RHEL/Fedora. dnf, rpm, /etc/httpd, httpd service, apache user — all fail on Debian/Ubuntu.

**New package:** `internal/utils/platform/`

### OS Detection

```go
// platform.go
type OS struct {
    Family  string // "rhel", "debian"
    ID      string // "fedora", "ubuntu", "rocky", "alma", "rhel", "debian"
    Version string // "39", "24.04"
}

func Detect() (OS, error) // parses /etc/os-release
```

Startup check: `platform.Detect()` runs at deploy start. Unrecognized OS fails with clear error: `"unsupported os: <id> — requires fedora, rocky, alma, rhel, ubuntu, or debian"`

### Package Manager Interface

```go
// packages.go
type PackageManager interface {
    Install(ctx context.Context, packages []string) error
    Remove(ctx context.Context, packages []string) error
    IsInstalled(pkg string) bool
    AddRepo(ctx context.Context, name, url string) error
}

func NewPackageManager(os OS) PackageManager
// returns *DNFManager{} or *APTManager{} based on os.Family
```

### DNFManager

Wraps current `dnf install -y` / `rpm -q` logic from `internal/distribution/okd/packages/packages.go`.

### APTManager

- `Install`: `apt-get install -y`
- `Remove`: `apt-get remove -y`
- `IsInstalled`: `dpkg -l <pkg>` exit code
- `AddRepo`: GPG key download + `/etc/apt/sources.list.d/` + `apt-get update`

### OS-Specific Path Map

| Component | RHEL/Fedora | Debian/Ubuntu |
|-----------|------------|---------------|
| terraform repo | `rpm.releases.hashicorp.com/RHEL/hashicorp.repo` | `apt.releases.hashicorp.com` GPG + sources.list |
| apache config | `/etc/httpd/conf/httpd.conf` | `/etc/apache2/apache2.conf` |
| apache service | `httpd` | `apache2` |
| apache package | `httpd` | `apache2` |
| apache user | `apache` | `www-data` |
| selinux | `semanage` (present) | skipped |

These are exposed via methods on `OS`:

```go
func (o OS) ApachePackageName() string  // "httpd" or "apache2"
func (o OS) ApacheConfigPath() string   // "/etc/httpd/conf/httpd.conf" or "/etc/apache2/apache2.conf"
func (o OS) ApacheServiceName() string  // "httpd" or "apache2"
func (o OS) ApacheUser() string         // "apache" or "www-data"
func (o OS) HasSELinux() bool
```

### Migration Path

- `internal/distribution/okd/packages/packages.go`: replace direct dnf/rpm calls with `PackageManager` interface. package manager instance passed in or resolved from detected OS.
- `setup/tools.go:installTerraform`: use `PackageManager.AddRepo()` + `PackageManager.Install()` instead of direct dnf calls
- `setup/apache.go`: use `OS.ApacheConfigPath()`, `OS.ApacheServiceName()`, `OS.ApacheUser()` instead of hardcoded values
- `dns/dnsmasq.go`: NetworkManager path stays (both distros have it). add `systemd-resolved` fallback for Ubuntu where NetworkManager may not be active.

---

## Constraints

- all wizard help text must be lowercase
- existing wizard patterns (DataDrivenStep, MultiFormStep, BaseStep) must be followed for new steps
- backward compatibility: existing configs without new fields must still work (zero-value defaults)
- no breaking changes to terraform state for existing deployments
