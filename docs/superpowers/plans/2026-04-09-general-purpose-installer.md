# General-Purpose OKD-on-Proxmox Installer — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make openshitctl work across diverse Proxmox environments, OS families, and architectures by fixing 12 audit findings.

**Architecture:** Config-first approach — each finding adds fields to `config.Config`, plumbs them through wizard steps, terraform templates, and runtime code. Changes are grouped so each task produces a compilable, testable commit.

**Tech Stack:** Go 1.24, bubbletea TUI, Terraform (bpg/proxmox provider), embedded templates

**Spec:** `docs/superpowers/specs/2026-04-09-general-purpose-installer-design.md`

---

## File Structure

### New files
- `internal/utils/platform/platform.go` — OS detection (`Detect()`, `OS` struct, service path methods)
- `internal/utils/platform/platform_test.go` — unit tests for OS detection parsing
- `internal/utils/platform/packages.go` — `PackageManager` interface + `DNFManager` + `APTManager`
- `internal/utils/platform/packages_test.go` — unit tests for package manager selection
- `internal/tui/wizard/steps/node_placement.go` — dedicated node placement wizard step

### Modified files (by task)
- `internal/config/cluster.go` — add VIP, multi-node, CPU type, data disk, arch, additional networks, NUMA fields
- `internal/config/defaults.go` — update defaults (insecure=false, new fields)
- `internal/config/validators.go` — add VIP validation, update IP range validation callers
- `internal/utils/netutil/ip.go` — replace `ValidateIPRangeWithin24` with `ValidateIPRangeInCIDR`
- `internal/utils/netutil/ip_test.go` — new test file for CIDR validation
- `internal/tui/wizard/steps/networking.go` — add VIP field, update ens18 help text
- `internal/tui/wizard/steps/proxmox.go` — flip insecure default
- `internal/tui/wizard/steps/advanced.go` — add CPU type, NUMA toggle fields
- `internal/tui/wizard/steps/resources.go` — split data disk into worker/master
- `internal/tui/wizard/steps/addons.go` — add prerequisite validation warnings
- `internal/tui/wizard/steps/review.go` — add resource summary + warnings
- `internal/tui/wizard/steps/defaults.go` — update defaults for new fields
- `internal/tui/wizard/step.go` — add StepIDNodePlacement
- `internal/tui/wizard/config.go` — add StepTypeNodePlacement, update step order
- `internal/cli/wizard_setup.go` — register node placement step
- `internal/distribution/okd/templates/templates.go` — extend TerraformVarsData, InstallConfigData
- `internal/distribution/okd/templates/terraform.tfvars.tmpl` — emit new fields
- `internal/distribution/okd/templates/install-config.yaml.tmpl` — template architecture field
- `internal/distribution/okd/setup/terraform.go` — plumb new config fields to template data
- `internal/distribution/okd/setup/tools.go` — multi-arch download URLs
- `internal/distribution/okd/setup/coreos.go` — multi-arch CoreOS detection
- `internal/distribution/okd/setup/apache.go` — use OS-specific paths
- `internal/distribution/okd/dns/dns.go` — use configured VIP instead of deriving
- `internal/distribution/okd/dns/dnsmasq.go` — add systemd-resolved fallback
- `internal/distribution/okd/setup/steps.go` — use configured VIP
- `internal/distribution/okd/postinstall/verify.go` — use configured VIP
- `internal/distribution/okd/postinstall/update_ingress.go` — use configured VIP
- `internal/distribution/okd/destroy/steps.go` — use configured VIP
- `internal/cli/summary.go` — use configured VIP
- `internal/distribution/okd/packages/packages.go` — use PackageManager interface
- `infrastructure/terraform/modules/proxmox-okd/main.tf` — dynamic data disk, NUMA, CPU type, per-node placement
- `infrastructure/terraform/modules/proxmox-okd/variables.tf` — add cpu_type, master_data_disk_size_gb, per-node vars

---

## Task 1: B5 — Make VIP Configurable

**Files:**
- Modify: `internal/config/cluster.go:62-65`
- Modify: `internal/config/defaults.go:60-62`
- Modify: `internal/config/validators.go:153-207`
- Modify: `internal/utils/netutil/ip.go:68-75`
- Modify: `internal/tui/wizard/steps/networking.go:123-137`
- Modify: `internal/tui/wizard/steps/defaults.go`
- Modify: `internal/tui/wizard/steps/review.go:193-228`
- Modify: `internal/distribution/okd/dns/dns.go:48-52`
- Modify: `internal/distribution/okd/setup/steps.go:162`
- Modify: `internal/distribution/okd/postinstall/verify.go:71`
- Modify: `internal/distribution/okd/postinstall/update_ingress.go:47`
- Modify: `internal/distribution/okd/destroy/steps.go:51`
- Modify: `internal/cli/summary.go:103`

- [ ] **Step 1: Add VIP field to config**

In `internal/config/cluster.go`, add `VIP` to `BastionConfig`:

```go
type BastionConfig struct {
	IP  string `yaml:"ip" json:"ip" mapstructure:"ip"`
	VIP string `yaml:"vip,omitempty" json:"vip,omitempty" mapstructure:"vip"`
}
```

- [ ] **Step 2: Add VIP resolution helper**

In `internal/utils/netutil/ip.go`, add a function that uses the configured VIP or falls back to derivation:

```go
// ResolveVIP returns the explicit VIP if set, otherwise derives one from the static IP start.
func ResolveVIP(explicitVIP, staticIPStart string) (string, error) {
	if explicitVIP != "" {
		if net.ParseIP(explicitVIP) == nil {
			return "", fmt.Errorf("invalid VIP address: %s", explicitVIP)
		}
		return explicitVIP, nil
	}
	return DeriveVIPFromStaticIP(staticIPStart)
}
```

- [ ] **Step 3: Update all 6 DeriveVIPFromStaticIP call sites**

Replace each `netutil.DeriveVIPFromStaticIP(cfg.Networking.StaticIP.Start)` with `netutil.ResolveVIP(cfg.Networking.Bastion.VIP, cfg.Networking.StaticIP.Start)` in:

1. `internal/distribution/okd/dns/dns.go:49`
2. `internal/distribution/okd/setup/steps.go:162`
3. `internal/distribution/okd/postinstall/verify.go:71`
4. `internal/distribution/okd/postinstall/update_ingress.go:47`
5. `internal/distribution/okd/destroy/steps.go:51`
6. `internal/cli/summary.go:103`

- [ ] **Step 4: Add VIP field to wizard networking step**

In `internal/tui/wizard/steps/networking.go`, add a VIP field to the "load balancing" section after `bastion_ip`:

```go
{
	Key:       "vip",
	Label:     "api vip",
	Default:   "",
	Help:      "virtual ip for kubernetes api — leave blank to auto-derive from static ip start",
	Validate: func(value string) error {
		if value == "" {
			return nil // optional, will be derived
		}
		return config.ValidateIP(value)
	},
	ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.Bastion.VIP = v }),
	ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.Bastion.VIP }),
},
```

- [ ] **Step 5: Add VIP validation to config validators**

