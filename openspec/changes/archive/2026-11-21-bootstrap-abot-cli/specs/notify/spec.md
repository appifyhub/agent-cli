# Spec Delta

## Purpose

Defines `agent-cli notify`, which sends a short text notification to one of the user's chats through the agent, with a clear precedence for choosing the target, stdin input for pipelines, and distinguishable failures.

## ADDED Requirements

### Requirement: Send a notification silently
`agent-cli notify "<message>"` SHALL send the message to the user's chat chosen by the target rules and, on success, print nothing on either stream (apart from the low credit warning below) and exit 0.

#### Scenario: Plain notification
- **GIVEN** a connected machine, no defaults and a backend that accepts the message and reports `credit_balance` 500
- **WHEN** the user runs `agent-cli notify "Build finished"`
- **THEN** the CLI sends `POST /cli/notify/user/<user id>` with `{"message": "Build finished", "platform": null}`
- **AND** stdout and stderr are empty and the exit code is 0

### Requirement: Message sources
The message SHALL be the single positional argument. When no argument is given and stdin is not a terminal, the message SHALL be read from stdin, limited to 4096 characters. An argument takes precedence over stdin. With no argument and a terminal on stdin the command SHALL exit 2 instead of waiting.

#### Scenario: Message from the command line
- **WHEN** the user runs `agent-cli notify "Dinner's ready"`
- **THEN** the request carries the message `Dinner's ready`

#### Scenario: Message from a pipe
- **GIVEN** stdin carries the text `make: *** [all] Error 2` followed by a newline
- **WHEN** the user runs `agent-cli notify` with no argument
- **THEN** the request carries the message `make: *** [all] Error 2`

#### Scenario: Argument beats stdin
- **GIVEN** stdin carries `from the pipe`
- **WHEN** the user runs `agent-cli notify "from the argument"`
- **THEN** the request carries `from the argument` and stdin is not read

#### Scenario: Nothing to send on a terminal
- **GIVEN** stdin is a terminal
- **WHEN** the user runs `agent-cli notify` with no argument
- **THEN** stderr is `error: no message given; pass it as an argument or pipe it on stdin`
- **AND** the exit code is 2 and no request was sent

#### Scenario: Oversized pipe input
- **GIVEN** stdin carries 5000 characters
- **WHEN** the user runs `agent-cli notify` with no argument
- **THEN** the request carries the first 4096 characters
- **AND** stderr is `warning: input truncated to 4096 characters` and the exit code is 0

#### Scenario: Oversized argument
- **WHEN** the user passes a 5000-character argument
- **THEN** stderr is `error: message is too long (max 4096 characters)`, the exit code is 2 and no request was sent

#### Scenario: Unquoted words
- **WHEN** the user runs `agent-cli notify Build finished`
- **THEN** stderr is `error: pass the message as a single quoted argument`, the exit code is 2 and no request was sent

### Requirement: Messages are trimmed and never empty
Surrounding whitespace and a leading byte-order mark SHALL be removed before sending. A message that is empty afterwards SHALL exit 2 with `error: message is empty` and no request.

#### Scenario: Whitespace only
- **WHEN** the user runs `agent-cli notify "   "`
- **THEN** stderr is `error: message is empty`, the exit code is 2 and no request was sent

#### Scenario: Trimmed input
- **GIVEN** stdin carries two blank lines, then `done`, then a blank line
- **WHEN** the user runs `agent-cli notify` with no argument
- **THEN** the request carries the message `done`

#### Scenario: Multi-line message
- **WHEN** the user passes a message with an interior newline
- **THEN** the newline is preserved in the request

### Requirement: Target precedence
The target SHALL be chosen in this order: `--chat` or `--platform`, then `THE_AGENT_CLI_DEFAULT_CHAT`, then `THE_AGENT_CLI_DEFAULT_PLATFORM`, then the stored default (chat or platform), then none (the backend picks the latest chat). A chat target SHALL use `POST /cli/notify/chat/{chat_id}`; any other target SHALL use `POST /cli/notify/user/{user_id}` with the platform or null.

