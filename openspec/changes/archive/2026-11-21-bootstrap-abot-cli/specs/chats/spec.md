# Spec Delta

## Purpose

Defines `agent-cli chats` and its subcommands: listing the chats the user can send notifications to, showing the default target, and setting or clearing it.

## ADDED Requirements

### Requirement: List chats
`agent-cli chats` SHALL fetch the user's chats from the backend and print a table on stdout with a header row `ID`, `PLATFORM`, `TITLE`, `PRIVATE` followed by `(* = default)`, one row per chat in the order the backend returns them, and exit 0. Titles SHALL be printed in full.

#### Scenario: Table with a default chat
- **GIVEN** the backend returns three chats and the stored default chat is the first one
- **WHEN** the user runs `agent-cli chats`
- **THEN** stdout is exactly
  ```
    ID                                PLATFORM  TITLE     PRIVATE   (* = default)
  * 3f2a9c1e7b6d4a58b0c1d2e3f4a5b6c7  telegram  Morpheus  yes
    9b1c0d2e3f4a5b6c7d8e9f0a1b2c3d4e  whatsapp  Morpheus  yes
    c0de1234c0de1234c0de1234c0de1234  telegram  Dev Team  no
  ```
- **AND** the exit code is 0

#### Scenario: Long titles are not truncated
- **GIVEN** a chat titled with 80 characters
- **WHEN** the user runs `agent-cli chats`
- **THEN** the full title appears in its row

#### Scenario: Chat without a title
- **GIVEN** the backend returns a chat whose title is null
- **WHEN** the user runs `agent-cli chats`
- **THEN** its title column shows `untitled`

#### Scenario: Unknown platform value
- **GIVEN** the backend returns a chat with platform `matrix`
- **WHEN** the user runs `agent-cli chats`
- **THEN** its platform column shows `matrix`

### Requirement: Default marker
A row SHALL be marked with `*` when its chat id equals the effective default chat (`THE_AGENT_CLI_DEFAULT_CHAT` first, then the stored default). A default platform SHALL NOT mark any row.

#### Scenario: Environment default marks the row
- **GIVEN** `THE_AGENT_CLI_DEFAULT_CHAT=9b1c0d2e3f4a5b6c7d8e9f0a1b2c3d4e` and a stored default chat `3f2a9c1e7b6d4a58b0c1d2e3f4a5b6c7`
- **WHEN** the user runs `agent-cli chats`
- **THEN** only the row of `9b1c0d2e3f4a5b6c7d8e9f0a1b2c3d4e` starts with `*`

#### Scenario: Platform default marks nothing
- **GIVEN** a stored default platform `whatsapp`
- **WHEN** the user runs `agent-cli chats`
- **THEN** no row starts with `*`

### Requirement: Empty list
When the backend returns no chats the CLI SHALL print `No chats found. Start a conversation with the agent on Telegram or WhatsApp, then try again.` on stdout and exit 0.

#### Scenario: No chats yet
- **GIVEN** the backend returns an empty array
- **WHEN** the user runs `agent-cli chats`
- **THEN** stdout is `No chats found. Start a conversation with the agent on Telegram or WhatsApp, then try again.` and the exit code is 0

### Requirement: Listing follows the shared failure rules
Authentication, network, rate-limit and server failures while listing chats SHALL behave as defined for the backend client.

#### Scenario: Token revoked
- **GIVEN** the backend answers `401`
- **WHEN** the user runs `agent-cli chats`
- **THEN** stderr is ``This CLI is not connected. Run `agent-cli connect <KEY>`.`` and the exit code is 3

#### Scenario: Not connected
- **GIVEN** a machine that is not connected
- **WHEN** the user runs `agent-cli chats`
- **THEN** the same guidance is printed, the exit code is 3 and no request was sent

### Requirement: Show the default
`agent-cli chats default` SHALL work offline and print the current default and a tip on stdout, exit 0. A chat default prints `Default chat: <id> (<platform> · <title>, private|group)`, a platform default prints `Default platform: <platform>`, and no default prints `No default chat is set; notifications go to your latest chat.` Environment defaults SHALL be marked with their variable name.

#### Scenario: Stored chat
- **GIVEN** a stored default chat `3f2a9c1e7b6d4a58b0c1d2e3f4a5b6c7` (telegram, `Morpheus`, private)
- **WHEN** the user runs `agent-cli chats default`
- **THEN** stdout is exactly
  ```
  Default chat: 3f2a9c1e7b6d4a58b0c1d2e3f4a5b6c7 (telegram · Morpheus, private)
  Tip: change it with `agent-cli chats default --set <chat-id> | --platform telegram|whatsapp | --clear`.
  ```

