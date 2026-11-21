# Spec Delta

## Purpose

Defines `agent-cli status`, the offline report of whether and how this machine is connected, which default chat applies, and which backend the CLI talks to.

## ADDED Requirements

### Requirement: Status is offline and read-only
`agent-cli status` SHALL read only local state. It SHALL NOT contact the network, prompt for anything, read a passphrase, access the system credential store, or modify any file.

#### Scenario: Works without a network
- **GIVEN** a connected machine and an unreachable backend
- **WHEN** the user runs `agent-cli status`
- **THEN** the report is printed without any network request

#### Scenario: Locked token is not unlocked
- **GIVEN** a passphrase machine and no terminal
- **WHEN** the user runs `agent-cli status`
- **THEN** no prompt is shown and the passphrase is not read

#### Scenario: Credential store untouched
- **GIVEN** a keychain machine
- **WHEN** the user runs `agent-cli status`
- **THEN** the credential store receives no calls

### Requirement: Connected report
When this machine is connected and the token is usable, `agent-cli status` SHALL print the connection line with the display name and hostname, then indented lines for the device id, the token location with expiry, the default chat, and the backend, and exit 0.

#### Scenario: Keychain machine with a default chat
- **GIVEN** a macOS machine connected as `Morpheus` on host `morpheus-mbp` with the token in the keychain expiring at 2027-02-14 10:21 local time, the clock reading 2026-11-14 10:21, a stored default chat `3f2a9c1e7b6d4a58b0c1d2e3f4a5b6c7` (telegram, `Morpheus`, private), device id `6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b` and API URL `https://agent.cloud.appifyhub.com`
- **WHEN** the user runs `agent-cli status`
- **THEN** stdout is exactly
  ```
  Connected as Morpheus (morpheus-mbp)
    Device ID: 6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b
    Token: macOS Keychain, expires 2027-02-14 10:21 (in 3 months)
    Default chat: 3f2a9c1e7b6d4a58b0c1d2e3f4a5b6c7 (telegram · Morpheus, private)
    Backend: https://agent.cloud.appifyhub.com
  ```
- **AND** the exit code is 0

#### Scenario: Token location per mode
- **WHEN** the token is stored in `passphrase` or `unsafe` mode
- **THEN** the token line starts with `  Token: ~/.agent-cli/credentials (encrypted), expires ` or `  Token: ~/.agent-cli/credentials (NOT encrypted), expires ` respectively

### Requirement: Not connected
Without a connection, `agent-cli status` SHALL print the two guidance lines on stdout and exit 3. A machine whose config says connected but whose `credentials` file is missing SHALL be reported the same way.

#### Scenario: Fresh machine
- **GIVEN** no config file exists and `THE_AGENT_CLI_TOKEN` is unset
- **WHEN** the user runs `agent-cli status`
- **THEN** stdout is exactly
  ```
  Not connected.
  Get a connect key from the agent's chat settings, then run `agent-cli connect <KEY>`.
  ```
- **AND** the exit code is 3

#### Scenario: Credentials file is gone
- **GIVEN** an unsafe machine whose `credentials` file was deleted
- **WHEN** the user runs `agent-cli status`
- **THEN** the same two lines are printed and the exit code is 3

### Requirement: Default target line
The default line SHALL show the effective default: `Default chat: <id> (<platform> · <title>, private|group)` for a stored chat, `Default platform: <platform>` for a platform, and `Default chat: none (latest chat is used)` when none applies. Defaults from the environment SHALL be marked with their variable name.

#### Scenario: Group chat
- **GIVEN** a stored default chat `c9` on telegram titled `Dev Team` that is not private
- **WHEN** the user runs `agent-cli status`
- **THEN** the line is `  Default chat: c9 (telegram · Dev Team, group)`

#### Scenario: Default platform
- **GIVEN** a stored default platform `whatsapp`
- **WHEN** the user runs `agent-cli status`
- **THEN** the line is `  Default platform: whatsapp`

#### Scenario: No default
- **GIVEN** no default anywhere
- **WHEN** the user runs `agent-cli status`
- **THEN** the line is `  Default chat: none (latest chat is used)`

#### Scenario: Environment chat
- **GIVEN** `THE_AGENT_CLI_DEFAULT_CHAT=c7` and a stored default chat `c9`
- **WHEN** the user runs `agent-cli status`
- **THEN** the line is `  Default chat: c7 (from THE_AGENT_CLI_DEFAULT_CHAT)`

