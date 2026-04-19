## okdctl completion

Generate shell completion script

### Synopsis

Generate a shell completion script for okdctl and write it to stdout.

Bash (add to ~/.bashrc or source in the current shell):
  okdctl completion bash > /etc/bash_completion.d/okdctl
  # or: source <(okdctl completion bash)

Zsh (add to ~/.zshrc):
  okdctl completion zsh > "${fpath[1]}/_okdctl"
  # or: source <(okdctl completion zsh)

Fish:
  okdctl completion fish > ~/.config/fish/completions/okdctl.fish

PowerShell (add to $PROFILE):
  okdctl completion powershell | Out-String | Invoke-Expression

```
okdctl completion [bash|zsh|fish|powershell]
```

### Options

```
  -h, --help   help for completion
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

