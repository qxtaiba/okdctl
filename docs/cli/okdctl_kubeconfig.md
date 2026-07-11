## okdctl kubeconfig

Print or export the cluster kubeconfig

### Synopsis

Print the cluster kubeconfig to stdout, write it to a file,
or merge it into an existing kubeconfig.

```
okdctl kubeconfig [flags]
```

### Examples

```
  okdctl kubeconfig                       # print to stdout
  okdctl kubeconfig --output-file ~/.kube/okd.cfg    # write to file
  okdctl kubeconfig --merge               # merge into $KUBECONFIG
```

### Options

```
  -h, --help                 help for kubeconfig
      --merge                merge into $KUBECONFIG or ~/.kube/config (non-destructive: existing entries preserved)
      --output-file string   write kubeconfig to file ('-' for stdout) (default "-")
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

* [okdctl](okdctl.md)	 - Deploy production-ready Kubernetes clusters

