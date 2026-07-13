## okdctl node add

Add a node to the cluster (not yet implemented)

### Synopsis

Adding a node requires building and uploading a per-node CoreOS ISO and
reviving the ignition HTTPS server post-install; it is deferred (see the node
lifecycle spec, phase 4). Use 'okdctl deploy' to grow a fresh cluster.

```
okdctl node add --role worker [flags]
```

### Options

```
  -h, --help          help for add
      --role string   node role to add (default "worker")
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

