# Spec Delta

## Purpose

Defines `agent-cli unlock`, which decrypts a passphrase-protected token once and exports it as `THE_AGENT_CLI_TOKEN` into the current terminal session so that later commands need no passphrase, without any background process.

## ADDED Requirements

### Requirement: Unlock for this terminal session
`agent-cli unlock` SHALL ask for the passphrase on stderr (`Passphrase: `, typed characters shown as `*`), decrypt the stored token, print only a single shell assignment of `THE_AGENT_CLI_TOKEN` to stdout, print `✓ Unlocked for this terminal session` to stderr, and exit 0. Evaluating the output in the shell SHALL make later `agent-cli` commands run without a passphrase prompt.

#### Scenario: Unlock on macOS or Linux
- **GIVEN** a passphrase machine whose token is `T` and the user types the right passphrase
- **WHEN** the user runs `eval "$(agent-cli unlock)"` in a POSIX shell
- **THEN** stdout of `agent-cli unlock` was exactly `export THE_AGENT_CLI_TOKEN='T'` followed by a newline
- **AND** stderr showed `Passphrase: ` and `✓ Unlocked for this terminal session`
- **AND** the shell now has `THE_AGENT_CLI_TOKEN` set to `T`

#### Scenario: Later commands do not prompt
- **GIVEN** the shell evaluated the unlock output
- **WHEN** the user runs `agent-cli chats`
- **THEN** no passphrase prompt is shown and the request carries `T`

### Requirement: Shell syntax
`--shell` SHALL select the syntax of the assignment: `sh` (also `bash`, `zsh`): `export THE_AGENT_CLI_TOKEN='T'`; `fish`: `set -gx THE_AGENT_CLI_TOKEN 'T'`; `pwsh` (also `powershell`): `$env:THE_AGENT_CLI_TOKEN = 'T'`. The default SHALL be `pwsh` on Windows and `sh` elsewhere. Single quotes inside a value SHALL be escaped for the chosen shell. Any other value SHALL exit 2.

#### Scenario: PowerShell on Windows
- **GIVEN** the binary runs on windows
- **WHEN** the user runs `agent-cli unlock | Invoke-Expression`
- **THEN** stdout is `$env:THE_AGENT_CLI_TOKEN = 'T'` followed by a newline

#### Scenario: Explicit PowerShell
- **WHEN** the user runs `agent-cli unlock --shell pwsh | Invoke-Expression`
- **THEN** stdout is `$env:THE_AGENT_CLI_TOKEN = 'T'` followed by a newline

#### Scenario: Fish
- **WHEN** the user runs `agent-cli unlock --shell fish`
- **THEN** stdout is `set -gx THE_AGENT_CLI_TOKEN 'T'` followed by a newline

#### Scenario: Quote escaping
- **GIVEN** a stored token that contains a single quote
- **WHEN** the user runs `agent-cli unlock --shell sh`
- **THEN** the single quote is escaped as `'\''` inside the assignment

#### Scenario: Unknown shell
- **WHEN** the user runs `agent-cli unlock --shell csh`
- **THEN** stderr is `error: invalid --shell "csh": use sh, bash, zsh, fish, pwsh, or powershell` and the exit code is 2

### Requirement: Running without eval
When stdout is a terminal, `agent-cli unlock` SHALL still print the assignment on stdout and SHALL add a hint on stderr telling the user how to apply it, using the form for the running OS.

#### Scenario: Hint on macOS or Linux
- **GIVEN** stdout is a terminal on darwin or linux
- **WHEN** the user runs `agent-cli unlock` and enters the right passphrase
- **THEN** the assignment line is printed and stderr contains ``To apply it to this terminal session, run: eval "$(agent-cli unlock)"``

#### Scenario: Hint on Windows
- **GIVEN** stdout is a terminal on windows
- **WHEN** the user runs `agent-cli unlock` and enters the right passphrase
- **THEN** stderr contains ``To apply it to this terminal session, run: agent-cli unlock --shell pwsh | Invoke-Expression``

### Requirement: Passphrase sources
`agent-cli unlock` SHALL use `THE_AGENT_CLI_PASSPHRASE` when set and otherwise prompt on a terminal. Without a terminal and without `THE_AGENT_CLI_PASSPHRASE` it SHALL exit 3 immediately with the locked-token error, never waiting for input.

#### Scenario: Passphrase from the environment
- **GIVEN** `THE_AGENT_CLI_PASSPHRASE` holds the right passphrase
- **WHEN** the user runs `eval "$(agent-cli unlock)"`
- **THEN** no prompt is shown and `THE_AGENT_CLI_TOKEN` is set

#### Scenario: No way to ask
- **GIVEN** stdin is not a terminal and `THE_AGENT_CLI_PASSPHRASE` is unset
- **WHEN** the user runs `agent-cli unlock`
- **THEN** the locked-token error is printed on stderr, stdout is empty and the exit code is 3

### Requirement: Wrong passphrase
A wrong passphrase SHALL print `error: incorrect passphrase` on stderr, leave stdout empty and exit 3.

#### Scenario: Wrong passphrase typed
- **WHEN** the user types a wrong passphrase
- **THEN** stderr is `error: incorrect passphrase`, stdout is empty and the exit code is 3

### Requirement: Nothing to unlock
When the stored token is not passphrase-protected, or `THE_AGENT_CLI_TOKEN` is already set, `agent-cli unlock` SHALL leave stdout empty, explain on stderr, and exit 0.

#### Scenario: Keychain machine
- **GIVEN** a machine connected with `--store keychain`
- **WHEN** the user runs `agent-cli unlock`
- **THEN** stderr is `Nothing to unlock: your token is not passphrase-protected.`, stdout is empty and the exit code is 0

#### Scenario: Already unlocked
- **GIVEN** `THE_AGENT_CLI_TOKEN` is set in the environment
- **WHEN** the user runs `agent-cli unlock`
- **THEN** stderr is `Already unlocked: THE_AGENT_CLI_TOKEN is set in this environment.`, stdout is empty and the exit code is 0

### Requirement: Needs a connection
Without a stored connection and without `THE_AGENT_CLI_TOKEN`, `agent-cli unlock` SHALL print the not-connected guidance on stderr, leave stdout empty and exit 3.

#### Scenario: Not connected
- **GIVEN** a machine that is not connected
- **WHEN** the user runs `agent-cli unlock`
- **THEN** stderr is ``This CLI is not connected. Run `agent-cli connect <KEY>`.``, stdout is empty and the exit code is 3

### Requirement: Output is safe to evaluate
Everything `agent-cli unlock` writes to stdout SHALL be the single assignment line. Prompts, confirmations, hints, warnings and errors SHALL go to stderr so that evaluating stdout can never run unintended text.

#### Scenario: Failure leaves nothing to evaluate
- **WHEN** `agent-cli unlock` fails for any reason
- **THEN** stdout is empty