#### Scenario: Stored platform
- **GIVEN** a stored default platform `whatsapp`
- **WHEN** the user runs `agent-cli chats default`
- **THEN** the first line is `Default platform: whatsapp` followed by the same tip

#### Scenario: No default
- **GIVEN** no default anywhere
- **WHEN** the user runs `agent-cli chats default`
- **THEN** the first line is `No default chat is set; notifications go to your latest chat.` followed by the same tip

#### Scenario: Environment default
- **GIVEN** `THE_AGENT_CLI_DEFAULT_PLATFORM=telegram`
- **WHEN** the user runs `agent-cli chats default`
- **THEN** the first line is `Default platform: telegram (from THE_AGENT_CLI_DEFAULT_PLATFORM)`

### Requirement: Set the default chat
`agent-cli chats default --set <chat-id>` SHALL fetch the chat list, require that the id is one of the user's chats (case-insensitive), store the chat's id, platform, title and privacy as the only default, print `✓ Default chat set to <id> (<platform> · <title>, private|group)` and exit 0. An unknown id SHALL exit 2.

#### Scenario: Known chat
- **GIVEN** the backend lists chat `9b1c0d2e3f4a5b6c7d8e9f0a1b2c3d4e` (whatsapp, `Morpheus`, private) and a default platform `telegram` is stored
- **WHEN** the user runs `agent-cli chats default --set 9B1C0D2E3F4A5B6C7D8E9F0A1B2C3D4E`
- **THEN** stdout is `✓ Default chat set to 9b1c0d2e3f4a5b6c7d8e9f0a1b2c3d4e (whatsapp · Morpheus, private)`
- **AND** the config holds that chat as the default and no default platform

#### Scenario: Unknown chat
- **GIVEN** the backend does not list chat `nope`
- **WHEN** the user runs `agent-cli chats default --set nope`
- **THEN** stderr is ``error: chat "nope" is not one of your chats; run `agent-cli chats` to list them``
- **AND** the exit code is 2 and the stored default is unchanged

#### Scenario: Not connected
- **GIVEN** a machine that is not connected
- **WHEN** the user runs `agent-cli chats default --set 3f2a9c1e7b6d4a58b0c1d2e3f4a5b6c7`
- **THEN** the not-connected guidance is printed and the exit code is 3

### Requirement: Set the default platform
`agent-cli chats default --platform telegram|whatsapp` SHALL work offline, store the platform as the only default (case-insensitive input), print `✓ Default platform set to <platform>` and exit 0. Other values SHALL exit 2.

#### Scenario: Valid platform
- **GIVEN** a stored default chat
- **WHEN** the user runs `agent-cli chats default --platform WhatsApp`
- **THEN** stdout is `✓ Default platform set to whatsapp`, the config holds that platform as the default and no default chat, and no request was sent

#### Scenario: Invalid platform
- **WHEN** the user runs `agent-cli chats default --platform slack`
- **THEN** stderr is `error: invalid --platform "slack": use telegram or whatsapp` and the exit code is 2

### Requirement: Clear the default
`agent-cli chats default --clear` SHALL work offline, remove any stored default, print `✓ Default cleared` and exit 0, also when nothing was stored.

#### Scenario: Clear
- **GIVEN** a stored default chat
- **WHEN** the user runs `agent-cli chats default --clear`
- **THEN** stdout is `✓ Default cleared` and the config has no default

### Requirement: One form at a time
`agent-cli chats default` SHALL display the effective default when no mutation flags are given. Mutations SHALL require exactly one of `--set <chat-id>`, `--platform` or `--clear`; conflicting flags, empty `--set` and positional arguments SHALL exit 2 and change nothing.

#### Scenario: Empty chat id
- **WHEN** the user runs `agent-cli chats default --set ""`
- **THEN** stderr is `error: choose one of --set <chat-id>, --platform, or --clear, or omit flags to show the default` and the exit code is 2

#### Scenario: Two forms
- **WHEN** the user runs `agent-cli chats default --clear --platform telegram`
- **THEN** the same error is printed and the exit code is 2

### Requirement: Environment defaults still win
When `THE_AGENT_CLI_DEFAULT_CHAT` or `THE_AGENT_CLI_DEFAULT_PLATFORM` is set, mutations under `chats default` SHALL still update the stored default and SHALL warn on stderr that the variable overrides it.

#### Scenario: Warning about the override
- **GIVEN** `THE_AGENT_CLI_DEFAULT_CHAT=c7`
- **WHEN** the user runs `agent-cli chats default --platform telegram`
- **THEN** stderr is `warning: THE_AGENT_CLI_DEFAULT_CHAT is set and overrides the stored default` and the exit code is 0
