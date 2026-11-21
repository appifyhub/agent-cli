# Spec Delta

## Purpose

Defines the observable shell of the command-line tool: help and version output, global flags, per-OS instructions, the text catalog, build-time identity, exit codes, output streams, color, text sanitization, and shell completion. The tool is generic: its command name, default backend and product page are build-time values. Examples use the default command name `agent-cli`; a build with another name (for example a hosted instance such as `abot`) shows its own name in the same places.

## ADDED Requirements

### Requirement: Root help explains the tool
`agent-cli --help`, `agent-cli -h`, `agent-cli help` and `agent-cli` with no arguments SHALL print to stdout, with exit code 0, a one-line description that mentions the agent and the product page URL, the usage line, the commands, the flags, the environment variables and the exit codes, in that order.

#### Scenario: Help via flag
- **WHEN** the user runs `agent-cli --help` on a default build
- **THEN** the first line is `agent-cli – command-line access to your agent: connect this machine and send notifications to your chats. More at https://agent.appifyhub.com`
- **AND** the next line is `Usage: agent-cli <command> [flags]`
- **AND** the process exits with code 0

#### Scenario: Command list wording
- **WHEN** the user runs `agent-cli --help`
- **THEN** the `Commands:` section lists, in this order, `connect` ("Connect this machine to your profile using a connect key"), `revoke-all` ("Revoke access for all CLIs on all your machines"), `status` ("Show the connection status of this machine"), `server` ("Show the health and version of the backend"), `chats` ("List your chats, show or set the default chat"), `notify` ("Send a notification to one of your chats"), `unlock` ("Unlock a passphrase-protected token for this terminal session"), `env` ("Show the environment variables the CLI reads and their values"), `skill` ("Teach AI coding agents to use this CLI"), `completion` ("Generate shell completion scripts") and `help` ("Help for any command")

#### Scenario: Flags, environment and exit codes
- **WHEN** the user runs `agent-cli --help`
- **THEN** the output contains the line `Flags: -v/--verbose, -h/--help, --version`
- **AND** the line `Environment: THE_AGENT_CLI_TOKEN, THE_AGENT_CLI_DEFAULT_CHAT, THE_AGENT_CLI_DEFAULT_PLATFORM, THE_AGENT_CLI_PASSPHRASE, THE_AGENT_CLI_API_URL, THE_AGENT_CLI_HOME, NO_COLOR`
- **AND** the line `Exit codes: 0 ok · 1 failure · 2 usage error · 3 not connected or locked · 4 chat unreachable · 5 insufficient credits · 6 rate limited`

#### Scenario: Bare invocation
- **WHEN** the user runs `agent-cli` with no arguments
- **THEN** the same help text as `agent-cli --help` is printed to stdout and the exit code is 0

#### Scenario: Product page comes from the build
- **GIVEN** a binary built with the product page URL `https://example.test/agent`
- **WHEN** the user runs `agent-cli --help`
- **THEN** the description line ends with `More at https://example.test/agent`

### Requirement: Command help is self-contained
`agent-cli <command> --help` and `agent-cli help <command>` SHALL print the command's purpose, usage line, flags with descriptions, at least one example, and the environment variables and exit codes that affect it.

#### Scenario: Notify help
- **WHEN** the user runs `agent-cli notify --help`
- **THEN** the output shows the usage `agent-cli notify [message] [--chat <id> | --platform telegram|whatsapp]`
- **AND** it documents `--chat`, `--platform` and the stdin form `echo "Backup finished" | agent-cli notify`
- **AND** it mentions `THE_AGENT_CLI_DEFAULT_CHAT`, `THE_AGENT_CLI_DEFAULT_PLATFORM` and the exit codes 2, 3, 4, 5 and 6

#### Scenario: Nested command help
- **WHEN** the user runs `agent-cli chats default --help`
- **THEN** the output documents the three forms `<chat-id>`, `--platform telegram|whatsapp` and `--clear`

### Requirement: Instructions match the running OS
Help text and hints that differ per operating system SHALL show only the instruction for the OS the binary is running on: `eval "$(agent-cli unlock)"` on macOS and Linux, and `agent-cli unlock --shell pwsh | Invoke-Expression` on Windows.

#### Scenario: Unlock hint on macOS or Linux
- **GIVEN** the binary runs on darwin or linux
- **WHEN** the user runs `agent-cli unlock --help`
- **THEN** the usage instruction is `eval "$(agent-cli unlock)"`
- **AND** the PowerShell form is not shown as the instruction

