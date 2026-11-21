# Spec Delta

## Purpose

Defines `agent-cli skill`, which teaches AI coding agents (Claude Code, Codex, opencode, pi, omp, Windsurf, Cursor, Gemini CLI and Antigravity, plus a plain-text option for other agents) how to use the CLI. Installation writes a short skill for an explicitly named agent; `show` prints the guide to stdout without writing files.

## ADDED Requirements

### Requirement: Skill command group
`agent-cli skill` SHALL have the subcommands `status`, `install`, `remove` and `show`. Without a subcommand it SHALL print its help and exit 0; an unknown subcommand SHALL exit 2. Help SHALL explain explicit agent selection, user and project scopes, and that installation writes one skill with no hooks or edits to agent settings or instruction files. `setup` SHALL NOT be retained as an alias.

#### Scenario: Group help
- **WHEN** the user runs `agent-cli skill`
- **THEN** stdout is the same text as `agent-cli skill --help` and lists `status`, `install`, `remove` and `show`
- **AND** the exit code is 0

#### Scenario: Unknown subcommand
- **WHEN** the user runs `agent-cli skill nope`
- **THEN** the exit code is 2 and stderr is a single `error: ` line

### Requirement: The guide
`agent-cli skill show` SHALL print the guide for agents to stdout and exit 0. It SHALL work offline, read no credentials and no config file, create or modify no file, and print identical text for the same build and operating system. The guide SHALL come from the strings catalog, name the build through the command name placeholder, and use the per-OS variants for shell-specific instructions. It SHALL cover, with the rules that matter most first and at most 200 lines in total:
- what the CLI is for (notifying the user's own chats through their agent) and when an agent should and should not send a notification: only when the user asked for it or a long task has finished or needs the user, short plain-text messages, never secrets or private data
- running `status` first, and what to do when the machine is not connected, locked or expired: ask the user to run `connect` or `unlock` themselves in a terminal, because the agent has no terminal prompt and must never ask for, type, store or repeat a connect key, token or passphrase
- how to send a notification, including the stdin form and the targets (`--chat`, `--platform`, the defaults and `chats`), and that success is silent
- how to check command success, distinguish warnings from failures, and respond to reported errors without blindly resending or retrying credit and rate-limit failures in a loop; numeric exit-code details belong in command help
- what never to do unprompted: read or print the config or credentials files, run `revoke-all`, use `connect --store unsafe`
- that `env` lists the variables the CLI reads and `--help` shows the details of every command

The guide SHALL have one human-editable Markdown source at `internal/text/skill.md`, next to `strings.toml`, shared by every agent and build. It SHALL be embedded and exposed through the catalog as `skill.guide`; build-name and shell-instruction placeholders SHALL be filled by the catalog. Shell-specific instruction fragments and the installed stub's wording SHALL remain editable in `strings.toml`, without duplicating the full guide per operating system or agent.

#### Scenario: Guide on stdout
- **WHEN** the user runs `agent-cli skill show`
- **THEN** stdout is the guide, stderr is empty and the exit code is 0

#### Scenario: The guide names the build
- **GIVEN** the build is `abot-dev`
- **WHEN** the user runs `abot-dev skill show`
- **THEN** every command in the guide is written as `abot-dev ...` and the text `agent-cli` does not appear

#### Scenario: Per-OS instructions
- **WHEN** the guide is printed on windows and on linux
- **THEN** the unlock instruction is the PowerShell form on windows and the `eval` form elsewhere, and the rest is identical

#### Scenario: The guide covers the command set
- **THEN** every registered operational command (excluding the `skill` management group, `completion` and `help`) appears in the guide; failure guidance is concise and installation, removal and numeric exit-code details remain in CLI help

#### Scenario: No side effects
- **GIVEN** the home directory does not exist and a token is set in `THE_AGENT_CLI_TOKEN`
- **WHEN** the user runs `agent-cli skill show`
- **THEN** the home directory still does not exist and the token text appears nowhere in the output

### Requirement: Pointer for other agents
`agent-cli skill show --pointer` SHALL print at most 6 lines of plain text that tell an agent to run `agent-cli skill show` and follow the guide, meant to be pasted into the instruction file (for example `AGENTS.md`) of an agent that has no skill support. It SHALL have the same guarantees as `show`. The tool SHALL NOT edit any instruction file itself.

#### Scenario: Pointer text
- **WHEN** the user runs `agent-cli skill show --pointer`
- **THEN** stdout has at most 6 lines, contains `agent-cli skill show`, and the exit code is 0

### Requirement: The installed skill
The skill SHALL be a file `SKILL.md` in a directory named exactly like the command name of the build (`agent-cli`, `agent-cli-dev`, `abot`, `abot-dev`). Its frontmatter SHALL hold `name` (equal to the directory name), a one-sentence `description` of at most 300 characters without angle brackets, and `metadata` with the key `agent-cli-stub` and a number that the CLI raises whenever the stub text changes. Its body SHALL hold a comment saying that `agent-cli skill install` installed it and `agent-cli skill remove` removes it, the instruction to run `agent-cli skill show` and follow the guide, and the fallback to `agent-cli --help` when that command fails. The file SHALL contain no credential, no backend URL and no per-user text, and SHALL be byte-identical on every operating system.

#### Scenario: Stub content
- **WHEN** `abot skill install codex` has run
- **THEN** `<dir>/abot/SKILL.md` starts with `---`, has `name: abot`, and its body tells the agent to run `abot skill show`

### Requirement: Supported agents and locations
The supported agent names SHALL be `claude`, `codex`, `opencode`, `pi`, `omp`, `windsurf`, `cursor`, `gemini` and `antigravity`. Devin SHALL NOT be a supported name. Each agent SHALL have separate user and project skill locations; they need not have the same relative path. The build's name SHALL be appended as the skill directory.

| Agent | User skill parent | Project skill parent, relative to cwd |
| --- | --- | --- |
| `claude` | `$CLAUDE_CONFIG_DIR/skills`, otherwise `~/.claude/skills` | `.claude/skills` |
| `codex` | `~/.agents/skills` | `.agents/skills` |
| `opencode` | `~/.agents/skills` | `.agents/skills` |
| `pi` | `~/.agents/skills` | `.agents/skills` |
| `omp` | `~/.agents/skills` | `.agents/skills` |
| `windsurf` | `~/.agents/skills` | `.agents/skills` |
| `cursor` | `~/.agents/skills` | `.agents/skills` |
| `gemini` | `~/.agents/skills` | `.agents/skills` |
| `antigravity` | `~/.gemini/config/skills` | `.agents/skills` |

The shared directory SHALL be operated on once when several agents resolve to the same path. Installation SHALL NOT depend on finding an agent binary or configuration directory.

#### Scenario: Explicit agent selection
- **WHEN** the user runs `agent-cli skill install codex` on a machine without Codex installed
- **THEN** `~/.agents/skills/agent-cli/SKILL.md` exists and no other skill location is written

#### Scenario: Claude user configuration override
- **GIVEN** `CLAUDE_CONFIG_DIR=/work/claude`
- **WHEN** the user runs `agent-cli skill install claude --user`
- **THEN** `/work/claude/skills/agent-cli/SKILL.md` exists and nothing is written under `~/.agents`

#### Scenario: Antigravity locations
- **GIVEN** cwd is `/work/demo`
- **WHEN** the user runs `agent-cli skill install antigravity --user`
- **THEN** `~/.gemini/config/skills/agent-cli/SKILL.md` exists
- **AND** running `agent-cli skill install antigravity --project` writes `/work/demo/.agents/skills/agent-cli/SKILL.md` instead

### Requirement: Installation scope
`install`, `status` and `remove` SHALL accept mutually exclusive `--user` and `--project` flags. User scope SHALL be the default. Project scope SHALL use the selected harness's project path rooted at the exact current working directory, even outside a Git repository. It SHALL NOT search ancestors for a repository root or use user configuration-directory overrides.

#### Scenario: Default user scope
- **WHEN** the user runs `agent-cli skill install gemini`
- **THEN** the destination is the same as with `--user`

#### Scenario: Nested project directory
- **GIVEN** cwd is `/work/repo/subdir` and `/work/repo/.git` exists
- **WHEN** the user runs `agent-cli skill install claude --project`
- **THEN** `/work/repo/subdir/.claude/skills/agent-cli/SKILL.md` exists and no skill is written at `/work/repo/.claude` or in the user home

#### Scenario: Project outside Git
- **GIVEN** cwd is `/work/plain` and no ancestor contains `.git`
- **WHEN** the user runs `agent-cli skill install codex --project`
- **THEN** `/work/plain/.agents/skills/agent-cli/SKILL.md` exists and the exit code is 0

#### Scenario: Conflicting scopes
- **WHEN** the user runs any of `install codex`, `status` or `remove` with both `--user` and `--project`
- **THEN** the exit code is 2 and nothing is written or removed

### Requirement: Install
`agent-cli skill install [agent]` SHALL require exactly one supported agent name or explicit `--all`, never both. `--all` selects all supported agents and operates once per shared directory. Without either selection it SHALL exit 2 without writes. Repeating installation SHALL leave a current stub untouched and update an older owned stub. Foreign skills SHALL be left unchanged unless `--force` is given. `--dry-run` SHALL report planned actions without writing. Files SHALL be written atomically with mode 0644 and directories 0755.

#### Scenario: First installation
- **WHEN** the user runs `agent-cli skill install claude`
- **THEN** stdout reports the agent, action `installed` and display path, and the exit code is 0

#### Scenario: Idempotent installation
- **WHEN** the user runs `agent-cli skill install codex` twice
- **THEN** the second run reports `unchanged` and modifies no file

#### Scenario: Updating an older stub
- **GIVEN** the installed stub has a lower `agent-cli-stub` number
- **WHEN** the user runs `agent-cli skill install codex`
- **THEN** the file is replaced and the action is `updated`

#### Scenario: Conflict
- **GIVEN** `~/.agents/skills/agent-cli/SKILL.md` exists and was not installed by this CLI
- **WHEN** the user runs `agent-cli skill install codex`
- **THEN** the file is unchanged, stdout reports `conflict` with the hint to use `--force`, and the exit code is 1
- **AND** with `--force` the file is replaced and the exit code is 0

#### Scenario: Dry run
- **WHEN** the user runs `agent-cli skill install cursor --dry-run`
- **THEN** the planned action is reported and no file or directory is created

#### Scenario: Failed write
- **GIVEN** the skill directory cannot be written
- **WHEN** the user runs `agent-cli skill install codex`
- **THEN** stderr is an `error: ` line naming the path and reason, the exit code is 1, and no partial file remains

#### Scenario: Invalid agent arguments
- **WHEN** the user runs `agent-cli skill install` without an agent, with an unknown agent, or with multiple agents
- **THEN** the exit code is 2 and no files are changed
- **AND** an unknown-agent error lists all nine supported names

### Requirement: Status
`agent-cli skill status` SHALL report each supported agent's skill state in the selected scope: `current`, `outdated`, `missing`, `foreign` or `unreadable`. It SHALL change nothing, create no directory, work offline and exit 0. The repeatable `--agent` flag SHALL optionally filter supported agents; an unknown name SHALL exit 2. Reports SHALL include the agent and display path; shared paths reflect shared installation state, not independent per-agent copies.

#### Scenario: Status after installation
- **GIVEN** `agent-cli skill install claude` ran
- **WHEN** the user runs `agent-cli skill status --agent claude`
- **THEN** the Claude Code skill is `current`

#### Scenario: Edited stub
- **GIVEN** the user edited an owned installed `SKILL.md`
- **WHEN** the user runs `agent-cli skill status`
- **THEN** the skill is `outdated`

#### Scenario: Read-only project status
- **GIVEN** no project skill directory exists in cwd
- **WHEN** the user runs `agent-cli skill status --project`
- **THEN** every skill is `missing` and no directory is created

### Requirement: Remove
`agent-cli skill remove` SHALL delete owned stubs in the selected scope, selected by one positional agent or explicit `--all`, never both; without either it SHALL exit 2 without changes, and remove skill directories that become empty. It SHALL leave foreign skills, sibling skills and parent skill directories untouched, support `--dry-run`, and exit 0 when nothing was installed. An unknown agent SHALL exit 2. Shared locations SHALL be removed once; removing their stub also removes its availability to other agents using that location, which help SHALL explain.

#### Scenario: Remove after installation
- **GIVEN** `agent-cli skill install codex` installed the skill
- **WHEN** the user runs `agent-cli skill remove codex`
- **THEN** the owned skill directory is gone, the shared parent and sibling skills remain, and the exit code is 0

#### Scenario: Foreign skill survives
- **GIVEN** a skill of the same name that this CLI did not install
- **WHEN** the user runs `agent-cli skill remove`
- **THEN** it is left in place and reported as foreign

#### Scenario: Scope isolation
- **GIVEN** owned skills exist in user and cwd project locations
- **WHEN** the user runs `agent-cli skill remove --project`
- **THEN** only the cwd project stubs are removed and user stubs remain unchanged

#### Scenario: Removal dry run
- **GIVEN** an owned skill exists in user scope
- **WHEN** the user runs `agent-cli skill remove --user --dry-run`
- **THEN** the intended removal is reported and the file remains unchanged


### Requirement: Builds side by side
The skill directory and the commands in the stub and guide SHALL use the command name of the build, so `abot` and `agent-cli` installed on one machine have independent skills, and removing one leaves the other.

#### Scenario: Two builds
- **GIVEN** both `abot` and `agent-cli` ran `skill install codex`
- **WHEN** `abot skill remove` runs
- **THEN** `~/.agents/skills/abot` is gone and `~/.agents/skills/agent-cli` is unchanged

### Requirement: No secrets
No `skill` command SHALL read or print a token, connect key, passphrase or the credentials file, and none of them SHALL appear in the guide, the stub or any output.

#### Scenario: Secrets in the environment
- **GIVEN** `THE_AGENT_CLI_TOKEN` and `THE_AGENT_CLI_PASSPHRASE` are set
- **WHEN** the user runs `show`, `status`, `install codex` and `remove`
- **THEN** neither value appears in any output or written file
