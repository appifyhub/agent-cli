package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/spf13/cobra"

	"github.com/appifyhub/agent-cli/internal/agentskill"
	"github.com/appifyhub/agent-cli/internal/render"
	"github.com/appifyhub/agent-cli/internal/term"
)

func (s *rootState) skillCommand() *cobra.Command {
	cat, name := s.deps.Text, s.deps.Build.Name
	command := withHelp(&cobra.Command{Use: "skill", Short: cat.T("command.skill.short"), Long: cat.T("command.skill.long", "name", name)}, cat.T("command.skill.examples", "name", name), cat.T("command.skill.environment"), cat.T("command.skill.exit_codes"))
	var pointer bool
	show := withHelp(&cobra.Command{
		Use: "show", Short: cat.T("command.skill_show.short"), Long: cat.T("command.skill_show.long"), Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			value := cat.T("skill.pointer", "name", name)
			if !pointer {
				value = cat.T("skill.guide", "name", name, "unlock", cat.T("skill.unlock", "name", name))
			}
			_, err := fmt.Fprint(s.deps.Stdout, value)
			return err
		},
	}, cat.T("command.skill_show.examples", "name", name), cat.T("command.skill_show.environment"), cat.T("command.skill_show.exit_codes"))
	show.Flags().BoolVar(&pointer, "pointer", false, cat.T("skill.flag_pointer"))
	install := withHelp(&cobra.Command{Use: "install [agent]", Short: cat.T("command.skill_install.short"), Long: cat.T("command.skill_install.long", "name", name)}, cat.T("command.skill_install.examples", "name", name), cat.T("command.skill_install.environment"), cat.T("command.skill_install.exit_codes"))
	status := withHelp(&cobra.Command{Use: "status", Short: cat.T("command.skill_status.short"), Long: cat.T("command.skill_status.long")}, cat.T("command.skill_status.examples", "name", name), cat.T("command.skill_status.environment"), cat.T("command.skill_status.exit_codes"))
	remove := withHelp(&cobra.Command{Use: "remove [agent]", Short: cat.T("command.skill_remove.short"), Long: cat.T("command.skill_remove.long")}, cat.T("command.skill_remove.examples", "name", name), cat.T("command.skill_remove.environment"), cat.T("command.skill_remove.exit_codes"))
	command.AddCommand(s.skillFilesCommand("install", install), s.skillFilesCommand("status", status), s.skillFilesCommand("remove", remove), show)
	return command
}

type skillOptions struct {
	user, project, dryRun, force, all bool
	agents                            []string
}

func (s *rootState) skillFilesCommand(operation string, command *cobra.Command) *cobra.Command {
	cat := s.deps.Text
	options := skillOptions{}
	complete := func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		agents := agentskill.Supported()
		names := make([]string, 0, len(agents))
		for _, a := range agents {
			names = append(names, a.Name)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	}
	command.Flags().BoolVar(&options.user, "user", false, cat.T("skill.flag_user"))
	command.Flags().BoolVar(&options.project, "project", false, cat.T("skill.flag_project"))
	if operation != "status" {
		command.ValidArgsFunction = complete
		command.Flags().BoolVar(&options.all, "all", false, cat.T("skill.flag_all"))
	} else {
		command.Flags().StringArrayVar(&options.agents, "agent", nil, cat.T("skill.flag_agent"))
		_ = command.RegisterFlagCompletionFunc("agent", complete)
	}
	if operation == "install" {
		command.Flags().BoolVar(&options.force, "force", false, cat.T("skill.flag_force"))
	}
	if operation != "status" {
		command.Flags().BoolVar(&options.dryRun, "dry-run", false, cat.T("skill.flag_dry_run"))
	}
	command.Args = func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("user") && cmd.Flags().Changed("project") {
			return usageError(cat.T("skill.scope_conflict"))
		}
		if operation != "status" {
			if options.all && len(args) != 0 || !options.all && len(args) != 1 {
				return usageError(cat.T("skill.selection_usage", "operation", operation))
			}
			options.agents = args
		} else if err := cobra.NoArgs(cmd, args); err != nil {
			return err
		}
		for _, name := range options.agents {
			if _, ok := agentskill.Lookup(name); !ok {
				names, _ := complete(nil, nil, "")
				return usageError(cat.T("skill.unknown_agent", "agent", name, "agents", strings.Join(names, ", ")))
			}
		}
		return nil
	}
	command.RunE = func(*cobra.Command, []string) error { return s.runSkillFiles(operation, options) }
	return command
}

func (s *rootState) skillStub() []byte {
	cat, name := s.deps.Text, s.deps.Build.Name
	return []byte(cat.T("skill.stub", "name", name, "description", cat.T("skill.stub_description", "name", name)))
}

func (s *rootState) runSkillFiles(operation string, options skillOptions) error {
	d, cat := &s.deps, s.deps.Text
	home, homeErr := d.UserHomeDir()
	root, err := home, homeErr
	if options.project {
		root, err = d.Getwd()
	}
	// an explicit Claude user directory works even when the home is unavailable
	if !options.project && len(options.agents) == 1 && options.agents[0] == "claude" && d.Getenv("CLAUDE_CONFIG_DIR") != "" {
		root, err = d.Getenv("CLAUDE_CONFIG_DIR"), nil
	}
	if err != nil || root == "" {
		return &Failure{Code: ExitFailure, Message: cat.T("skill.location_error")}
	}
	all := agentskill.Supported()
	selected := all[:]
	if len(options.agents) > 0 {
		selected = make([]agentskill.Agent, 0, len(options.agents))
		for _, name := range options.agents {
			a, _ := agentskill.Lookup(name)
			selected = append(selected, a)
		}
	}
	stub := s.skillStub()
	results := make(map[string]string)
	conflict := false
	for _, agent := range selected {
		dir := agentskill.Directory(agent, root, d.Getenv("CLAUDE_CONFIG_DIR"), d.Build.Name, options.project)
		result, done := results[dir]
		if !done {
			if operation == "status" {
				state, _ := agentskill.Inspect(dir, stub)
				result = cat.T("skill.state." + string(state))
			} else {
				var action agentskill.Action
				if operation == "install" {
					action, err = agentskill.Install(dir, stub, options.force, options.dryRun)
				} else {
					action, err = agentskill.Remove(dir, stub, options.dryRun)
				}
				if err != nil {
					var pathErr *fs.PathError
					if errors.As(err, &pathErr) {
						err = pathErr.Err
					}
					return &Failure{Code: ExitFailure, Message: cat.T("skill.io_error", "path", dir, "reason", err.Error())}
				}
				result = cat.T("skill.action." + string(action))
				conflict = conflict || action == agentskill.Conflict
			}
			results[dir] = result
		}
		line := cat.T("skill.report", "agent", agent.DisplayName, "result", result, "path", term.Clean(render.DisplayPath(dir, home, d.GOOS), 0))
		if _, err := fmt.Fprintln(d.Stdout, line); err != nil {
			return err
		}
	}
	if conflict {
		return &Failure{Code: ExitFailure, Silent: true}
	}
	return nil
}