In `internal/config/validators.go`, inside `validateAdvancedNetworking`, after the bastion IP checks (~line 164), add:

```go
if cfg.Networking.Bastion.VIP != "" {
	if !IsValidIP(cfg.Networking.Bastion.VIP) {
		result.AddError("networking.bastion.vip", "must be a valid IP address")
	} else {
		checkIPInCIDR(cfg.Networking.Bastion.VIP, machineCIDR, "networking.bastion.vip", "machine CIDR", result)
		if cfg.Networking.Bastion.VIP == gateway {
			result.AddError("networking.bastion.vip", "vip cannot be the same as the gateway")
		}
		if cfg.Networking.Bastion.VIP == bastionIP {
			result.AddError("networking.bastion.vip", "vip cannot be the same as the bastion ip")
		}
	}
}
```

- [ ] **Step 6: Add VIP to review step**

In `internal/tui/wizard/steps/review.go`, inside `renderNetworking`, after the bastion line (~line 208), add:

```go
if s.cfg.Networking.Bastion.VIP != "" {
	b.WriteString(st.kvPair("api vip", s.cfg.Networking.Bastion.VIP))
	b.WriteString("\n")
}
```

- [ ] **Step 7: Build and verify**

Run: `go build ./...`
Expected: Clean compilation

- [ ] **Step 8: Commit**

```
feat(config): make api vip configurable

add VIP field to BastionConfig with auto-derivation fallback.
all call sites now use ResolveVIP() which prefers explicit config
over the hardcoded .10 last-octet convention.
```

---

## Task 2: B7 — Fix CIDR Validation

**Files:**
- Modify: `internal/utils/netutil/ip.go:24-42`
- Create: `internal/utils/netutil/ip_test.go`
- Modify: `internal/distribution/okd/dns/dns.go:44`
- Modify: `internal/distribution/okd/setup/nodes.go` (grep for ValidateIPRangeWithin24)

- [ ] **Step 1: Write tests for the new validation function**

Create `internal/utils/netutil/ip_test.go`:

```go
package netutil

import "testing"

func TestValidateIPRangeInCIDR(t *testing.T) {
	tests := []struct {
		name    string
		startIP string
		count   int
		cidr    string
		wantErr bool
	}{
		{"fits in /24", "192.168.1.100", 10, "192.168.1.0/24", false},
		{"overflows /24", "192.168.1.250", 10, "192.168.1.0/24", true},
		{"fits in /16", "192.168.1.100", 300, "192.168.0.0/16", false},
		{"outside /25", "192.168.1.200", 5, "192.168.1.0/25", true},
		{"start outside cidr", "10.0.0.1", 1, "192.168.1.0/24", true},
		{"single ip fits", "192.168.1.1", 1, "192.168.1.0/24", false},
		{"zero count", "192.168.1.1", 0, "192.168.1.0/24", true},
		{"negative count", "192.168.1.1", -1, "192.168.1.0/24", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIPRangeInCIDR(tt.startIP, tt.count, tt.cidr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIPRangeInCIDR(%q, %d, %q) error = %v, wantErr %v",
					tt.startIP, tt.count, tt.cidr, err, tt.wantErr)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/utils/netutil/ -run TestValidateIPRangeInCIDR -v`
Expected: FAIL — `ValidateIPRangeInCIDR` undefined

- [ ] **Step 3: Implement ValidateIPRangeInCIDR**

In `internal/utils/netutil/ip.go`, replace `ValidateIPRangeWithin24` with:

```go
// ValidateIPRangeInCIDR checks that startIP through startIP+count-1 all
// fall within the given CIDR. Replaces the old /24-only check.
func ValidateIPRangeInCIDR(startIP string, count int, cidr string) error {
	if count <= 0 {
		return fmt.Errorf("count must be positive: %d", count)
	}

	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}

	start := net.ParseIP(startIP)
	if start == nil || start.To4() == nil {
		return fmt.Errorf("invalid IPv4 address: %s", startIP)
	}

	if !network.Contains(start) {
		return fmt.Errorf("start IP %s is not within CIDR %s", startIP, cidr)
	}

	// Check the last IP in the range
	endIP, err := CalculateVMIP(startIP, count-1)
	if err != nil {
		return fmt.Errorf("failed to calculate end of range: %w", err)
	}

	end := net.ParseIP(endIP)
	if !network.Contains(end) {
		return fmt.Errorf("IP range %s + %d addresses exceeds CIDR %s (last IP would be %s)", startIP, count, cidr, endIP)
	}

	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/utils/netutil/ -run TestValidateIPRangeInCIDR -v`
Expected: PASS

- [ ] **Step 5: Update callers**

In `internal/distribution/okd/dns/dns.go:44`, replace:
```go
if err := netutil.ValidateIPRangeWithin24(staticIPStart, totalNodes); err != nil {
```
with:
```go
if err := netutil.ValidateIPRangeInCIDR(staticIPStart, totalNodes, cfg.Networking.MachineCIDR); err != nil {
```

Find and update the other caller in `internal/distribution/okd/setup/nodes.go` similarly — it needs the machine CIDR passed in from the config.

- [ ] **Step 6: Delete old function**

Remove `ValidateIPRangeWithin24` from `internal/utils/netutil/ip.go` (lines 24-42).

- [ ] **Step 7: Build and verify**

Run: `go build ./... && go test ./internal/utils/netutil/ -v`
Expected: Clean build, all tests pass

- [ ] **Step 8: Commit**

```
fix(netutil): validate ip range against actual cidr, not hardcoded /24

ValidateIPRangeInCIDR replaces ValidateIPRangeWithin24. checks both
start and end IPs are within the configured machine cidr, supporting
/25, /16, and other non-/24 networks.
```

---

## Task 3: F3 + F2 — Insecure Default + ens18 Help Text

**Files:**
- Modify: `internal/config/defaults.go:23`
- Modify: `internal/tui/wizard/steps/proxmox.go:119-122`
- Modify: `internal/tui/wizard/steps/networking.go:114-116`

- [ ] **Step 1: Flip insecure default**

In `internal/config/defaults.go:23`, change:
```go
Insecure:    true,
```
to:
```go
Insecure:    false,
```

- [ ] **Step 2: Flip wizard default and update help text**

In `internal/tui/wizard/steps/proxmox.go:121-122`, change:
```go
Default:  "yes",
Help:     "skip tls certificate verification (yes for self-signed certs)",
```
to:
```go
Default:  "no",
Help:     "skip tls certificate verification — set to yes only for self-signed certs",
```

- [ ] **Step 3: Update ens18 help text**

In `internal/tui/wizard/steps/networking.go:116`, change:
```go
Help:      "network interface name on vms",
```
to:
```go
Help:      "network interface inside vms — ens18 is the proxmox/virtio default; use ip link in a vm to verify",
```

- [ ] **Step 4: Build and verify**

Run: `go build ./...`
Expected: Clean compilation

- [ ] **Step 5: Commit**

```
fix(config): default insecure to false, improve help text

flip tls verification default to secure. update ens18 help text
to explain it's proxmox-specific and how to verify.
```

