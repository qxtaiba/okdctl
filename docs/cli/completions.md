# Shell completion

Generate and install a completion script for your shell:

```sh
# bash — persistent
okdctl completion bash > /etc/bash_completion.d/okdctl

# bash — current session only
source <(okdctl completion bash)

# zsh — persistent (pick a dir in $fpath)
okdctl completion zsh > "${fpath[1]}/_okdctl"

# zsh — current session only
source <(okdctl completion zsh)

# fish
okdctl completion fish > ~/.config/fish/completions/okdctl.fish
```

Command reference: [okdctl completion](okdctl_completion.md).