#### Scenario: Unlock hint on Windows
- **GIVEN** the binary runs on windows
- **WHEN** the user runs `agent-cli unlock --help`
- **THEN** the usage instruction is `agent-cli unlock --shell pwsh | Invoke-Expression`
- **AND** the `eval` form is not shown as the instruction

### Requirement: Version output is offline and exact
`agent-cli --version` SHALL print `agent-cli <version> (<os>/<arch>, <date>)` to stdout and exit 0, without network access and without creating files. With `--verbose` it SHALL additionally print the build commit.

#### Scenario: Release build
- **GIVEN** a build with version `0.1.0` and build date `2026-11-14`, running on darwin/arm64
- **WHEN** the user runs `agent-cli --version`
- **THEN** stdout is `agent-cli 0.1.0 (darwin/arm64, 2026-11-14)`

#### Scenario: Local build
- **GIVEN** a build made without commit and date variables
- **WHEN** the user runs `agent-cli --version`
- **THEN** stdout is `agent-cli <VERSION> (<os>/<arch>, unknown)`

#### Scenario: Verbose version
- **GIVEN** a build with commit `1a2b3c4`
- **WHEN** the user runs `agent-cli --version --verbose`
- **THEN** stdout has the normal version line followed by `  commit: 1a2b3c4`

### Requirement: Global flags
The flags `-v/--verbose`, `-h/--help` and `--version` SHALL be accepted: `--verbose` and `--help` on every command before or after the subcommand name, `--version` on the root command. Verbose output SHALL go to stderr.

#### Scenario: Verbose after the subcommand
- **WHEN** the user runs `agent-cli notify "Build finished" -v` against a working backend
- **THEN** stderr contains `Calling backend…`
- **AND** stdout does not contain that line

#### Scenario: Help after the subcommand
- **WHEN** the user runs `agent-cli connect -h`
- **THEN** the help for `connect` is printed and the exit code is 0

### Requirement: Exit code contract
The process SHALL exit with 0 for success, 1 for failure (network, server, unexpected), 2 for usage or validation errors, 3 for not connected, locked or revoked, 4 for an unreachable chat, 5 for insufficient credits, and 6 for rate limiting. No other code SHALL be used.

#### Scenario: Unknown command
- **WHEN** the user runs `agent-cli frobnicate`
- **THEN** stderr is one line starting with `error: unknown command "frobnicate" for "agent-cli"`
- **AND** the exit code is 2

#### Scenario: Unknown flag
- **WHEN** the user runs `agent-cli status --nope`
- **THEN** stderr starts with `error: unknown flag: --nope` and the exit code is 2

#### Scenario: Unexpected internal failure
- **GIVEN** a bug causes a panic inside a command
- **WHEN** the command runs without `--verbose`
- **THEN** the exit code is 1 and stderr is a single `error:` line without a stack trace
- **AND** with `--verbose` the stack trace is printed to stderr

### Requirement: Output streams and error format
Command results SHALL go to stdout. Errors, warnings, prompts and verbose diagnostics SHALL go to stderr. Each error SHALL be one line starting with `error: `, except the not-connected guidance `This CLI is not connected. Run `agent-cli connect <KEY>`.`, which is printed verbatim.

#### Scenario: Error line
- **WHEN** a command fails with a handled error such as an unreachable server
- **THEN** stdout is empty
- **AND** stderr is a single line starting with `error: `

#### Scenario: Prompts keep stdout clean
- **WHEN** a command shows a passphrase or confirmation prompt
- **THEN** the prompt text is written to stderr and nothing is written to stdout until the command's result

### Requirement: Color is opt-out and terminal-only
Output SHALL use color only when the destination stream is a terminal with ANSI support, `NO_COLOR` is unset or empty, and `TERM` is not `dumb`. Apply this policy independently to stdout and stderr. Otherwise output SHALL contain no escape sequences. Help headings SHALL be bold cyan, success confirmations green, warnings yellow and errors red; status summaries SHALL be green when ready and yellow when not connected, locked or expired. Styling SHALL preserve the plain text and exit code. The success mark is `✓` and error lines start with `error:` regardless of color. Shell assignments from `unlock` and generated completion scripts SHALL always remain unstyled.

#### Scenario: NO_COLOR set
- **GIVEN** `NO_COLOR=1` and stdout is a terminal
- **WHEN** a command prints `✓ Connected as Morpheus (morpheus-mbp)`
- **THEN** the output contains no ANSI escape sequences

