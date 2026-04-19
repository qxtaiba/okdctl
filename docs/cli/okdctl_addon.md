## okdctl addon

Manage cluster addons

### Synopsis

List, install, uninstall, and verify optional cluster addons.

### Options

```
  -h, --help   help for addon
```

### Options inherited from parent commands

```
  -c, --config string       configuration file (default "okdctl.yaml")
      --log-file string     write log output to this file in addition to stderr
      --log-format string   log output format (text, json) (default "text")
      --log-level string    log verbosity (debug, info, warn, error) (default "info")
  -q, --quiet               suppress info/warn logs (alias for --log-level=error)
  -v, --verbose             enable debug logging (alias for --log-level=debug)
```

### SEE ALSO

* [okdctl](okdctl.md)	 - Deploy production-ready Kubernetes clusters
* [okdctl addon install](okdctl_addon_install.md)	 - Install one addon (or all enabled addons with --all)
* [okdctl addon list](okdctl_addon_list.md)	 - List registered addons and their config state
* [okdctl addon uninstall](okdctl_addon_uninstall.md)	 - Uninstall a named addon
* [okdctl addon verify](okdctl_addon_verify.md)	 - Verify health of all enabled addons

