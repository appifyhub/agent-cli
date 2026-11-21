# Spec Delta

## Purpose

Defines `agent-cli connect`, which links this machine to the user's profile with a connect key from the agent, stores the issued token in the chosen storage mode, and reports the result.

## ADDED Requirements

### Requirement: Connect usage
`agent-cli connect <KEY>` SHALL accept exactly one connect key and the optional flags `--ttl <text>` and `--store keychain|passphrase|unsafe`. The key SHALL be trimmed and uppercased before it is sent. A missing key or extra arguments SHALL exit with code 2.

#### Scenario: Missing key
- **WHEN** the user runs `agent-cli connect`
- **THEN** stderr is `error: missing connect key; usage: agent-cli connect <KEY> [--ttl <text>] [--store keychain|passphrase|unsafe]`
- **AND** the exit code is 2 and no request was sent

#### Scenario: Key normalization
- **WHEN** the user runs `agent-cli connect " k7q2-m9xa-p3rt "`
- **THEN** the request carries `connect_key` `K7Q2-M9XA-P3RT`

#### Scenario: Extra argument
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT extra`
- **THEN** the exit code is 2 and no request was sent

### Requirement: Successful connect with the keychain
When the backend accepts the key and the token is stored in the system credential store, the CLI SHALL print on stdout the connection line with the user's display name and the machine's hostname, where the token is stored, the expiry in local time with a relative suffix, and a note that a new connect key was generated, then exit 0.

#### Scenario: Keychain on macOS
- **GIVEN** the clock reads 2026-11-14 10:21 in the Europe/Berlin time zone, the OS is darwin, the hostname is `morpheus-mbp`, and the backend answers with `user_display_name` `Morpheus` and `expires_at` `2027-02-14T09:21:00Z`
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT --ttl 3mo`
- **THEN** stdout is exactly
  ```
  ✓ Connected as Morpheus (morpheus-mbp)
    Token stored in: macOS Keychain
    Expires: 2027-02-14 10:21 (in 3 months)
    A new connect key was generated for your profile.
  ```
- **AND** the exit code is 0

#### Scenario: Keychain on Windows and Linux
- **WHEN** the same connect succeeds on windows or linux
- **THEN** the second line is `  Token stored in: Windows Credential Manager` on windows and `  Token stored in: system keyring (Secret Service)` on linux

### Requirement: Successful connect with a passphrase
With `--store passphrase` the CLI SHALL ask for the passphrase twice on stderr (`Choose a passphrase: ` and `Repeat passphrase: `, typed characters shown as `*`) before contacting the backend, store the token encrypted, and end its stdout with the OS-specific unlock hint. With `THE_AGENT_CLI_PASSPHRASE` set it SHALL not prompt.

#### Scenario: Passphrase on macOS or Linux
- **GIVEN** the clock and backend answer as in the keychain example, and the user types the same passphrase twice
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT --store passphrase`
- **THEN** stdout is exactly
  ```
  ✓ Connected as Morpheus (morpheus-mbp)
    Token stored in: ~/.agent-cli/credentials (encrypted)
    Expires: 2027-02-14 10:21 (in 3 months)
    A new connect key was generated for your profile.
    To unlock once per terminal session: eval "$(agent-cli unlock)"
  ```
- **AND** stderr shows the two prompts, each followed by one `*` per typed character, and the exit code is 0

#### Scenario: Passphrase on Windows
- **WHEN** the same connect succeeds on windows
- **THEN** the second line is `  Token stored in: ~\.agent-cli\credentials (encrypted)`
- **AND** the last line is `  To unlock once per terminal session: agent-cli unlock --shell pwsh | Invoke-Expression`

#### Scenario: Passphrases differ
- **WHEN** the user types two different passphrases
- **THEN** stderr is `error: passphrases do not match`, the exit code is 2 and no request was sent

#### Scenario: Empty passphrase
- **WHEN** the user submits an empty passphrase
- **THEN** stderr is `error: passphrase cannot be empty`, the exit code is 2 and no request was sent

#### Scenario: Passphrase from the environment
- **GIVEN** `THE_AGENT_CLI_PASSPHRASE=hunter2` and no terminal
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT --store passphrase`
- **THEN** no prompt is shown and the token is encrypted with `hunter2`