---

## Task 4: F4 — CPU Type Configurable

**Files:**
- Modify: `internal/config/cluster.go:77-92`
- Modify: `internal/config/defaults.go:14-24`
- Modify: `internal/tui/wizard/steps/advanced.go:11-65`
- Modify: `internal/distribution/okd/templates/templates.go:31-55`
- Modify: `internal/distribution/okd/templates/terraform.tfvars.tmpl`
- Modify: `internal/distribution/okd/setup/terraform.go:58-93`
- Modify: `infrastructure/terraform/modules/proxmox-okd/variables.tf`
- Modify: `infrastructure/terraform/modules/proxmox-okd/main.tf:46,148,254`

- [ ] **Step 1: Add CPUType to ProxmoxConfig**

In `internal/config/cluster.go`, add to `ProxmoxConfig` after the `Insecure` field:

```go
CPUType  string `yaml:"cpu_type,omitempty" json:"cpuType,omitempty" mapstructure:"cpu_type"`
```

- [ ] **Step 2: Set default**

In `internal/config/defaults.go`, add `CPUType: "host"` to the Proxmox config block (after `Insecure: false`).

- [ ] **Step 3: Add wizard field**

In `internal/tui/wizard/steps/advanced.go`, add a new section after "installation timeouts":

```go
{
	Title: "proxmox vm settings",
	Fields: []wizard.FieldDefinition{
		{
			Key:     "cpu_type",
			Label:   "cpu type",
			Default: "host",
			Help:    "cpu type for vms — host gives best performance, x86-64-v2 or kvm64 allow live migration",
			Required: true,
			ConfigSet: func(cfg *config.Config, value string) error {
				if cfg.Provider.Proxmox != nil {
					cfg.Provider.Proxmox.CPUType = value
				}
				return nil
			},
			ConfigGet: func(cfg *config.Config) string {
				if cfg.Provider.Proxmox != nil && cfg.Provider.Proxmox.CPUType != "" {
					return cfg.Provider.Proxmox.CPUType
				}
				return "host"
			},
		},
	},
},
```

- [ ] **Step 4: Add to TerraformVarsData**

In `internal/distribution/okd/templates/templates.go`, add to `TerraformVarsData`:

```go
CPUType string
```

- [ ] **Step 5: Plumb through terraform.go**

In `internal/distribution/okd/setup/terraform.go`, inside `buildTerraformVarsData`, add to the return struct:

```go
CPUType: proxmox.CPUType,
```

If empty, default to `"host"`:
```go
cpuType := proxmox.CPUType
if cpuType == "" {
	cpuType = "host"
}
```

- [ ] **Step 6: Update tfvars template**

In `internal/distribution/okd/templates/terraform.tfvars.tmpl`, add after the `numa_enabled` line:

```
cpu_type = "{{ .CPUType }}"
```

- [ ] **Step 7: Add TF variable**

In `infrastructure/terraform/modules/proxmox-okd/variables.tf`, add in the optional section:

```hcl
variable "cpu_type" {
  description = "cpu type for vms (host, x86-64-v2, x86-64-v3, kvm64)"
  type        = string
  default     = "host"
}
```

- [ ] **Step 8: Update TF main.tf**

In `infrastructure/terraform/modules/proxmox-okd/main.tf`, replace `type = "host"` with `type = var.cpu_type` in all three CPU blocks (bootstrap line 46, master line 148, worker line 254).

- [ ] **Step 9: Build and verify**

Run: `go build ./...`
Expected: Clean compilation

- [ ] **Step 10: Commit**

```
feat(proxmox): make cpu type configurable

add cpu_type to ProxmoxConfig, wizard advanced step, and terraform
module. defaults to "host" for best performance; x86-64-v2/kvm64
options enable live migration between different cpu generations.
```

---

## Task 5: F5 — Data Disk Optional for Workers and Masters

**Files:**
- Modify: `internal/config/cluster.go:114-116`
- Modify: `internal/config/defaults.go:89-91`
- Modify: `internal/tui/wizard/steps/resources.go:97-112`
- Modify: `internal/tui/wizard/steps/review.go:257-261`
- Modify: `internal/distribution/okd/templates/templates.go:31-55`
- Modify: `internal/distribution/okd/templates/terraform.tfvars.tmpl`
- Modify: `internal/distribution/okd/setup/terraform.go:30-43,58-93`
- Modify: `infrastructure/terraform/modules/proxmox-okd/variables.tf`
- Modify: `infrastructure/terraform/modules/proxmox-okd/main.tf:287-295`

- [ ] **Step 1: Split DisksConfig**

In `internal/config/cluster.go`, replace `DisksConfig`:

```go
type DisksConfig struct {
	WorkerDataSizeGB int `yaml:"worker_data_size_gb" json:"workerDataSizeGb" mapstructure:"worker_data_size_gb"`
	MasterDataSizeGB int `yaml:"master_data_size_gb" json:"masterDataSizeGb" mapstructure:"master_data_size_gb"`
	// Deprecated: use WorkerDataSizeGB. Kept for config migration.
	DataSizeGB int `yaml:"data_size_gb,omitempty" json:"dataSizeGb,omitempty" mapstructure:"data_size_gb"`
}
```

- [ ] **Step 2: Update defaults**

In `internal/config/defaults.go`, replace the Disks block:

```go
Disks: DisksConfig{
	WorkerDataSizeGB: 500,
	MasterDataSizeGB: 0,
},
```

- [ ] **Step 3: Add migration logic**

In `internal/distribution/okd/setup/terraform.go`, update `getDiskSizes` to handle migration:

```go
func getDiskSizes(cfg *config.Config) (cpDisk, workerDisk, workerDataDisk, masterDataDisk int) {
	cpDisk = cfg.Topology.ControlPlane.Disk
	if cpDisk == 0 {
		cpDisk = 50
	}
	workerDisk = cfg.Topology.Workers.Disk
	if workerDisk == 0 {
		workerDisk = cpDisk
	}
	workerDataDisk = cfg.Disks.WorkerDataSizeGB
	if workerDataDisk == 0 && cfg.Disks.DataSizeGB > 0 {
		workerDataDisk = cfg.Disks.DataSizeGB // migration from old field
	}
	masterDataDisk = cfg.Disks.MasterDataSizeGB
	return cpDisk, workerDisk, workerDataDisk, masterDataDisk
}
```

- [ ] **Step 4: Add MasterDataDiskSizeGB to TerraformVarsData**

In `internal/distribution/okd/templates/templates.go`, add:

```go
MasterDataDiskSizeGB int
```

And rename `DataDiskSizeGB` to `WorkerDataDiskSizeGB` for clarity (update template references too).

- [ ] **Step 5: Update buildTerraformVarsData**

Update the function to pass both disk sizes through.

- [ ] **Step 6: Update tfvars template**

Replace `data_disk_size_gb = {{ .DataDiskSizeGB }}` with:

```
worker_data_disk_size_gb = {{ .WorkerDataDiskSizeGB }}
master_data_disk_size_gb = {{ .MasterDataDiskSizeGB }}
```

