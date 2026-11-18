package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// commandOrder is the order of the Commands section of the root help; commands that are not
// registered yet are simply absent.
var commandOrder = []string{"connect", "revoke-all", "status", "server", "chats", "notify", "unlock", "env", "completion", "help"}

// help annotations: the environment and exit-code lines of a command's help
const (
	annotationEnvironment = "environment"
	annotationExitCodes   = "exit_codes"
)

// withHelp attaches the examples and the environment and exit-code lines to a command's help.
func withHelp(cmd *cobra.Command, examples, environment, exitCodes string) *cobra.Command {
	cmd.Example = examples
	cmd.Annotations = map[string]string{annotationEnvironment: environment, annotationExitCodes: exitCodes}
	return cmd
}

// rootState is the per-invocation state shared by the commands.
type rootState struct {
	deps    Deps
	verbose bool
	version bool
}

func newRoot(deps Deps) (*cobra.Command, *rootState) {
	state := &rootState{deps: deps}
	cat := deps.Text

	root := &cobra.Command{
		Use:           deps.Build.Name,
		Short:         cat.T("root.description", "name", deps.Build.Name, "product_url", deps.Build.ProductURL),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if state.version {
				state.printVersion()
				return nil
			}
			return cmd.Help()
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetOut(deps.Stdout)
	root.SetErr(deps.Stderr)
	root.SetIn(deps.Stdin)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError(err.Error()) })

	root.PersistentFlags().BoolVarP(&state.verbose, "verbose", "v", false, cat.T("root.flag_verbose"))
	root.PersistentFlags().BoolP("help", "h", false, cat.T("root.flag_help"))
	root.Flags().BoolVar(&state.version, "version", false, cat.T("root.flag_version"))

	root.SetHelpCommand(state.helpCommand())
	root.SetHelpFunc(state.renderHelp)

	root.AddCommand(state.connectCommand())
	root.AddCommand(state.envCommand())
	root.AddCommand(state.serverCommand())
	root.AddCommand(state.completionCommand())

	state.guardArgs(root)
	return root, state
}

// guardArgs makes every positional-argument problem a usage error. A command without its own
// validator accepts no arguments, or only a known subcommand when it has subcommands.
func (s *rootState) guardArgs(cmd *cobra.Command) {
	validate := cmd.Args
	switch {
	case validate == nil && cmd.HasSubCommands():
		validate = func(c *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			return usageError(s.deps.Text.T("error.unknown_command", "command", args[0], "path", c.CommandPath()))
		}
	case validate == nil:
		validate = cobra.NoArgs
	}
	if !cmd.Runnable() && cmd.HasSubCommands() {
		// without a run function Cobra prints help and never validates the arguments
		cmd.RunE = func(c *cobra.Command, _ []string) error { return c.Help() }
	}
	cmd.Args = func(c *cobra.Command, args []string) error {
		if err := validate(c, args); err != nil {
			return usageError(err.Error())
		}
		return nil
	}
	for _, child := range cmd.Commands() {
		s.guardArgs(child)
	}
}

func (s *rootState) printVersion() {
	build := s.deps.Build
	_, _ = fmt.Fprintln(s.deps.Stdout, build.VersionLine(s.deps.GOOS, s.deps.GOARCH))
	if s.verbose {
		_, _ = fmt.Fprintln(s.deps.Stdout, build.CommitLine())
	}
}

func (s *rootState) renderHelp(cmd *cobra.Command, _ []string) {
	out := s.commandHelp(cmd)
	if !cmd.HasParent() {
		out = s.rootHelp(cmd)
	}
	_, _ = fmt.Fprint(s.deps.Stdout, out)
}