### Requirement: Successful connect with the unsafe store
With `--store unsafe` the CLI SHALL store the token unencrypted in the `credentials` file and say so on the `Token stored in:` line.

#### Scenario: Unsafe store
- **WHEN** the connect succeeds with `--store unsafe`
- **THEN** the second stdout line is `  Token stored in: ~/.agent-cli/credentials (NOT encrypted)`
- **AND** there is no unlock hint line

### Requirement: Storage is decided before the key is spent
The CLI SHALL resolve the storage mode, collect any passphrase, and make sure the home directory can be created and written, all before it contacts the backend, so that a local problem never wastes the single-use connect key.

#### Scenario: Prompt failure
- **GIVEN** `--store passphrase` and the two typed passphrases differ
- **WHEN** the command stops
- **THEN** the backend received no request

#### Scenario: Unusable keychain
- **GIVEN** the credential store is unusable and there is no way to ask for a passphrase
- **WHEN** the user connects without `--store`
- **THEN** the backend received no request

#### Scenario: Unwritable home directory
- **GIVEN** `THE_AGENT_CLI_HOME` points to a location that cannot be created or written
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT`
- **THEN** stderr starts with `error: cannot use the home directory ` and names the path
- **AND** the exit code is 1 and the backend received no request

### Requirement: Machine details sent to the backend
The connect request SHALL carry the machine's device id, a hostname reduced to letters, digits, dots and hyphens (at most 32 characters, runs of other characters replaced by a single hyphen; when the host name is empty, cannot be read or nothing is left of it, `device-` followed by the first 12 hexadecimal digits of the device id, so the name is as unique as the id), and the operating system name (`darwin`, `linux` or `windows`). `--ttl` SHALL be sent exactly as typed, and omitted when the flag is absent.

#### Scenario: Hostname cleanup
- **GIVEN** the machine hostname is `Morpheus's MacBook Pro`
- **WHEN** the user connects
- **THEN** the request carries `hostname` `Morpheus-s-MacBook-Pro`
- **AND** the success line shows `(Morpheus-s-MacBook-Pro)`

#### Scenario: Long hostname
- **GIVEN** the machine hostname is 40 characters long
- **WHEN** the user connects
- **THEN** the request carries a hostname of at most 32 characters that matches `^[A-Za-z0-9.-]{1,32}$`

#### Scenario: Missing hostname
- **GIVEN** the machine hostname cannot be read, is empty, or consists only of characters that are removed
- **AND** the device id is `13892199-ba28-8c7e-a6ab-399e6bfddc6f`
- **WHEN** the user connects
- **THEN** the request carries `hostname` `device-13892199ba28`
- **AND** the success line shows `(device-13892199ba28)`

#### Scenario: TTL passed through untouched
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT --ttl 3mo`
- **THEN** the request carries `ttl_as_text` `3mo`

#### Scenario: No TTL flag
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT`
- **THEN** the request has no `ttl_as_text` field

### Requirement: TTL errors come from the server
The CLI SHALL NOT parse the TTL. When the backend rejects it the CLI SHALL exit with code 2 and print a message built from the backend's error code and the typed value.

#### Scenario: Invalid TTL syntax
- **GIVEN** the backend answers `400` with error code `1043`
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT --ttl 1h30m`
- **THEN** stderr is `error: invalid TTL "1h30m": use a single unit, e.g. 90m`
- **AND** the exit code is 2 and nothing was stored

#### Scenario: TTL too long
- **GIVEN** the backend answers `400` with error code `1044`
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT --ttl 3y`
- **THEN** stderr is `error: TTL exceeds the server's maximum token lifetime`
- **AND** the exit code is 2 and nothing was stored

### Requirement: Invalid connect key
When the backend does not accept the key (`404` with error code `2015`, or any `401` or `403`) the CLI SHALL print `error: invalid connect key`, exit with code 1, store nothing and leave any existing connection untouched.

#### Scenario: Unknown key
- **GIVEN** the backend answers `404` with error code `2015`
- **WHEN** the user runs `agent-cli connect XXXX-XXXX-XXXX`
- **THEN** stderr is `error: invalid connect key` and the exit code is 1