- [ ] **Step 7: Add TF variable for master data disk**

In `variables.tf`, add:

```hcl
variable "master_data_disk_size_gb" {
  description = "size of data disk for master nodes (0 = no data disk)"
  type        = number
  default     = 0
}

variable "worker_data_disk_size_gb" {
  description = "size of data disk for worker nodes (0 = no data disk)"
  type        = number
  default     = 500
}
```

Update existing `data_disk_size_gb` validation to allow 0.

- [ ] **Step 8: Make worker data disk dynamic in main.tf**

Replace the static worker data disk block (lines 287-295) with:

```hcl
dynamic "disk" {
  for_each = var.worker_data_disk_size_gb > 0 ? [1] : []
  content {
    datastore_id = var.data_storage
    size         = var.worker_data_disk_size_gb
    interface    = "scsi1"
    iothread     = true
    ssd          = false
    discard      = "on"
    serial       = "CEPH-DATA"
  }
}
```

- [ ] **Step 9: Add master data disk dynamic block**

In the master resource block, after the OS disk, add:

```hcl
dynamic "disk" {
  for_each = var.master_data_disk_size_gb > 0 ? [1] : []
  content {
    datastore_id = var.data_storage
    size         = var.master_data_disk_size_gb
    interface    = "scsi1"
    iothread     = true
    ssd          = false
    discard      = "on"
    serial       = "CEPH-DATA"
  }
}
```

- [ ] **Step 10: Update wizard resources step**

In `internal/tui/wizard/steps/resources.go`, replace the storage section with two fields:

```go
{
	Title: "data storage",
	Fields: []wizard.FieldDefinition{
		{
			Key:       "worker_data_disk",
			Label:     "worker data disk (gb)",
			Default:   "500",
			Help:      "data disk per worker for ceph/storage — set to 0 to disable",
			Required:  true,
			Validate:  config.ValidateIntRange(" (in gb)", 0, 5000),
			ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Disks.WorkerDataSizeGB = v }),
			ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Disks.WorkerDataSizeGB }),
		},
		{
			Key:       "master_data_disk",
			Label:     "master data disk (gb)",
			Default:   "0",
			Help:      "data disk per master for ceph/storage — set to 0 to disable",
			Required:  true,
			Validate:  config.ValidateIntRange(" (in gb)", 0, 5000),
			ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Disks.MasterDataSizeGB = v }),
			ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Disks.MasterDataSizeGB }),
		},
	},
},
```

- [ ] **Step 11: Update review step renderCompute**

Update `internal/tui/wizard/steps/review.go` to show both worker and master data disks.

- [ ] **Step 12: Update renderResourceSummary in resources.go**

Update the summary in `renderResourceSummary` to use `worker_data_disk` and `master_data_disk` field names and compute both totals.

- [ ] **Step 13: Build and verify**

Run: `go build ./...`
Expected: Clean compilation

- [ ] **Step 14: Commit**

```
feat(disks): make data disk optional for workers and masters

split data_size_gb into worker_data_size_gb and master_data_size_gb.
setting either to 0 disables the data disk for that role. terraform
uses dynamic blocks. old config field is migrated automatically.
```

---

## Task 6: B6 — Plumb Terraform Advanced Features (NUMA + Additional Networks)

**Files:**
- Modify: `internal/config/cluster.go:77-92`
- Modify: `internal/config/defaults.go:14-24`
- Modify: `internal/tui/wizard/steps/advanced.go`
- Modify: `internal/distribution/okd/templates/templates.go:31-55`
- Modify: `internal/distribution/okd/templates/terraform.tfvars.tmpl`
- Modify: `internal/distribution/okd/setup/terraform.go:58-93`
- Modify: `infrastructure/terraform/modules/proxmox-okd/main.tf`

- [ ] **Step 1: Add config types**

In `internal/config/cluster.go`, add the `AdditionalNetwork` type and new fields to `ProxmoxConfig`:

```go
type AdditionalNetwork struct {
	Bridge  string `yaml:"bridge" json:"bridge" mapstructure:"bridge"`
	Model   string `yaml:"model,omitempty" json:"model,omitempty" mapstructure:"model"`
	VLANTag int    `yaml:"vlan_tag,omitempty" json:"vlanTag,omitempty" mapstructure:"vlan_tag"`
}
```

Add to `ProxmoxConfig`:
```go
AdditionalNetworks []AdditionalNetwork `yaml:"additional_networks,omitempty" json:"additionalNetworks,omitempty" mapstructure:"additional_networks"`
NUMAEnabled        bool                `yaml:"numa_enabled,omitempty" json:"numaEnabled,omitempty" mapstructure:"numa_enabled"`
```

- [ ] **Step 2: Add NUMA toggle to wizard advanced step**

In `internal/tui/wizard/steps/advanced.go`, add a field to the "proxmox vm settings" section (created in Task 4):

```go
{
	Key:     "numa_enabled",
	Label:   "enable numa",
	Default: "no",
	Help:    "enable numa topology for vms — improves performance on multi-socket hosts",
	Required: true,
	Validate: ValidateYesNo,
	ConfigSet: func(cfg *config.Config, value string) error {
		if cfg.Provider.Proxmox != nil {
			cfg.Provider.Proxmox.NUMAEnabled = value == "yes"
		}
		return nil
	},
	ConfigGet: func(cfg *config.Config) string {
		if cfg.Provider.Proxmox != nil && cfg.Provider.Proxmox.NUMAEnabled {
			return "yes"
		}
		return "no"
	},
},
```

- [ ] **Step 3: Add to TerraformVarsData**

In `templates.go`, add:
```go
NUMAEnabled          bool
AdditionalNetworks   string // JSON-encoded for HCL
```

- [ ] **Step 4: Update buildTerraformVarsData**

In `terraform.go`, add to the return struct:
```go
NUMAEnabled:        proxmox.NUMAEnabled,
AdditionalNetworks: formatAdditionalNetworks(proxmox.AdditionalNetworks),
```

