package cli

import (
	"github.com/spf13/cobra"
)

func (s *rootState) completionCommand() *cobra.Command {
	return &cobra.Command{
		Use:       "completion bash|zsh|fish|powershell",
		Short:     s.deps.Text.T("command.completion.short"),
		Long:      s.deps.Text.T("command.completion.long"),
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			root, out := cmd.Root(), s.deps.Stdout
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(out, true)
			case "zsh":
				return root.GenZshCompletion(out)
			case "fish":
				return root.GenFishCompletion(out, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(out)
			}
			return usageError(s.deps.Text.T("command.completion.unsupported_shell", "shell", args[0]))
		},
	}
}
