# Spec Delta

## Purpose

Defines `agent-cli revoke-all`, which revokes the access of every CLI the user has connected on every machine and removes the token stored on this machine.

## ADDED Requirements

### Requirement: Confirmation before revoking
On an interactive terminal `agent-cli revoke-all` SHALL ask `This revokes access for ALL CLIs on all your machines. Continue? [y/N] ` on stderr and proceed only when the answer is `y` or `yes` in any letter case. Any other answer, including an empty one or end of input, SHALL print `Aborted.` on stderr, exit with code 1 and change nothing.

#### Scenario: Confirmed
- **GIVEN** a connected machine and an interactive terminal
- **WHEN** the user runs `agent-cli revoke-all` and answers `y`
- **THEN** the revoke request is sent

#### Scenario: Default answer is no
- **GIVEN** a connected machine and an interactive terminal
- **WHEN** the user runs `agent-cli revoke-all` and presses Enter
- **THEN** stderr ends with `Aborted.`, the exit code is 1, no request was sent and the local token is kept

#### Scenario: Explicit no
- **WHEN** the user answers `n`
- **THEN** the command aborts the same way

### Requirement: Non-interactive use needs --yes
`--yes` SHALL skip the confirmation. Without a terminal and without `--yes` the command SHALL refuse with exit code 2 before any prompt or request.

#### Scenario: Scripted revoke
- **GIVEN** a connected machine and no terminal
- **WHEN** the user runs `agent-cli revoke-all --yes`
- **THEN** no prompt is shown and the revoke request is sent

#### Scenario: Refusing without confirmation
- **GIVEN** a connected machine and no terminal
- **WHEN** the user runs `agent-cli revoke-all`
- **THEN** stderr is `error: refusing to revoke without confirmation; pass --yes`
- **AND** the exit code is 2 and no request was sent

### Requirement: Successful revocation
After the backend confirms, the CLI SHALL print `✓ Access revoked for all CLIs. Local token removed.` to stdout, remove the local token and exit 0.

#### Scenario: Revoke with --yes
- **GIVEN** a keychain machine with a default chat and the backend answers `{"status": "OK"}`
- **WHEN** the user runs `agent-cli revoke-all --yes`
- **THEN** stdout is `✓ Access revoked for all CLIs. Local token removed.` and the exit code is 0
- **AND** the request was `DELETE /cli/user/<user id>/devices` with the stored token
- **AND** the credential store entry is gone, while the device id and default chat remain in `config.toml`

### Requirement: Already revoked counts as success
When the backend rejects the token on this call (401, 404 with error code 2001, or a token-level 403) the CLI SHALL print `✓ Already revoked. Local token removed.`, remove the local token and exit 0.

#### Scenario: Token was already revoked
- **GIVEN** a connected machine and the backend answers `401` with error code `4005`
- **WHEN** the user runs `agent-cli revoke-all --yes`
- **THEN** stdout is `✓ Already revoked. Local token removed.`, the exit code is 0 and the local token is removed

#### Scenario: Forbidden
- **GIVEN** the backend answers `403` with error code `3002`
- **WHEN** the user runs `agent-cli revoke-all --yes`
- **THEN** the same line is printed and the exit code is 0

#### Scenario: Merged-away account
- **GIVEN** the backend answers `404` with error code `2001`
- **WHEN** the user runs `agent-cli revoke-all --yes`
- **THEN** the same line is printed and the exit code is 0

### Requirement: Unreachable backend keeps the token
When the backend cannot be reached or answers with a 5xx, the CLI SHALL print `error: could not reach the server; local token kept`, exit 1 and keep the local token. A 429 SHALL print `error: rate limited; wait a moment and try again`, exit 6 and keep the local token.

#### Scenario: Network failure
- **GIVEN** nothing is listening at the API URL
- **WHEN** the user runs `agent-cli revoke-all --yes`
- **THEN** stderr is `error: could not reach the server; local token kept`, the exit code is 1 and the token is still stored

#### Scenario: Server error
- **GIVEN** the backend answers `503`
- **WHEN** the user runs `agent-cli revoke-all --yes`
- **THEN** the same message is printed and the exit code is 1

#### Scenario: Rate limited
- **GIVEN** the proxy answers `429`
- **WHEN** the user runs `agent-cli revoke-all --yes`
- **THEN** the exit code is 6 and the token is still stored

### Requirement: Needs a connection first
Without a token the command SHALL print the not-connected guidance and exit 3 before showing any prompt. For a passphrase-protected token the confirmation SHALL be asked before the passphrase, and the unlocking rules of credential storage apply.

#### Scenario: Not connected
- **GIVEN** a machine that is not connected
- **WHEN** the user runs `agent-cli revoke-all`
- **THEN** stderr is ``This CLI is not connected. Run `agent-cli connect <KEY>`.`` and the exit code is 3
- **AND** no prompt was shown

#### Scenario: Locked token without terminal
- **GIVEN** a passphrase machine, no terminal, no `THE_AGENT_CLI_PASSPHRASE` and no `THE_AGENT_CLI_TOKEN`
- **WHEN** the user runs `agent-cli revoke-all --yes`
- **THEN** the locked-token error is printed and the exit code is 3

#### Scenario: Locked token on a terminal
- **GIVEN** a passphrase machine on an interactive terminal
- **WHEN** the user runs `agent-cli revoke-all`
- **THEN** the confirmation prompt appears first and the passphrase prompt only after the user confirmed

### Requirement: Environment token
When `THE_AGENT_CLI_TOKEN` is set, the revoke call SHALL use it. On success the stored token, if any, SHALL be removed too because every token is now revoked. If the environment token is rejected, the command SHALL print `error: the token in THE_AGENT_CLI_TOKEN was rejected by the server (expired or revoked)`, exit 3 and leave stored credentials untouched.

#### Scenario: Revoke with an environment token
- **GIVEN** `THE_AGENT_CLI_TOKEN` is set and a stored token exists
- **WHEN** the user runs `agent-cli revoke-all --yes` and the backend confirms
- **THEN** stdout is `✓ Access revoked for all CLIs. Local token removed.` and the stored token is removed

#### Scenario: Environment token without stored token
- **GIVEN** `THE_AGENT_CLI_TOKEN` is set and nothing is stored
- **WHEN** the backend confirms the revocation
- **THEN** stdout is `✓ Access revoked for all CLIs.`

#### Scenario: Environment token rejected
- **GIVEN** `THE_AGENT_CLI_TOKEN` is set, a different stored token exists and the backend answers `401`
- **WHEN** the user runs `agent-cli revoke-all --yes`
- **THEN** stderr is `error: the token in THE_AGENT_CLI_TOKEN was rejected by the server (expired or revoked)`
- **AND** the exit code is 3 and the stored token is still present