#### Scenario: Output piped
- **WHEN** `agent-cli status` output is piped to another program
- **THEN** the piped output contains no ANSI escape sequences

#### Scenario: Interactive terminal
- **GIVEN** stdout is a terminal, `NO_COLOR` is unset and `TERM=xterm-256color`
- **WHEN** a command prints a success line
- **THEN** the `✓` mark is colored

#### Scenario: Mixed terminal and redirected streams
- **GIVEN** stdout is redirected, stderr is an ANSI terminal, `NO_COLOR` is unset and `TERM=xterm`
- **WHEN** a successful default change also reports an environment override
- **THEN** stdout contains the plain success line and stderr contains the yellow warning

#### Scenario: Unlock output is safe to evaluate
- **GIVEN** both output streams are ANSI terminals and color is enabled
- **WHEN** `agent-cli unlock` succeeds
- **THEN** the assignment on stdout contains no ANSI escapes and the confirmation on stderr is green

### Requirement: Untrusted text is neutralized
Text that originates outside the program (chat titles, display names, server error messages) SHALL have control characters and terminal escape sequences removed before it is printed, and server error messages SHALL be limited to one line of at most 200 characters.

#### Scenario: Chat title with escape sequence
- **GIVEN** the backend returns a chat titled `Dev\u001b[31m Team`
- **WHEN** the user runs `agent-cli chats`
- **THEN** the printed title contains no ESC character

#### Scenario: Multi-line server message
- **GIVEN** the backend answers an error with a 600-character message containing newlines
- **WHEN** the CLI prints that message
- **THEN** it is printed on one line, truncated to 200 characters

### Requirement: Times are shown in local time with a relative suffix
Timestamps (token expiry) SHALL be shown as `YYYY-MM-DD HH:MM` in the machine's local time zone followed by a relative suffix in parentheses: `in less than a minute`, `in N minutes`, `in N hours`, `in N days` (under 30 days), then `in N months`, with a multiple of 12 months shown as years. Past times use `N <unit> ago`. Counts are rounded to the nearest unit; singular forms are used for 1.

#### Scenario: Three calendar months
- **GIVEN** the clock reads 2026-11-14 10:21 in the Europe/Berlin time zone
- **WHEN** an expiry of 2027-02-14 09:21 UTC is shown
- **THEN** the text is `2027-02-14 10:21 (in 3 months)`

#### Scenario: Short and long horizons
- **WHEN** the expiry is 40 seconds, 90 minutes, 5 hours, 10 days, 18 months or 24 months away
- **THEN** the suffixes are `in less than a minute`, `in 2 hours`, `in 5 hours`, `in 10 days`, `in 18 months` and `in 2 years`

#### Scenario: One unit and the past
- **WHEN** the expiry is 1 day away, 12 months away, or 2 days in the past
- **THEN** the suffixes are `in 1 day`, `in 1 year` and `2 days ago`

### Requirement: One editable text catalog
All user-facing text (help, hints, messages, errors, glyphs) SHALL come from a single human-editable catalog that is embedded in the binary. Entries MAY have per-OS variants. Text produced by the command-line parser for malformed invocations is the only exception.

#### Scenario: Editing a catalog entry
- **GIVEN** a maintainer changes the catalog text for the `status` command description
- **WHEN** the binary is rebuilt and the user runs `agent-cli --help`
- **THEN** the `status` line shows the new description and no other code changed

#### Scenario: Variant selection
- **GIVEN** the catalog has a Windows variant for a hint
- **WHEN** the binary runs on windows
- **THEN** the Windows variant is printed, and on other systems the default entry is printed

### Requirement: Build-time identity
The command name, commit, build date, default API URL and product page URL SHALL be settable at build time, and an empty value SHALL count as unset. Unset values SHALL default to `agent-cli`, `unknown`, `unknown`, `http://localhost` and `https://agent.appifyhub.com`. The version SHALL always come from the `VERSION` file and SHALL NOT be settable separately. The command name SHALL be used wherever the tool names itself: help, usage lines, the version line, shell completion and the user agent.

#### Scenario: Default build
- **GIVEN** a build made without build-time values, `THE_AGENT_CLI_API_URL` is unset, no config file exists and `THE_AGENT_CLI_TOKEN` is set
- **WHEN** the user runs `agent-cli status`
- **THEN** the `Backend:` line shows `http://localhost`