#### Scenario: Explicit chat
- **WHEN** the user runs `agent-cli notify --chat c1 "Dinner's ready"`
- **THEN** the request is `POST /cli/notify/chat/c1` with `{"message": "Dinner's ready"}`

#### Scenario: Explicit platform
- **WHEN** the user runs `agent-cli notify --platform whatsapp "Deploy done"`
- **THEN** the request is `POST /cli/notify/user/<user id>` with `"platform": "whatsapp"`

#### Scenario: Flag beats environment chat
- **GIVEN** `THE_AGENT_CLI_DEFAULT_CHAT=c7`
- **WHEN** the user runs `agent-cli notify --platform telegram "hi"`
- **THEN** the request goes to the user endpoint with platform `telegram`

#### Scenario: Flag beats environment platform
- **GIVEN** `THE_AGENT_CLI_DEFAULT_PLATFORM=telegram`
- **WHEN** the user runs `agent-cli notify --chat c1 "hi"`
- **THEN** the request goes to `/cli/notify/chat/c1`

#### Scenario: Environment chat beats environment platform
- **GIVEN** `THE_AGENT_CLI_DEFAULT_CHAT=c7` and `THE_AGENT_CLI_DEFAULT_PLATFORM=telegram`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** the request goes to `/cli/notify/chat/c7`

#### Scenario: Environment platform beats stored default
- **GIVEN** `THE_AGENT_CLI_DEFAULT_PLATFORM=whatsapp` and a stored default chat `c9`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** the request goes to the user endpoint with platform `whatsapp`

#### Scenario: Stored default chat
- **GIVEN** a stored default chat `c9` and no environment defaults
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** the request goes to `/cli/notify/chat/c9`

#### Scenario: Stored default platform
- **GIVEN** a stored default platform `telegram` and no environment defaults
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** the request goes to the user endpoint with platform `telegram`

#### Scenario: No default
- **GIVEN** no default anywhere
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** the request goes to the user endpoint with `"platform": null`

### Requirement: Conflicting and invalid targets
`--chat` together with `--platform` SHALL exit 2 with `error: use either --chat or --platform, not both`. An unsupported `--platform` value or an empty `--chat` SHALL exit 2. No request SHALL be sent in these cases.

#### Scenario: Both flags
- **WHEN** the user runs `agent-cli notify --chat c1 --platform telegram "hi"`
- **THEN** stderr is `error: use either --chat or --platform, not both`, the exit code is 2 and no request was sent

#### Scenario: Unsupported platform
- **WHEN** the user runs `agent-cli notify --platform slack "hi"`
- **THEN** stderr is `error: invalid --platform "slack": use telegram or whatsapp` and the exit code is 2

#### Scenario: Empty chat id
- **WHEN** the user runs `agent-cli notify --chat "" "hi"`
- **THEN** stderr is `error: --chat needs a chat id` and the exit code is 2

### Requirement: Verbose confirmation
With `--verbose` the CLI SHALL print `Calling backend…` on stderr before the request and, after success, `✓ Sent to <Platform> · <title> (private|group)` on stdout using the chat the backend reports, with the platform shown as `Telegram` or `WhatsApp`.

#### Scenario: Verbose send to WhatsApp
- **GIVEN** the backend answers with a private WhatsApp chat titled `Morpheus`
- **WHEN** the user runs `agent-cli notify --platform whatsapp "Deploy done" -v`
- **THEN** stderr is `Calling backend…`
- **AND** stdout is `✓ Sent to WhatsApp · Morpheus (private)`

#### Scenario: Verbose send to a group
- **GIVEN** the backend answers with a Telegram chat titled `Dev Team` that is not private
- **WHEN** the user runs `agent-cli notify --chat c9 "Deploy done" -v`
- **THEN** stdout is `✓ Sent to Telegram · Dev Team (group)`

### Requirement: Low credit warning after notifying
After a successful notification, when the backend reports a `credit_balance` below 100 credits, the CLI SHALL print `warning: low credit balance: <credits> credits left; top up in the agent's chat settings` to stderr, even though the command is otherwise silent. Amounts SHALL be shown only in credits, with no currency conversion. The exit code SHALL stay 0. A missing, null or non-numeric balance SHALL be ignored.