Add the formatting helper:
```go
func formatAdditionalNetworks(networks []config.AdditionalNetwork) string {
	if len(networks) == 0 {
		return "[]"
	}
	var parts []string
	for _, n := range networks {
		model := n.Model
		if model == "" {
			model = "virtio"
		}
		entry := fmt.Sprintf(`{ model = %q, bridge = %q`, model, n.Bridge)
		if n.VLANTag > 0 {
			entry += fmt.Sprintf(`, tag = %d`, n.VLANTag)
		}
		entry += " }"
		parts = append(parts, entry)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
```

- [ ] **Step 5: Update tfvars template**

Replace the hardcoded lines:
```
additional_networks = []

numa_enabled = false
```
with:
```
additional_networks = {{ .AdditionalNetworks }}

numa_enabled = {{ .NUMAEnabled }}
```

- [ ] **Step 6: Add NUMA block to TF main.tf**

In `infrastructure/terraform/modules/proxmox-okd/main.tf`, add `numa {}` block inside each VM resource (bootstrap, master, worker), after the `memory {}` block:

```hcl
  numa {
    enabled = var.numa_enabled
  }
```

- [ ] **Step 7: Build and verify**

Run: `go build ./...`
Expected: Clean compilation

- [ ] **Step 8: Commit**

```
feat(proxmox): plumb numa and additional networks to terraform

pass numa_enabled and additional_networks from config through
to terraform instead of hardcoding false/[]. numa block added
to all vm resources in terraform module.
```

---

## Task 7: F1 — Resource Capacity Warning in Review

**Files:**
- Modify: `internal/tui/wizard/steps/review.go:111-134,230-266`

- [ ] **Step 1: Add resource summary with warnings to renderCompute**

In `internal/tui/wizard/steps/review.go`, extend `renderCompute` to add totals and warnings after the per-role details. Add at the end of the function, before the final `"\n"`:

```go
// Resource totals
totalCPU := cpCPU*cpCount + 4 // +4 for bootstrap
totalMemGB := (cpMem*cpCount + 8192) / 1024 // +8192 for bootstrap
totalOSDiskGB := cpDisk*cpCount + 50 // +50 for bootstrap
totalDataDiskGB := 0

wCount := 0
if s.cfg.Topology.Workers.Count > 0 {
	wCount = s.cfg.Topology.Workers.Count
	wCPU := s.cfg.Topology.Workers.CPU
	wMem := s.cfg.Topology.Workers.Memory
	wDisk := s.cfg.Topology.Workers.Disk
	totalCPU += wCPU * wCount
	totalMemGB += (wMem * wCount) / 1024
	totalOSDiskGB += wDisk * wCount
}

if s.cfg.Disks.WorkerDataSizeGB > 0 {
	totalDataDiskGB += s.cfg.Disks.WorkerDataSizeGB * wCount
}
if s.cfg.Disks.MasterDataSizeGB > 0 {
	totalDataDiskGB += s.cfg.Disks.MasterDataSizeGB * cpCount
}

b.WriteString(st.separator)
b.WriteString("\n")
totalSpec := fmt.Sprintf("%d vcpu, %d gb ram, %d gb disk", totalCPU, totalMemGB, totalOSDiskGB+totalDataDiskGB)
b.WriteString(st.kvPair("total", totalSpec))
b.WriteString("\n")

warnStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning)
if totalMemGB > 64 {
	b.WriteString(warnStyle.Render("  total ram exceeds 64 gb — verify your proxmox host has sufficient memory"))
	b.WriteString("\n")
}
if totalCPU > 32 {
	b.WriteString(warnStyle.Render("  total vcpu exceeds 32 — verify your proxmox host has sufficient cores"))
	b.WriteString("\n")
}
```

- [ ] **Step 2: Build and verify**

Run: `go build ./...`
Expected: Clean compilation

- [ ] **Step 3: Commit**

```
feat(wizard): add resource capacity summary and warnings to review

show total vcpu, ram, and disk in the review step compute section.
warn when totals exceed 32 vcpu or 64 gb ram.
```

---

## Task 8: F7 — Addon Prerequisite Validation

**Files:**
- Modify: `internal/tui/wizard/steps/addons.go:54-125`

- [ ] **Step 1: Add ExtraContent with prerequisite warnings**

In `internal/tui/wizard/steps/addons.go`, update `NewAddonsStep` to add an ExtraContent callback:

```go
func NewAddonsStep() (*wizard.DataDrivenStep, *wizard.DataDrivenStep) {
	step := wizard.NewDataDrivenStep(AddonsStepDefinition)

	step.WithExtraContentFunc(func(s *wizard.DataDrivenStep, width int) string {
		return renderAddonWarnings(s)
	})

	return step, step
}

func renderAddonWarnings(step *wizard.DataDrivenStep) string {
	warnStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning)
	var warnings []string

	if step.Value("flux_enabled") == "yes" {
		home, _ := os.UserHomeDir()
		keyPath := filepath.Join(home, ".ssh", "flux-deploy-key")
		if _, err := os.Stat(keyPath); os.IsNotExist(err) {
			warnings = append(warnings, warnStyle.Render("  flux requires ssh deploy key at ~/.ssh/flux-deploy-key — create it before deploying"))
		}
	}

	if step.Value("secretstore_enabled") == "yes" {
		if _, err := exec.LookPath("sops"); err != nil {
			warnings = append(warnings, warnStyle.Render("  secretstore requires sops — install before deploying"))
		}
	}

	if len(warnings) == 0 {
		return ""
	}
	return "\n" + strings.Join(warnings, "\n")
}
```

Add imports: `"os"`, `"os/exec"`, `"path/filepath"`, `"strings"`, `"github.com/charmbracelet/lipgloss"`, and the `tui` package.

- [ ] **Step 2: Build and verify**

Run: `go build ./...`
Expected: Clean compilation

- [ ] **Step 3: Commit**

```
feat(wizard): add addon prerequisite warnings

show inline warnings in addons step when flux deploy key or sops
binary is missing. warnings are non-blocking — user can proceed
and install prerequisites before deploying.
```

---

## Task 9: B2 — Multi-Arch Support

**Files:**
- Modify: `internal/distribution/okd/setup/tools.go:170-190`
- Modify: `internal/distribution/okd/setup/coreos.go:116-132`
- Modify: `internal/distribution/okd/templates/templates.go:18-29`
- Modify: `internal/distribution/okd/templates/install-config.yaml.tmpl`

- [ ] **Step 1: Add arch helper to tools.go**

At the top of `internal/distribution/okd/setup/tools.go`, add:

```go
import "runtime"

func downloadArch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "arm64"
	default:
		return "amd64"
	}
}

// coreOSArch maps GOARCH to the CoreOS stream architecture key.
func coreOSArch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "aarch64"
	default:
		return "x86_64"
	}
}
```

- [ ] **Step 2: Update tool download URLs**

Replace the three install functions:

```go
func (p *Phase) installYQ(ctx context.Context) error {
	arch := downloadArch()
	return p.installBinary(ctx, binaryInstallSpec{
		name: "yq", versionFlag: "--version",
		url: fmt.Sprintf("https://github.com/mikefarah/yq/releases/latest/download/yq_linux_%s", arch),
	})
}

func (p *Phase) installHelm(ctx context.Context) error {
	arch := downloadArch()
	return p.installBinary(ctx, binaryInstallSpec{
		name: "helm", versionFlag: "version",
		url: fmt.Sprintf("https://get.helm.sh/helm-v3.17.3-linux-%s.tar.gz", arch),
		archiveBinary: "helm", stripComponents: 1,
	})
}

func (p *Phase) installSops(ctx context.Context) error {
	arch := downloadArch()
	return p.installBinary(ctx, binaryInstallSpec{
		name: "sops", versionFlag: "--version",
		url: fmt.Sprintf("https://github.com/getsops/sops/releases/download/v3.9.4/sops-v3.9.4.linux.%s", arch),
	})
}
```

- [ ] **Step 3: Update CoreOS detection**

In `internal/distribution/okd/setup/coreos.go:116-132`, replace:

```go
arch, ok := streamData.Architectures["x86_64"]
if !ok {
	return nil, fmt.Errorf("x86_64 architecture not found in CoreOS stream")
}
```

with:

```go
archKey := coreOSArch()
arch, ok := streamData.Architectures[archKey]
if !ok {
	return nil, fmt.Errorf("%s architecture not found in CoreOS stream", archKey)
}
```

And replace `Architecture: "x86_64"` with `Architecture: archKey`.

- [ ] **Step 4: Template architecture in install-config**

In `internal/distribution/okd/templates/templates.go`, add `Architecture string` to `InstallConfigData`.

In `internal/distribution/okd/templates/install-config.yaml.tmpl`, replace both `architecture: amd64` with `architecture: {{ .Architecture }}`.

Update all callers that construct `InstallConfigData` to pass `Architecture: runtime.GOARCH` (which is already `"amd64"` or `"arm64"`).

- [ ] **Step 5: Build and verify**

Run: `go build ./...`
Expected: Clean compilation

- [ ] **Step 6: Commit**

```
feat(arch): detect runtime architecture for tool downloads and coreos

use runtime.GOARCH to select correct download URLs for yq, helm,
sops, and coreos iso. install-config.yaml architecture field is
now templated instead of hardcoded to amd64.
```

---

## Task 10: B3 — Per-VM Multi-Node Proxmox

**Files:**
- Modify: `internal/config/cluster.go:77-92`
- Modify: `internal/config/defaults.go`
- Modify: `internal/config/validators.go`
- Create: `internal/tui/wizard/steps/node_placement.go`
- Modify: `internal/tui/wizard/step.go:14-27`
- Modify: `internal/tui/wizard/config.go:24-52`
- Modify: `internal/cli/wizard_setup.go:40-51`
- Modify: `internal/distribution/okd/templates/templates.go:31-55`
- Modify: `internal/distribution/okd/templates/terraform.tfvars.tmpl`
- Modify: `internal/distribution/okd/setup/terraform.go:58-93`
- Modify: `infrastructure/terraform/modules/proxmox-okd/variables.tf`
- Modify: `infrastructure/terraform/modules/proxmox-okd/main.tf:38,140,244`

- [ ] **Step 1: Add multi-node fields to ProxmoxConfig**

In `internal/config/cluster.go`, add:

```go
MasterNodes []string `yaml:"master_nodes,omitempty" json:"masterNodes,omitempty" mapstructure:"master_nodes"`
WorkerNodes []string `yaml:"worker_nodes,omitempty" json:"workerNodes,omitempty" mapstructure:"worker_nodes"`
```

- [ ] **Step 2: Add TF variables**

In `infrastructure/terraform/modules/proxmox-okd/variables.tf`, add:

```hcl
variable "master_target_nodes" {
  description = "per-master proxmox node assignment (index-based, falls back to target_node)"
  type        = list(string)
  default     = []
}

variable "worker_target_nodes" {
  description = "per-worker proxmox node assignment (index-based, falls back to target_node)"
  type        = list(string)
  default     = []
}
```

- [ ] **Step 3: Update TF main.tf node_name**

In `main.tf`, update the master resource `node_name`:

```hcl
node_name = length(var.master_target_nodes) > count.index ? var.master_target_nodes[count.index] : var.target_node
```

Same for workers:
```hcl
node_name = length(var.worker_target_nodes) > count.index ? var.worker_target_nodes[count.index] : var.target_node
```

Bootstrap stays as `node_name = var.target_node`.

- [ ] **Step 4: Add to TerraformVarsData and template**

In `templates.go`, add:
```go
MasterTargetNodes string
WorkerTargetNodes string
```

In `terraform.go`, format the node lists:
```go
MasterTargetNodes: formatStringList(proxmox.MasterNodes),
WorkerTargetNodes: formatStringList(proxmox.WorkerNodes),
```

Add helper:
```go
func formatStringList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("%q", item)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
```

In `terraform.tfvars.tmpl`, add:
```
master_target_nodes = {{ .MasterTargetNodes }}
worker_target_nodes = {{ .WorkerTargetNodes }}
```

- [ ] **Step 5: Register step constants**

In `internal/tui/wizard/step.go`, add:
```go
StepIDNodePlacement StepID = "node-placement"
```

In `internal/tui/wizard/config.go`, add:
```go
StepTypeNodePlacement StepType = "node-placement"
```

Update `DefaultConfig()` step order to insert after basics:
```go
{Type: StepTypeBasics, Required: true},
{Type: StepTypeNodePlacement, Required: false},
{Type: StepTypeNetworking, Required: true},
```

- [ ] **Step 6: Create node placement step**

Create `internal/tui/wizard/steps/node_placement.go`. This is a custom step (not DataDrivenStep) that:

1. Implements `wizard.WizardStep`, `wizard.ConfigApplier`, `wizard.ConditionalStep`, `wizard.FocusableStep`, `wizard.ResizableStep`, `wizard.HelpProvider`, `wizard.DescribedStep`
2. Reads config to get master/worker counts and default node from `cfg.Provider.Proxmox.Node`
3. Has an "available nodes" text field (comma-separated, pre-filled with default node)
4. Has per-VM selector fields populated from the available nodes list
5. `ShouldShow` returns `false` if only one unique node is available
6. On `Apply`, writes `MasterNodes` and `WorkerNodes` to config

The implementation should follow the pattern of the DistributionStep — custom bubbletea component using `components.Selector` for each VM's node assignment.

Key structure:

```go
type NodePlacementStep struct {
	wizard.BaseStep
	cfg          *config.Config
	nodesInput   string // comma-separated available nodes
	masterNodes  []string
	workerNodes  []string
	focusIndex   int
	editing      bool
}
```

This is the most complex new step. The full implementation will need:
- Text input for available nodes
- Dynamic list of selectors based on master/worker count
- Navigation between fields (up/down/tab)
- View rendering with sections for control plane and worker placement

- [ ] **Step 7: Register in wizard_setup.go**

In `internal/cli/wizard_setup.go`, add to `defaultStepRegistrations`:

```go
{wizard.StepTypeNodePlacement, func() (wizard.WizardStep, any) { return NewNodePlacementStep(), nil }},
```

Place it after the basics registration and before networking.

- [ ] **Step 8: Validate node names**

In `internal/config/validators.go`, add validation for node names in MasterNodes/WorkerNodes:

```go
for i, node := range proxmox.MasterNodes {
	if node != "" && !proxmoxNamePattern.MatchString(node) {
		result.AddError(fmt.Sprintf("proxmox.master_nodes[%d]", i), "must be a valid Proxmox node name")
	}
}
for i, node := range proxmox.WorkerNodes {
	if node != "" && !proxmoxNamePattern.MatchString(node) {
		result.AddError(fmt.Sprintf("proxmox.worker_nodes[%d]", i), "must be a valid Proxmox node name")
	}
}
```

- [ ] **Step 9: Build and verify**

Run: `go build ./...`
Expected: Clean compilation

- [ ] **Step 10: Commit**

```
feat(proxmox): per-vm multi-node placement

add MasterNodes/WorkerNodes to ProxmoxConfig for per-vm proxmox
node assignment. dedicated wizard step with per-vm selectors.
terraform falls back to target_node for unspecified vms.
```

---

## Task 11: B1 — Full OS Abstraction (Platform Package)

**Files:**
- Create: `internal/utils/platform/platform.go`
- Create: `internal/utils/platform/platform_test.go`
- Create: `internal/utils/platform/packages.go`
- Create: `internal/utils/platform/packages_test.go`

- [ ] **Step 1: Write platform detection tests**

Create `internal/utils/platform/platform_test.go`:

```go
package platform

import "testing"

func TestParseOSRelease(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    OS
	}{
		{
			"fedora",
			"ID=fedora\nVERSION_ID=39\nID_LIKE=\"rhel centos fedora\"\n",
			OS{Family: "rhel", ID: "fedora", Version: "39"},
		},
		{
			"ubuntu",
			"ID=ubuntu\nVERSION_ID=\"24.04\"\nID_LIKE=debian\n",
			OS{Family: "debian", ID: "ubuntu", Version: "24.04"},
		},
		{
			"rocky",
			"ID=\"rocky\"\nVERSION_ID=\"9.3\"\nID_LIKE=\"rhel centos fedora\"\n",
			OS{Family: "rhel", ID: "rocky", Version: "9.3"},
		},
		{
			"debian",
			"ID=debian\nVERSION_ID=\"12\"\n",
			OS{Family: "debian", ID: "debian", Version: "12"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOSRelease(tt.content)
			if err != nil {
				t.Fatalf("parseOSRelease() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("parseOSRelease() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/utils/platform/ -run TestParseOSRelease -v`
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Implement OS detection**

Create `internal/utils/platform/platform.go`:

```go
package platform

import (
	"fmt"
	"os"
	"strings"
)

type OS struct {
	Family  string // "rhel", "debian"
	ID      string // "fedora", "ubuntu", "rocky", "alma", "rhel", "debian"
	Version string // "39", "24.04"
}

var rhelIDs = map[string]bool{
	"fedora": true, "rhel": true, "rocky": true, "alma": true, "centos": true,
}

var debianIDs = map[string]bool{
	"debian": true, "ubuntu": true,
}

func Detect() (OS, error) {
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return OS{}, fmt.Errorf("cannot read /etc/os-release: %w", err)
	}
	return parseOSRelease(string(content))
}

func parseOSRelease(content string) (OS, error) {
	fields := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		fields[key] = strings.Trim(value, "\"")
	}

	id := strings.ToLower(fields["ID"])
	if id == "" {
		return OS{}, fmt.Errorf("ID not found in os-release")
	}

	family := detectFamily(id, fields["ID_LIKE"])
	if family == "" {
		return OS{}, fmt.Errorf("unsupported os: %s — requires fedora, rocky, alma, rhel, ubuntu, or debian", id)
	}

	return OS{
		Family:  family,
		ID:      id,
		Version: fields["VERSION_ID"],
	}, nil
}

func detectFamily(id, idLike string) string {
	if rhelIDs[id] {
		return "rhel"
	}
	if debianIDs[id] {
		return "debian"
	}
	// Check ID_LIKE for derivatives
	for _, like := range strings.Fields(idLike) {
		if rhelIDs[like] {
			return "rhel"
		}
		if debianIDs[like] {
			return "debian"
		}
	}
	return ""
}

// Service path methods

func (o OS) ApachePackageName() string {
	if o.Family == "debian" {
		return "apache2"
	}
	return "httpd"
}

func (o OS) ApacheConfigPath() string {
	if o.Family == "debian" {
		return "/etc/apache2/apache2.conf"
	}
	return "/etc/httpd/conf/httpd.conf"
}

func (o OS) ApacheServiceName() string {
	if o.Family == "debian" {
		return "apache2"
	}
	return "httpd"
}

func (o OS) ApacheUser() string {
	if o.Family == "debian" {
		return "www-data"
	}
	return "apache"
}

func (o OS) HasSELinux() bool {
	return o.Family == "rhel"
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/utils/platform/ -run TestParseOSRelease -v`
Expected: PASS

- [ ] **Step 5: Write PackageManager tests**

Create `internal/utils/platform/packages_test.go`:

```go
package platform

import "testing"

func TestNewPackageManager(t *testing.T) {
	tests := []struct {
		os       OS
		wantType string
	}{
		{OS{Family: "rhel"}, "*platform.DNFManager"},
		{OS{Family: "debian"}, "*platform.APTManager"},
	}
	for _, tt := range tests {
		t.Run(tt.os.Family, func(t *testing.T) {
			pm := NewPackageManager(tt.os)
			got := fmt.Sprintf("%T", pm)
			if got != tt.wantType {
				t.Errorf("NewPackageManager(%v) type = %s, want %s", tt.os, got, tt.wantType)
			}
		})
	}
}
```

- [ ] **Step 6: Implement PackageManager**

Create `internal/utils/platform/packages.go`:

```go
package platform

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

type PackageManager interface {
	Install(ctx context.Context, packages []string, logger *slog.Logger) error
	Remove(ctx context.Context, packages []string, logger *slog.Logger) error
	IsInstalled(pkg string) bool
	AddRepo(ctx context.Context, name, url string, logger *slog.Logger) error
}

func NewPackageManager(os OS) PackageManager {
	if os.Family == "debian" {
		return &APTManager{}
	}
	return &DNFManager{}
}

// DNFManager wraps dnf/rpm for RHEL-family systems.
type DNFManager struct{}

func (m *DNFManager) Install(ctx context.Context, packages []string, logger *slog.Logger) error {
	if len(packages) == 0 {
		return nil
	}
	logger.Info(fmt.Sprintf("packages: installing %s", strings.Join(packages, ", ")))
	args := append([]string{"install", "-y"}, packages...)
	if err := system.RunSudo(ctx, "dnf", args...); err != nil {
		return fmt.Errorf("dnf install failed: %w", err)
	}
	return nil
}

func (m *DNFManager) Remove(ctx context.Context, packages []string, logger *slog.Logger) error {
	if len(packages) == 0 {
		return nil
	}
	var installed []string
	for _, pkg := range packages {
		if m.IsInstalled(pkg) {
			installed = append(installed, pkg)
		}
	}
	if len(installed) == 0 {
		return nil
	}
	args := append([]string{"remove", "-y"}, installed...)
	return system.RunSudo(ctx, "dnf", args...)
}

func (m *DNFManager) IsInstalled(pkg string) bool {
	return exec.Command("rpm", "-q", pkg).Run() == nil
}

func (m *DNFManager) AddRepo(ctx context.Context, name, url string, logger *slog.Logger) error {
	logger.Info(fmt.Sprintf("packages: adding repository %s", name))
	return system.RunSudo(ctx, "dnf", "config-manager", "--add-repo", url)
}

// APTManager wraps apt-get/dpkg for Debian-family systems.
type APTManager struct{}

func (m *APTManager) Install(ctx context.Context, packages []string, logger *slog.Logger) error {
	if len(packages) == 0 {
		return nil
	}
	logger.Info(fmt.Sprintf("packages: installing %s", strings.Join(packages, ", ")))
	args := append([]string{"install", "-y"}, packages...)
	if err := system.RunSudo(ctx, "apt-get", args...); err != nil {
		return fmt.Errorf("apt-get install failed: %w", err)
	}
	return nil
}

func (m *APTManager) Remove(ctx context.Context, packages []string, logger *slog.Logger) error {
	if len(packages) == 0 {
		return nil
	}
	var installed []string
	for _, pkg := range packages {
		if m.IsInstalled(pkg) {
			installed = append(installed, pkg)
		}
	}
	if len(installed) == 0 {
		return nil
	}
	args := append([]string{"remove", "-y"}, installed...)
	return system.RunSudo(ctx, "apt-get", args...)
}

func (m *APTManager) IsInstalled(pkg string) bool {
	cmd := exec.Command("dpkg", "-l", pkg)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	// dpkg -l outputs "ii" prefix for installed packages
	return strings.Contains(string(output), "ii  "+pkg)
}

func (m *APTManager) AddRepo(ctx context.Context, name, url string, logger *slog.Logger) error {
	logger.Info(fmt.Sprintf("packages: adding repository %s", name))
	// For Debian, url should be the full sources.list line or a .list file URL
	// This is a simplified version — actual Terraform repo setup needs GPG key handling
	return system.RunSudo(ctx, "sh", "-c",
		fmt.Sprintf("echo 'deb [arch=$(dpkg --print-architecture)] %s any main' > /etc/apt/sources.list.d/%s.list && apt-get update", url, name))
}
```

- [ ] **Step 7: Run tests**

Run: `go test ./internal/utils/platform/ -v`
Expected: PASS

- [ ] **Step 8: Commit**

```
feat(platform): add os detection and package manager abstraction

new internal/utils/platform package with OS detection via
/etc/os-release and PackageManager interface (DNFManager for
rhel/fedora, APTManager for debian/ubuntu). includes service
path methods for apache config/user/service differences.
```

---

## Task 12: B1 — Migrate Existing Code to Platform Package

**Files:**
- Modify: `internal/distribution/okd/packages/packages.go`
- Modify: `internal/distribution/okd/setup/tools.go:102-122`
- Modify: `internal/distribution/okd/setup/apache.go:29,40`
- Modify: `internal/distribution/okd/setup/phase.go` (add OS field)
- Modify: `internal/distribution/okd/dns/dnsmasq.go:101-168`

- [ ] **Step 1: Add OS to Phase**

In the setup Phase struct (likely `internal/distribution/okd/setup/phase.go`), add a field for the detected OS and package manager:

```go
import "github.com/qxtaiba/okd-proxmox-cli/internal/utils/platform"

// Add to Phase struct:
OS  platform.OS
Pkg platform.PackageManager
```

Initialize these at Phase construction time using `platform.Detect()`.

- [ ] **Step 2: Update packages.go to use PackageManager**

In `internal/distribution/okd/packages/packages.go`, change `Install` and `Remove` to accept a `platform.PackageManager` parameter instead of calling `dnf`/`rpm` directly. Or refactor the callers to use `Phase.Pkg` directly and deprecate the package-level functions.

The simplest migration: replace the direct `dnf`/`rpm` calls in `packages.go` with calls to the `PackageManager` interface methods, passed as a parameter.

- [ ] **Step 3: Update installTerraform for multi-OS**

In `internal/distribution/okd/setup/tools.go`, replace the RHEL-specific terraform install:

```go
func (p *Phase) installTerraform(ctx context.Context) error {
	p.Log.Info("tools: installing terraform via hashicorp repository")

	switch p.OS.Family {
	case "rhel":
		if err := p.Pkg.AddRepo(ctx, "hashicorp", "https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo", p.Log); err != nil {
			return fmt.Errorf("failed to add HashiCorp repository: %w", err)
		}
	case "debian":
		// Install GPG key and add apt repo
		if err := system.RunSudo(ctx, "sh", "-c",
			"wget -O- https://apt.releases.hashicorp.com/gpg | gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg"); err != nil {
			return fmt.Errorf("failed to add HashiCorp GPG key: %w", err)
		}
		if err := system.RunSudo(ctx, "sh", "-c",
			`echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" > /etc/apt/sources.list.d/hashicorp.list`); err != nil {
			return fmt.Errorf("failed to add HashiCorp repository: %w", err)
		}
		if err := system.RunSudo(ctx, "apt-get", "update"); err != nil {
			return fmt.Errorf("failed to update package list: %w", err)
		}
	}

	if err := p.Pkg.Install(ctx, []string{"terraform"}, p.Log); err != nil {
		return fmt.Errorf("failed to install terraform: %w", err)
	}

	if !isToolInstalled(toolTerraform) {
		return fmt.Errorf("terraform installation verification failed")
	}

	version := getToolVersion("terraform", "--version")
	p.Log.Info(fmt.Sprintf("tools: terraform installed (%s)", version))
	return nil
}
```

- [ ] **Step 4: Update apache.go for multi-OS**

In `internal/distribution/okd/setup/apache.go`, replace hardcoded paths:

- `"apache:apache"` → `p.OS.ApacheUser() + ":" + p.OS.ApacheUser()`
- `"/etc/httpd/conf/httpd.conf"` → `p.OS.ApacheConfigPath()`
- SELinux section: wrap with `if p.OS.HasSELinux() { ... }`

- [ ] **Step 5: Add systemd-resolved fallback to dnsmasq.go**

In `internal/distribution/okd/dns/dnsmasq.go`, update `ConfigureSystemResolver` to handle Ubuntu's systemd-resolved:

After the NetworkManager check, add a fallback:
```go
if !IsNetworkManagerActive() {
	// Try systemd-resolved (common on Ubuntu)
	if system.IsServiceActive(ctx, "systemd-resolved") {
		logger.Info("dns: configuring systemd-resolved to use dnsmasq")
		// Point resolved to localhost where dnsmasq listens
		return system.RunSudo(ctx, "sh", "-c",
			`mkdir -p /etc/systemd/resolved.conf.d && echo -e "[Resolve]\nDNS=127.0.0.1\nDomains=~." > /etc/systemd/resolved.conf.d/dnsmasq.conf && systemctl restart systemd-resolved`)
	}
	logger.Warn("dns: neither NetworkManager nor systemd-resolved found, skipping system resolver configuration")
	return nil
}
```

- [ ] **Step 6: Build and verify**

Run: `go build ./...`
Expected: Clean compilation

- [ ] **Step 7: Commit**

```
refactor(platform): migrate bastion code to os-agnostic abstractions

replace direct dnf/rpm calls with PackageManager interface.
terraform install handles both rhel and debian repos. apache
paths and user derived from detected os. dnsmasq falls back
to systemd-resolved on ubuntu.
```

---

## Verification

After all tasks are complete:

- [ ] `go build ./...` — clean compilation
- [ ] `go vet ./...` — no issues
- [ ] `go test ./...` — all tests pass
- [ ] `golangci-lint run` — no new warnings
- [ ] Manual wizard run: `go run ./cmd/openshitctl deploy` — verify new fields appear correctly
