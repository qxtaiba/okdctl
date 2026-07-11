## okdctl completion

Generate shell completion script

### Synopsis

Generate a shell completion script for okdctl and write it to stdout.

```
okdctl completion <bash|zsh|fish>
```

### Examples

```
  # Bash — write to the system completions directory or source inline:
  okdctl completion bash > /etc/bash_completion.d/okdctl
  source <(okdctl completion bash)

  # Zsh — write to a directory on $fpath or source inline:
  okdctl completion zsh > "${fpath[1]}/_okdctl"
  source <(okdctl completion zsh)

  # Fish:
  okdctl completion fish > ~/.config/fish/completions/okdctl.fish
```

### Options

```
  -h, --help   help for completion
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