func (s *rootState) rootHelp(root *cobra.Command) string {
	cat, build := s.deps.Text, s.deps.Build
	var commands []*cobra.Command
	for _, name := range commandOrder {
		for _, c := range root.Commands() {
			if c.Name() == name && (c.IsAvailableCommand() || name == "help") {
				commands = append(commands, c)
			}
		}
	}
	width := 0
	for _, c := range commands {
		width = max(width, len(c.Name()))
	}

	var b strings.Builder
	fmt.Fprintln(&b, cat.T("root.description", "name", build.Name, "product_url", build.ProductURL))
	fmt.Fprintf(&b, "%s\n\n", cat.T("root.usage", "name", build.Name))
	fmt.Fprintln(&b, cat.T("root.commands_label"))
	for _, c := range commands {
		fmt.Fprintf(&b, "  %-*s  %s\n", width, c.Name(), c.Short)
	}
	fmt.Fprintf(&b, "\n%s\n", cat.T("root.flags"))
	fmt.Fprintln(&b, cat.T("root.environment"))
	fmt.Fprintln(&b, cat.T("root.exit_codes"))
	return b.String()
}

func (s *rootState) commandHelp(cmd *cobra.Command) string {
	cat := s.deps.Text
	var b strings.Builder
	description := cmd.Long
	if description == "" {
		description = cmd.Short
	}
	fmt.Fprintf(&b, "%s\n\n", description)
	fmt.Fprintf(&b, "%s %s\n", cat.T("help.usage_label"), usageLine(cmd))
	if lines := flagLines(cmd.LocalFlags(), cmd.InheritedFlags()); len(lines) > 0 {
		fmt.Fprintf(&b, "\n%s\n%s\n", cat.T("help.flags_label"), strings.Join(lines, "\n"))
	}
	if cmd.Example != "" {
		fmt.Fprintf(&b, "\n%s\n%s\n", cat.T("help.examples_label"), cmd.Example)
	}
	fmt.Fprintf(&b, "\n%s\n%s\n", cmd.Annotations[annotationEnvironment], cmd.Annotations[annotationExitCodes])
	return b.String()
}

// usageLine is the command path followed by the argument synopsis from Use; unlike Cobra's UseLine it
// does not depend on whether the flags were parsed yet, so "help <command>" and "<command> -h" agree.
func usageLine(cmd *cobra.Command) string {
	_, synopsis, _ := strings.Cut(cmd.Use, " ")
	return strings.TrimSpace(cmd.CommandPath() + " " + synopsis)
}

// flagLines formats flags as aligned "  -s, --long <value>  usage" lines, local flags first.
func flagLines(sets ...*pflag.FlagSet) []string {
	type entry struct{ name, usage string }
	var entries []entry
	seen := map[string]bool{}
	for _, set := range sets {
		set.VisitAll(func(f *pflag.Flag) {
			if f.Hidden || seen[f.Name] {
				return
			}
			seen[f.Name] = true
			name := "    --" + f.Name
			if f.Shorthand != "" {
				name = "-" + f.Shorthand + ", --" + f.Name
			}
			varname, usage := pflag.UnquoteUsage(f)
			if varname != "" && f.Value.Type() != "bool" {
				name += " <" + varname + ">"
			}
			entries = append(entries, entry{name, usage})
		})
	}
	width := 0
	for _, e := range entries {
		width = max(width, len(e.name))
	}
	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		lines = append(lines, fmt.Sprintf("  %-*s  %s", width, e.name, e.usage))
	}
	return lines
}

func (s *rootState) helpCommand() *cobra.Command {
	cat := s.deps.Text
	return withHelp(&cobra.Command{
		Use:   "help [command]",
		Short: cat.T("command.help.short"),
		Long:  cat.T("command.help.long"),
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root := cmd.Root()
			target, _, err := root.Find(args)
			if err != nil || (target == root && len(args) > 0) {
				return usageError(s.deps.Text.T("error.unknown_help_topic", "topic", strings.Join(args, " ")))
			}
			return target.Help()
		},
	}, cat.T("command.help.examples", "name", s.deps.Build.Name), cat.T("command.help.environment"), cat.T("command.help.exit_codes"))
}
