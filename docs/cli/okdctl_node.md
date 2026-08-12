## okdctl node

Manage cluster node lifecycle

### Synopsis

Add, remove, and resize cluster nodes as first-class operations that span
Proxmox VMs (via Terraform), Terraform state, and the Kubernetes lifecycle
(cordon, drain, CSR, etcd-quorum safety).

### Options

```
  -h, --help   help for node
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

* [okdctl](okdctl.md)	 - Provision OKD clusters on Proxmox VE
* [okdctl node add](okdctl_node_add.md)	 - Add worker node(s) to the cluster
* [okdctl node list](okdctl_node_list.md)	 - List cluster nodes with role, readiness, and sizing drift
* [okdctl node manage](okdctl_node_manage.md)	 - Interactively manage node lifecycle (resize / add / remove)
* [okdctl node remove](okdctl_node_remove.md)	 - Remove a worker node from the cluster
* [okdctl node resize](okdctl_node_resize.md)	 - Resize node CPU/memory/OS-disk per role, rolled out one node at a time
* [okdctl node snapshot](okdctl_node_snapshot.md)	 - Manual, single-node Proxmox VM snapshots