#### Scenario: Custom API URL at build time
- **GIVEN** a build with the default API URL `https://staging.agent.cloud.appifyhub.com`, `THE_AGENT_CLI_API_URL` is unset, no config file exists and `THE_AGENT_CLI_TOKEN` is set
- **WHEN** the user runs `status`
- **THEN** the `Backend:` line shows `https://staging.agent.cloud.appifyhub.com`

#### Scenario: Command name in help
- **GIVEN** a build with the command name `my-agent`
- **WHEN** the user runs `my-agent --help`
- **THEN** the first line starts with `my-agent – command-line access to your agent` and the usage line is `Usage: my-agent <command> [flags]`

#### Scenario: Version from the version file
- **GIVEN** `VERSION` contains `0.1.0`
- **WHEN** the binary is built in any way (`go build`, `go install`, the build script or a release)
- **THEN** its `--version` line shows `0.1.0`

### Requirement: Shell completion
`agent-cli completion bash|zsh|fish|powershell` SHALL print the completion script for that shell to stdout and exit 0 without creating files. An unsupported shell SHALL exit 2. Completion SHALL offer the allowed values of `--store`, `--platform` and `--shell`.

#### Scenario: Generate zsh completion
- **WHEN** the user runs `agent-cli completion zsh`
- **THEN** stdout is a zsh completion script for `agent-cli` and the exit code is 0

#### Scenario: Unsupported shell
- **WHEN** the user runs `agent-cli completion tcsh`
- **THEN** stderr starts with `error: ` and the exit code is 2

#### Scenario: Complete flag values
- **WHEN** the shell asks the completion engine for values of `agent-cli connect --store `
- **THEN** it suggests `keychain`, `passphrase` and `unsafe`

### Requirement: Informational commands have no side effects
`--help`, `--version`, `help`, `env` and `completion` SHALL NOT create or modify the home directory, read credentials, or contact the network.

#### Scenario: Fresh machine
- **GIVEN** `THE_AGENT_CLI_HOME` points to a directory that does not exist
- **WHEN** the user runs `agent-cli --version`, `agent-cli --help`, `agent-cli help notify`, `agent-cli env` and `agent-cli completion bash`
- **THEN** the directory still does not exist afterwards

### Requirement: Secret prompts show a mask
Prompts for secrets (passphrases) SHALL echo one `*` per typed character instead of the character, SHALL support Backspace, SHALL finish on Enter, and SHALL NOT write the typed text to any stream. Prompt text and mask go to stderr.

#### Scenario: Typing a passphrase
- **GIVEN** an interactive terminal
- **WHEN** the user types `abc` and presses Enter at a passphrase prompt
- **THEN** stderr shows the prompt followed by `***` and a line break
- **AND** the command receives the secret `abc`

#### Scenario: Correcting a typo
- **WHEN** the user types `abcd`, presses Backspace, then Enter
- **THEN** the mask shrinks by one `*` and the command receives `abc`

#### Scenario: Non-ASCII characters
- **WHEN** the user types the eight characters `pässword`
- **THEN** exactly eight `*` are shown and the command receives `pässword`

#### Scenario: Nothing leaks
- **WHEN** the user typed the secret `correct horse` at a prompt
- **THEN** neither stdout nor stderr contains the text `correct horse`

### Requirement: Interrupted prompts restore the terminal
When the user interrupts a masked-input or confirmation prompt (Ctrl-C), the CLI SHALL restore the terminal to its previous state, print `Aborted.` to stderr, and exit with code 1.

#### Scenario: Interrupt during passphrase prompt
- **GIVEN** a passphrase prompt is waiting on a terminal
- **WHEN** the user presses Ctrl-C
- **THEN** terminal echo is back on, stderr shows `Aborted.`, and the exit code is 1

### Requirement: Windows console shows Unicode correctly
On Windows, when stdout or stderr is a console, the CLI SHALL switch the console output code page to UTF-8 before printing anything and SHALL restore the previous code page when it exits, so that `✓`, `·` and non-ASCII titles render correctly. There SHALL be no ASCII fallback.

#### Scenario: Legacy code page
- **GIVEN** a Windows console that uses code page 437
- **WHEN** the user runs `agent-cli status` and the report contains `·`
- **THEN** the character renders correctly
- **AND** after the process exits the console code page is 437 again

#### Scenario: Redirected output
- **GIVEN** stdout and stderr are redirected to files on Windows
- **WHEN** the user runs any command
- **THEN** the console code page is not changed
