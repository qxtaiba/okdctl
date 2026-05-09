package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion <bash|zsh|fish>",
	Short: "Generate shell completion script",
	Long: `Generate a shell completion script for okdctl and write it to stdout.

Bash (add to ~/.bashrc or source in the current shell):
  okdctl completion bash > /etc/bash_completion.d/okdctl
  # or: source <(okdctl completion bash)

Zsh (add to ~/.zshrc):
  okdctl completion zsh > "${fpath[1]}/_okdctl"
  # or: source <(okdctl completion zsh)

Fish:
  okdctl completion fish > ~/.config/fish/completions/okdctl.fish`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE:                  runCompletion,
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.AddCommand(completionCmd)
}

func runCompletion(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	switch args[0] {
	case "bash":
		return cmd.Root().GenBashCompletionV2(out, true)
	case "zsh":
		return cmd.Root().GenZshCompletion(out)
	case "fish":
		return cmd.Root().GenFishCompletion(out, true)
	default:
		return fmt.Errorf("unknown shell %q", args[0])
	}
}
