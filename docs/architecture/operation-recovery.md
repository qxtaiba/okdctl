# Operation recovery and placement

Node add, resize and remove checkpoints record the approved operation scope and parameters. Retry the same request to resume. Changed dimensions, scope, batch size or disruption options require an explicit fresh-operation decision with `--acknowledge-interrupted-op`; this does not roll back earlier effects. Legacy partial markers lack these parameters and cannot establish an equivalent request. The lifecycle wizard restores recorded parameters and runs a read-only resume preview before execution.

Terraform mutation callers share `terraform.WithStateRecovery`. A failed state snapshot prevents mutation; mutation errors retain the backup path and typed cause. Destroy snapshots before init. Bootstrap teardown writes its sentinel after snapshot and before apply. Target selection, plan gates and checkpoint semantics remain with their domain owners.

Addon installation distinguishes validation from attempted mutation. A failed partial install can need compensation. `InstallAll` compensates the failing addon; `InstallOne` compensates the request's attempted installs in reverse order. Flux and secret-store preserve pre-existing namespaces on failure. New-namespace cleanup uses a bounded detached context and joins cleanup errors with the installation error; this is not transactional rollback of updates to existing resources.

## Proxmox placement

Per-role placement lists use Terraform's node index ordering and fall back to the default node for missing entries. Existing VM power and snapshot operations resolve current ownership from the Proxmox cluster inventory, including after HA migration. Unknown or ambiguous ownership fails before disruption.

Day-2 capacity checks query each destination host and the relevant OS/data datastore. Same-store disk demand is combined; a shared datastore must fit the total demand across destination hosts. These are observations, not reservations against concurrent external allocation.

Multi-host provisioning requires NFS, CIFS or CephFS ISO storage with ISO content enabled and active on every selected node. Upload uses SSH argv mode and the configured fingerprint policy, resolves the real storage path, and checks required volumes from every destination. A directory's `shared` flag alone does not establish this contract. Unsupported layouts fail before provisioning; no automatic per-host ISO replication is performed.

## Effective configuration

File-loaded v2 configurations preserve explicit resource values. Omitted/zero bootstrap CPU, memory and OS disk inherit from the control plane; an omitted/zero worker OS disk also inherits. Optional data disks remain zero when absent. The wizard's preset remains a preset, while file omission resolves through `config.Effective`. Saving materializes the resolved resource values, so a later control-plane edit does not turn those now-explicit values back into inheritance. Derived static netmask comes from the machine CIDR. Credentials remain excluded from saved YAML.

## Terminal settings

The wizard supports 80×24 and larger terminals. Smaller terminals show a bounded resize notice. Use `OKDCTL_THEME=light` for a light background, or `OKDCTL_HIGH_CONTRAST=1` for the high-contrast palette. `NO_COLOR=1` suppresses color; textual markers still identify focus, selection and errors. High contrast takes precedence over the light palette. Text fields keep Home/End for cursor editing; Ctrl+Home/Ctrl+End scroll the containing page. While editing a key/value cell, Enter keeps the edit and Escape restores that cell's prior value.