#### Scenario: Environment platform
- **GIVEN** `THE_AGENT_CLI_DEFAULT_PLATFORM=telegram`
- **WHEN** the user runs `agent-cli status`
- **THEN** the line is `  Default platform: telegram (from THE_AGENT_CLI_DEFAULT_PLATFORM)`

### Requirement: Locked token
When the stored token is passphrase-protected and `THE_AGENT_CLI_TOKEN` is not set, `agent-cli status` SHALL show the connected report with the token marked `(encrypted, locked)`, add the OS-specific unlock line, and exit 3.

#### Scenario: Locked on macOS or Linux
- **GIVEN** a passphrase machine connected as `Morpheus` and no `THE_AGENT_CLI_TOKEN`
- **WHEN** the user runs `agent-cli status`
- **THEN** the token line is `  Token: ~/.agent-cli/credentials (encrypted, locked), expires 2027-02-14 10:21 (in 3 months)`
- **AND** a further line `  To unlock: eval "$(agent-cli unlock)"` follows the report lines
- **AND** the exit code is 3

#### Scenario: Locked on Windows
- **WHEN** the same machine runs on windows
- **THEN** the unlock line is `  To unlock: agent-cli unlock --shell pwsh | Invoke-Expression`

### Requirement: Expired token
When the stored expiry time has passed, `agent-cli status` SHALL show the token line with `expired on <time> (<relative> ago)`, add a line telling the user to connect again, and exit 3, without changing any file.

#### Scenario: Expired two days ago
- **GIVEN** a keychain machine whose token expired at 2027-02-14 10:21 and the clock reads 2027-02-16 10:21
- **WHEN** the user runs `agent-cli status`
- **THEN** the token line is `  Token: macOS Keychain, expired on 2027-02-14 10:21 (2 days ago)`
- **AND** the next line is ``  Run `agent-cli connect <KEY>` to connect again.``
- **AND** the exit code is 3

### Requirement: Environment token
When `THE_AGENT_CLI_TOKEN` is set, `agent-cli status` SHALL report the connection as coming from it, show the expiry read from the token when present, and ignore stored credentials. A value that is not shaped like a CLI token SHALL exit 3 with `error: THE_AGENT_CLI_TOKEN is not a valid CLI token`.

#### Scenario: Environment token with expiry
- **GIVEN** `THE_AGENT_CLI_TOKEN` holds a token whose `exp` claim is 2027-02-14 10:21 local time, and the clock reads 2026-11-14 10:21
- **WHEN** the user runs `agent-cli status`
- **THEN** the first line is `Connected using THE_AGENT_CLI_TOKEN`
- **AND** the token line is `  Token: THE_AGENT_CLI_TOKEN environment variable, expires 2027-02-14 10:21 (in 3 months)`
- **AND** the exit code is 0

#### Scenario: Expired environment token
- **GIVEN** `THE_AGENT_CLI_TOKEN` holds a token whose `exp` claim has passed
- **WHEN** the user runs `agent-cli status`
- **THEN** the token line says `expired on` and the exit code is 3

#### Scenario: Environment token without expiry
- **GIVEN** `THE_AGENT_CLI_TOKEN` holds a shaped token with a subject and no `exp` claim
- **WHEN** the user runs `agent-cli status`
- **THEN** the token line is `  Token: THE_AGENT_CLI_TOKEN environment variable` without an expiry suffix, and the exit code is 0
- **AND** stored credentials are not accessed and token claims are not treated as proof of server-side validity


#### Scenario: Malformed environment token
- **GIVEN** `THE_AGENT_CLI_TOKEN=garbage`
- **WHEN** the user runs `agent-cli status`
- **THEN** stderr is `error: THE_AGENT_CLI_TOKEN is not a valid CLI token` and the exit code is 3

### Requirement: Backend line
The `Backend:` line SHALL show the effective API URL: `THE_AGENT_CLI_API_URL`, else the stored URL, else the build-time default.

#### Scenario: Environment override
- **GIVEN** a connected machine with stored `api_url` `https://a.test` and `THE_AGENT_CLI_API_URL=https://b.test`
- **WHEN** the user runs `agent-cli status`
- **THEN** the line is `  Backend: https://b.test`