#### Scenario: Silent command with a low balance
- **GIVEN** the backend accepts the message and reports `credit_balance` 42.37
- **WHEN** the user runs `agent-cli notify "Build finished"`
- **THEN** stdout is empty
- **AND** stderr is `warning: low credit balance: 42.37 credits left; top up in the agent's chat settings`
- **AND** the exit code is 0

#### Scenario: Healthy balance
- **GIVEN** the backend reports `credit_balance` 500
- **WHEN** the user runs `agent-cli notify "Build finished"`
- **THEN** stdout and stderr are empty

#### Scenario: Threshold
- **WHEN** the backend reports `credit_balance` 100
- **THEN** no warning is printed
- **AND** with `credit_balance` 99.99 the warning shows `99.99 credits left`

#### Scenario: Together with verbose output
- **GIVEN** the backend reports `credit_balance` 42.37 and a private WhatsApp chat titled `Morpheus`
- **WHEN** the user runs `agent-cli notify --platform whatsapp "Deploy done" -v`
- **THEN** stderr shows `Calling backend…` and then the warning
- **AND** stdout is `✓ Sent to WhatsApp · Morpheus (private)`

#### Scenario: No warning on failure
- **GIVEN** the backend answers `422` with error code `1034`
- **WHEN** the user runs `agent-cli notify "Build finished"`
- **THEN** stderr holds only the insufficient-credits error and the exit code is 5

### Requirement: A successful status means the message was sent
Any 2xx answer to a notify call SHALL count as sent: exit code 0 and no error, even when the body is empty or unreadable. Unusable parts of the body SHALL only reduce what `--verbose` can show.

#### Scenario: Empty body
- **GIVEN** the backend answers `200` with an empty body
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** nothing is printed and the exit code is 0

#### Scenario: Verbose without chat details
- **GIVEN** the backend answers `200` with `{}`
- **WHEN** the user runs `agent-cli notify "hi" -v`
- **THEN** stdout is `✓ Sent`

### Requirement: Distinguishable failures
Failures SHALL be distinguishable by exit code: usage or validation 2, not connected or locked 3 (a rejected stored token is removed locally), chat unreachable 4, insufficient credits 5, rate limited 6, any other failure 1. Messages SHALL be those defined for the backend client.

#### Scenario: Not connected
- **GIVEN** a machine that is not connected
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** stderr is ``This CLI is not connected. Run `agent-cli connect <KEY>`.``, the exit code is 3 and no request was sent

#### Scenario: Token revoked
- **GIVEN** the backend answers `401` with error code `4005`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** stderr is ``This CLI is not connected. Run `agent-cli connect <KEY>`.``, the exit code is 3 and the local token is removed

#### Scenario: Locked and no terminal
- **GIVEN** a passphrase machine, no terminal and no passphrase source
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** stderr is ``error: token is locked; set THE_AGENT_CLI_PASSPHRASE or run `eval "$(agent-cli unlock)"` `` on macOS and Linux and the exit code is 3

#### Scenario: Chat unreachable
- **GIVEN** the backend answers `403` with error code `3010`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** the exit code is 4 and the token is kept

#### Scenario: Insufficient credits
- **GIVEN** the backend answers `422` with error code `1034`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** the exit code is 5

#### Scenario: Rate limited
- **GIVEN** the backend answers `429` with error code `6004`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** the exit code is 6

#### Scenario: Message rejected by the server
- **GIVEN** the backend answers `422` with error code `1046`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** stderr is `error: the message was rejected: it must be between 1 and 4096 characters` and the exit code is 2

#### Scenario: Provider could not deliver
- **GIVEN** the backend answers `502` with error code `5018`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** stderr is `error: the platform could not deliver the message; try again later` and the exit code is 1

#### Scenario: Policies not accepted
- **GIVEN** the backend answers `403` with error code `3008`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** stderr is `error: accept the agent's policies in chat settings, then try again`
- **AND** the exit code is 1 and the local token is kept

#### Scenario: Waitlist
- **GIVEN** the backend answers `403` with error code `3007`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** stderr is `error: your account is still on the waitlist; try again once the agent has activated it`
- **AND** the exit code is 1 and the local token is kept
