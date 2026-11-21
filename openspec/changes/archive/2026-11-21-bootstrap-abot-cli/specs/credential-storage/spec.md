# Spec Delta

## Purpose

Defines how the CLI stores, protects, retrieves and removes the user's access token on a machine: the three storage modes, default selection, passphrase unlocking, `THE_AGENT_CLI_TOKEN` precedence, local removal, and the guarantee that secrets never leak.

## ADDED Requirements

### Requirement: Storage modes
The CLI SHALL support three storage modes chosen at connect time with `--store`: `keychain` (the operating system's credential store), `passphrase` (an encrypted `credentials` file in the home directory) and `unsafe` (a plain-text `credentials` file readable only by its owner). The chosen mode SHALL be recorded in `config.toml`.

#### Scenario: Keychain mode
- **GIVEN** the operating system's credential store is usable
- **WHEN** `agent-cli connect K7Q2-M9XA-P3RT --store keychain` succeeds
- **THEN** the token is held by the credential store
- **AND** no `credentials` file exists and `config.toml` records `store = "keychain"`

#### Scenario: Passphrase mode
- **WHEN** `agent-cli connect K7Q2-M9XA-P3RT --store passphrase` succeeds
- **THEN** a `credentials` file exists with mode 0600 on Unix and `config.toml` records `store = "passphrase"`

#### Scenario: Unsafe mode
- **WHEN** `agent-cli connect K7Q2-M9XA-P3RT --store unsafe` succeeds
- **THEN** the `credentials` file holds the token in plain text with mode 0600 on Unix and `config.toml` records `store = "unsafe"`

#### Scenario: Unknown mode
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT --store vault`
- **THEN** stderr is `error: invalid --store "vault": use keychain, passphrase, or unsafe`
- **AND** the exit code is 2 and the server was not contacted

### Requirement: Default storage selection
Without `--store`, the CLI SHALL use `keychain` when the credential store is usable, else `passphrase` when a passphrase can be obtained (interactive terminal or `THE_AGENT_CLI_PASSPHRASE`), else fail with exit code 2 and a message that suggests `--store unsafe` or `THE_AGENT_CLI_TOKEN`. An explicitly requested mode SHALL never be replaced by another one.

#### Scenario: Keychain available
- **GIVEN** the credential store is usable
- **WHEN** the user connects without `--store`
- **THEN** the mode is `keychain` and no passphrase prompt appears

#### Scenario: No keychain but a terminal
- **GIVEN** the credential store is unusable and stdin is a terminal
- **WHEN** the user connects without `--store`
- **THEN** the mode is `passphrase` and the CLI prompts for a passphrase

#### Scenario: No keychain, no terminal, passphrase in the environment
- **GIVEN** the credential store is unusable, stdin is not a terminal and `THE_AGENT_CLI_PASSPHRASE` is set
- **WHEN** the user connects without `--store`
- **THEN** the mode is `passphrase` and the passphrase from the environment is used without prompting

#### Scenario: No keychain and nothing to ask a passphrase with
- **GIVEN** the credential store is unusable, stdin is not a terminal and `THE_AGENT_CLI_PASSPHRASE` is unset
- **WHEN** the user connects without `--store`
- **THEN** stderr is `error: no keychain is available and no passphrase can be requested here; use --store unsafe or provide a token with THE_AGENT_CLI_TOKEN`
- **AND** the exit code is 2 and the server was not contacted

#### Scenario: Explicit keychain is unavailable
- **GIVEN** the credential store is unusable
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT --store keychain`
- **THEN** stderr is `error: no keychain is available on this machine; use --store passphrase or --store unsafe`
- **AND** the exit code is 2 and the server was not contacted

#### Scenario: Explicit passphrase without a way to ask
- **GIVEN** stdin is not a terminal and `THE_AGENT_CLI_PASSPHRASE` is unset
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT --store passphrase`
- **THEN** stderr is `error: a passphrase is required; run in a terminal or set THE_AGENT_CLI_PASSPHRASE`
- **AND** the exit code is 2 and the server was not contacted

### Requirement: Keychain and unsafe modes never prompt
In `keychain` and `unsafe` modes the CLI SHALL NOT ask for a passphrase, neither at connect time nor when the token is used later.

#### Scenario: Connect without prompts
- **WHEN** the user connects with `--store keychain` or `--store unsafe`
- **THEN** no prompt is shown and `THE_AGENT_CLI_PASSPHRASE` is ignored

#### Scenario: Later use without prompts
- **GIVEN** a machine connected in `unsafe` mode
- **WHEN** the user runs `agent-cli chats` with no terminal attached
- **THEN** the command proceeds without any prompt

### Requirement: Passphrase protection
In `passphrase` mode the token SHALL be stored encrypted under a key derived from the user's passphrase. The passphrase SHALL NOT be stored anywhere. Without the correct passphrase the file SHALL reveal no readable part of the token, and a wrong passphrase SHALL fail without any network request.

#### Scenario: Encrypted at rest
- **GIVEN** a machine connected with `--store passphrase`
- **WHEN** the `credentials` file is inspected
- **THEN** it contains no 8-character substring of the token

#### Scenario: Correct passphrase
- **GIVEN** a machine connected with `--store passphrase` and passphrase `correct horse`
- **WHEN** the user runs `agent-cli chats` with `THE_AGENT_CLI_PASSPHRASE=correct horse`
- **THEN** the request carries the original token

#### Scenario: Wrong passphrase
- **GIVEN** a machine connected with `--store passphrase`
- **WHEN** the user runs `agent-cli chats` with a wrong `THE_AGENT_CLI_PASSPHRASE`
- **THEN** stderr is `error: incorrect passphrase`, the exit code is 3 and no request was sent

#### Scenario: Damaged credentials file
- **GIVEN** the `credentials` file of a passphrase machine was truncated
- **WHEN** the user runs `agent-cli chats` with the right passphrase
- **THEN** stderr is ``error: the credentials file is damaged; run `agent-cli connect <KEY>` to connect again`` and the exit code is 3

### Requirement: Token source order
When `THE_AGENT_CLI_TOKEN` is set and non-empty it SHALL be used as the token, and the stored token SHALL NOT be read: no keychain access, no passphrase prompt, no credentials file access. A value that is not shaped like a CLI token SHALL be rejected locally.

#### Scenario: Environment token on a passphrase machine
- **GIVEN** a machine connected with `--store passphrase`, stdin is not a terminal and `THE_AGENT_CLI_TOKEN` is set
- **WHEN** the user runs `agent-cli chats`
- **THEN** the request carries the `THE_AGENT_CLI_TOKEN` value, no prompt is shown, and the exit code is 0

#### Scenario: Empty environment token
- **GIVEN** `THE_AGENT_CLI_TOKEN` is set to an empty string and a stored token exists
- **WHEN** the user runs `agent-cli chats`
- **THEN** the stored token is used

#### Scenario: Malformed environment token
- **GIVEN** `THE_AGENT_CLI_TOKEN=not-a-token`
- **WHEN** the user runs `agent-cli chats`
- **THEN** stderr is `error: THE_AGENT_CLI_TOKEN is not a valid CLI token`, the exit code is 3 and no request was sent

### Requirement: Unlocking a passphrase token
For a passphrase-protected stored token the CLI SHALL take the passphrase from `THE_AGENT_CLI_PASSPHRASE` when set, otherwise prompt with `Passphrase: ` on stderr when stdin is a terminal. Otherwise the token is locked and the CLI SHALL exit 3 immediately, without reading stdin and without waiting.

#### Scenario: Passphrase from the environment
- **GIVEN** a passphrase machine and `THE_AGENT_CLI_PASSPHRASE` set to the right passphrase
- **WHEN** the user runs `agent-cli chats`
- **THEN** no prompt is shown and the command proceeds

#### Scenario: Prompt on a terminal
- **GIVEN** a passphrase machine, stdin is a terminal and `THE_AGENT_CLI_PASSPHRASE` is unset
- **WHEN** the user runs `agent-cli chats` and types the right passphrase
- **THEN** stderr shows the prompt `Passphrase: ` followed by one `*` per typed character, and the command proceeds

#### Scenario: Locked without a terminal on macOS or Linux
- **GIVEN** a passphrase machine, stdin is not a terminal, `THE_AGENT_CLI_PASSPHRASE` and `THE_AGENT_CLI_TOKEN` are unset, on darwin or linux
- **WHEN** the user runs `agent-cli chats`
- **THEN** stderr is ``error: token is locked; set THE_AGENT_CLI_PASSPHRASE or run `eval "$(agent-cli unlock)"` ``
- **AND** the exit code is 3, the command returned immediately and no request was sent

#### Scenario: Locked without a terminal on Windows
- **GIVEN** the same situation on windows
- **WHEN** the user runs `agent-cli chats`
- **THEN** stderr is ``error: token is locked; set THE_AGENT_CLI_PASSPHRASE or run `agent-cli unlock --shell pwsh | Invoke-Expression` `` and the exit code is 3

### Requirement: Removing the local token
Removing the local token SHALL delete the secret from its store and remove the token metadata from `config.toml`, while keeping the device id, API URL and default target. Removal SHALL succeed when nothing is stored.

#### Scenario: Remove from the keychain
- **GIVEN** a keychain machine with a default chat
- **WHEN** the local token is removed
- **THEN** the credential store has no entry, `config.toml` has no token metadata, and the device id and default chat remain

#### Scenario: Remove the credentials file
- **GIVEN** an unsafe machine
- **WHEN** the local token is removed
- **THEN** the `credentials` file no longer exists

#### Scenario: Removal is idempotent
- **WHEN** the local token is removed a second time
- **THEN** no error occurs

### Requirement: Missing stored secret
When `config.toml` says the machine is connected but the secret is absent from its store, commands that need the token SHALL treat the machine as not connected: print the not-connected guidance, clear the token metadata, and exit 3.

#### Scenario: Keychain entry was deleted
- **GIVEN** a keychain machine whose entry the user deleted in the system keychain app
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** stderr is ``This CLI is not connected. Run `agent-cli connect <KEY>`.``
- **AND** the exit code is 3 and `config.toml` no longer has token metadata

#### Scenario: Credentials file was deleted
- **GIVEN** an unsafe machine whose `credentials` file was deleted
- **WHEN** the user runs `agent-cli chats`
- **THEN** the same guidance is printed and the exit code is 3

### Requirement: Credential store failures keep the token
When the credential store fails for a reason other than a missing entry (locked, unavailable, access denied), the CLI SHALL exit 1 with `error: could not read the token from <store>: <reason>` and SHALL keep the stored token and its metadata.

#### Scenario: Keychain read error
- **GIVEN** a keychain machine whose credential store returns an access error
- **WHEN** the user runs `agent-cli chats`
- **THEN** stderr starts with `error: could not read the token from ` and the exit code is 1
- **AND** the token metadata in `config.toml` is unchanged

### Requirement: Connecting again replaces the secret
Connecting a machine that already has a stored token SHALL replace it and SHALL remove any secret left in a previously used store.

#### Scenario: Switch from unsafe to keychain
- **GIVEN** a machine connected with `--store unsafe`
- **WHEN** the user connects again with `--store keychain`
- **THEN** the `credentials` file is deleted and the token is in the credential store

#### Scenario: Switch from keychain to passphrase
- **GIVEN** a machine connected with `--store keychain`
- **WHEN** the user connects again with `--store passphrase`
- **THEN** the credential store entry is deleted and an encrypted `credentials` file exists

### Requirement: Secrets never leak
Tokens, passphrases and connect keys SHALL NOT appear in config files, logs, verbose output, error messages, or process arguments of child programs. The only command that prints a token is `agent-cli unlock`.

#### Scenario: Verbose failure
- **GIVEN** the backend rejects a connect with a server error
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT --store passphrase -v` with `THE_AGENT_CLI_PASSPHRASE=hunter2`
- **THEN** neither stdout nor stderr contains `K7Q2-M9XA-P3RT` or `hunter2`

#### Scenario: Token never printed by other commands
- **GIVEN** a connected machine
- **WHEN** the user runs `agent-cli status`, `agent-cli chats` and `agent-cli notify "hi"` with `-v`
- **THEN** no output contains the token text
