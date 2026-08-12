## okdctl node resize

Resize node CPU/memory/OS-disk per role, rolled out one node at a time

### Synopsis

Change per-role node resources and roll the change out one node at a
time. Masters are etcd-health-gated before and after every node and applied
with an in-place-update plan gate (a VM replace is refused). Workers roll
without the etcd gate.

Sizing is per-role: the role's memory/cpu/disk knob is updated in config and
tfvars, but each targeted apply mutates only the current node; other same-role
nodes pick up the pending change on the next full deploy. Run 'okdctl plan'
after a resize to see exactly which same-role siblings still have the change
pending.

At least one of --memory-mb, --cpu, or --os-disk-gb is required; an omitted
dimension keeps the role's current value. --os-disk-gb is grow-only (shrink is
refused) and, unlike memory/cpu, is realized live: the Proxmox disk is grown
and the in-guest filesystem grown into it via 'oc debug' without a
power-cycle. A resize that combines --os-disk-gb with --memory-mb/--cpu still
grows the disk live before the power-cycle that realizes the other dimensions.
--os-disk-gb is role-scoped only ('masters'/'workers', not a single node
name): a same-role sibling can always catch up on a memory/cpu change at its
next full deploy, but CoreOS only grows the filesystem on firstboot, so a
sibling left behind by a single-node disk grow could never catch up, and a
later same-size role-wide resize would then be refused by the grow-only check
above.

--skip-drain power-cycles the node without cordoning/draining it. The resize is
realized by a hypervisor stop→start that kills the node's pods regardless;
skipping the drain lets them restart in place on the now-roomier node instead of
evicting them cluster-wide. Prefer it when the cluster is memory-saturated, where
a drain's evicted pods cannot reschedule and the drain times out. The etcd and
Ceph health gates around the power-cycle still run. --skip-drain has no effect
on a disk-only resize, which never power-cycles.

An interrupted role roll records an op marker and resumes automatically on the
next 'okdctl node resize' of the same role or node, skipping already-completed
nodes and steps. --acknowledge-interrupted-op overrides a marker left by a
different op or node instead of refusing.

```
okdctl node resize (masters|workers|<name>) [flags]
```

### Examples

```
  okdctl node resize masters --memory-mb 24576 --yes --confirm-cluster grappleberry
  okdctl node resize workers --memory-mb 16384 --dry-run
  okdctl node resize grappleberry-master0 --memory-mb 30720 --skip-drain --yes --confirm-cluster grappleberry
  okdctl node resize masters --os-disk-gb 100
```

### Options

```
      --acknowledge-interrupted-op   override a stranded marker left by a different op or node and proceed fresh
      --confirm-cluster string       required with --yes; must equal the config cluster name
      --cpu int                      new per-node cpu cores (0 keeps current)
      --dry-run                      run gates and the plan gate without mutating anything
  -h, --help                         help for resize
      --memory-mb int                new per-node memory in MiB (0 keeps current)
      --os-disk-gb int               grow the role's OS disk to this size in GiB (grow-only, role-scoped only — 'masters'/'workers', not a single node; disk-only resizes are live, no power-cycle)
      --skip-drain                   power-cycle without cordon/drain so pods restart in place (use when a drain can't reschedule under memory pressure); etcd/Ceph gates still run
  -y, --yes                          skip confirmation prompt
```

### Options inherited from parent commands

```
  -c, --config string       configuration file (default "okdctl.yaml")
      --log-file string     write log output to this file in addition to stderr (replaces the default okdctl.log sink of deploy/destroy/cleanup)
      --log-format string   log output format: text (TTY default) | json (auto-selected when stderr is piped)
      --log-level string    log verbosity (debug, info, warn, error) (default "info")
  -q, --quiet               suppress info/warn logs (alias for --log-level=error)
  -v, --verbose             enable debug logging (alias for --log-level=debug)
```

### SEE ALSO

* [okdctl node](okdctl_node.md)	 - Manage cluster node lifecycle