#### Scenario: Existing connection survives
- **GIVEN** a machine that is already connected
- **WHEN** a connect attempt fails with an invalid key
- **THEN** the previous token and config are unchanged

### Requirement: Other connect failures
An unreachable backend, a `5xx`, a rate limit, an unexpected response or a rejected device description (`422` with error code `1045`) SHALL store nothing and leave any existing connection untouched. The exit codes SHALL follow the backend-client rules, except that `1045` SHALL print `error: the server rejected this machine's details; please report this` with exit code 1.

#### Scenario: Backend unreachable
- **GIVEN** nothing is listening at the API URL
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT`
- **THEN** stderr is `error: could not reach the server`, the exit code is 1 and nothing was stored

#### Scenario: Rate limited
- **GIVEN** the proxy answers `429`
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT`
- **THEN** the exit code is 6

#### Scenario: Device rejected
- **GIVEN** the backend answers `422` with error code `1045`
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT`
- **THEN** stderr is `error: the server rejected this machine's details; please report this` and the exit code is 1

### Requirement: Persistence after a successful connect
After a successful response the CLI SHALL store the token, then write the config file (device id, API URL, store mode, user id, display name, hostname, expiry). If the token cannot be stored the CLI SHALL print `error: connected, but the token could not be stored: <reason>; the connect key was used, get a new one from the agent`, exit with code 1 and leave the config unchanged.

#### Scenario: Store failure after success
- **GIVEN** the backend accepted the key but writing the `credentials` file fails
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT --store unsafe`
- **THEN** stderr starts with `error: connected, but the token could not be stored: ` and ends with `; the connect key was used, get a new one from the agent`
- **AND** the exit code is 1 and `config.toml` has no token metadata

#### Scenario: Config write failure rolls the secret back
- **GIVEN** the backend accepted the key and the token was stored, but writing `config.toml` fails
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT`
- **THEN** the exit code is 1 and stderr starts with `error: `
- **AND** the stored secret was removed again

### Requirement: Reconnecting
Connecting a machine that is already connected SHALL replace the previous connection. When the new connection belongs to a different user than the previous one, the stored default chat or platform SHALL be cleared; for the same user it SHALL be kept.

#### Scenario: Same user reconnects
- **GIVEN** a connected machine with default platform `telegram` for user `u1`
- **WHEN** the user connects again and the backend answers with `user_id` `u1`
- **THEN** the default platform is still `telegram`

#### Scenario: Different user connects
- **GIVEN** a connected machine with a default chat for user `u1`
- **WHEN** the user connects again and the backend answers with `user_id` `u2`
- **THEN** the stored default is cleared

### Requirement: Low credit warning after connecting
After a successful connect, when the backend reports a `credit_balance` below 100 credits, the CLI SHALL print `warning: low credit balance: <credits> credits left; top up in the agent's chat settings` to stderr, with credits truncated to two decimals. Amounts SHALL be shown only in credits, with no currency conversion. Stdout and the exit code SHALL not change. A missing, null or non-numeric balance SHALL be ignored.

#### Scenario: Low balance
- **GIVEN** the backend answers the connect with `credit_balance` 42.37
- **WHEN** the connect succeeds
- **THEN** stderr is `warning: low credit balance: 42.37 credits left; top up in the agent's chat settings`
- **AND** stdout is the usual success output and the exit code is 0

#### Scenario: Balance at the threshold
- **GIVEN** the backend answers with `credit_balance` 100
- **WHEN** the connect succeeds
- **THEN** stderr is empty

#### Scenario: Just below the threshold
- **GIVEN** the backend answers with `credit_balance` 99.999
- **WHEN** the connect succeeds
- **THEN** stderr is `warning: low credit balance: 99.99 credits left; top up in the agent's chat settings`

#### Scenario: Negative balance
- **GIVEN** the backend answers with `credit_balance` -3.5
- **WHEN** the connect succeeds
- **THEN** stderr is `warning: low credit balance: -3.50 credits left; top up in the agent's chat settings`

#### Scenario: Balance not reported
- **GIVEN** the backend answers without `credit_balance`, or with `null` or a string
- **WHEN** the connect succeeds
- **THEN** stderr is empty and the exit code is 0
