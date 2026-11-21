# Spec Delta

## Purpose

Defines where the CLI keeps its non-secret local state (home directory, config file, device identity, defaults) and how environment variables and build-time defaults combine into the effective settings.

## ADDED Requirements

### Requirement: Home directory
The CLI SHALL keep local state in the directory named by `THE_AGENT_CLI_HOME`, or in `.<name>` inside the user's home directory when `THE_AGENT_CLI_HOME` is unset, where `<name>` is the command name of the build (`.abot` for `abot`, `.abot-dev` for `abot-dev`, `.agent-cli` for `agent-cli`), so variants installed side by side never share state. The directory SHALL be created with owner-only permissions (mode 0700 where the OS supports modes) the first time state is written, and never by a read-only command.

#### Scenario: Default location
- **GIVEN** `THE_AGENT_CLI_HOME` is unset and the user's home directory is `/home/morpheus`
- **WHEN** `agent-cli connect` succeeds
- **THEN** `/home/morpheus/.agent-cli` exists and has mode 0700 on Unix

#### Scenario: Override location
- **GIVEN** `THE_AGENT_CLI_HOME=/tmp/agent-test`
- **WHEN** `agent-cli connect` succeeds
- **THEN** all state files are under `/tmp/agent-test` and nothing is written to `~/.agent-cli`

#### Scenario: Relative override
- **GIVEN** `THE_AGENT_CLI_HOME=state` and the working directory is `/work`
- **WHEN** `agent-cli connect` succeeds
- **THEN** state files are under `/work/state`

#### Scenario: Read-only command on a fresh machine
- **GIVEN** the home directory does not exist
- **WHEN** the user runs `agent-cli status`
- **THEN** the exit code is 3 and the home directory still does not exist

### Requirement: Config file contents
`config.toml` in the home directory SHALL hold the device id, the API URL, the token storage mode, token metadata (user id, display name, hostname, expiry time) and the default chat or default platform. It SHALL NOT contain a token, passphrase or connect key, and SHALL be readable only by its owner (mode 0600 where supported).

#### Scenario: File after connect
- **WHEN** `agent-cli connect K7Q2-M9XA-P3RT` succeeds
- **THEN** `config.toml` contains the device id, the API URL used, the store mode, the user id, display name, hostname and expiry
- **AND** it does not contain the token text
- **AND** it has mode 0600 on Unix

#### Scenario: Unknown keys are ignored
- **GIVEN** `config.toml` contains a key that this version does not know
- **WHEN** the user runs `agent-cli status`
- **THEN** the command works as if the key were absent

### Requirement: Device identity
The CLI SHALL derive a UUID as the device id the first time the machine connects, store it in the config file, and send the same id on every later connect from the same home directory. The id SHALL be as unique to the machine as possible: it is a hash (never the raw values) of the identifier that the operating system keeps for the installation when it can be read (the hardware UUID on macOS, `/etc/machine-id` or `/var/lib/dbus/machine-id` on Linux, `MachineGuid` on Windows), otherwise of the hardware (MAC) addresses of the machine's network interfaces together with its host name, user name, operating system and architecture, and only when the machine offers neither, a random version-4 UUID. The absolute path of the home directory is part of the hash, so different home directories SHALL have different ids even on one machine. The machine identifier and the addresses SHALL NOT be stored, printed or sent as such.

#### Scenario: First connect
- **GIVEN** no config file exists
- **WHEN** `agent-cli connect` succeeds
- **THEN** the config file holds a valid UUID as `device_id`
- **AND** the connect request carried that same value

#### Scenario: Derived from the machine
- **GIVEN** the operating system reports a machine identifier
- **WHEN** the id is derived twice for the same home directory
- **THEN** both ids are equal, the identifier text does not appear in them, and a different home directory or a different machine identifier gives a different id

#### Scenario: Machine identifier unavailable
- **GIVEN** no machine identifier can be read but a hardware address exists
- **WHEN** the id is derived
- **THEN** it depends on the hardware addresses, host name, user name, operating system and architecture, and not on the order of the interfaces

#### Scenario: Nothing to derive from
- **GIVEN** neither a machine identifier nor a hardware address is available
- **WHEN** the id is derived
- **THEN** it is a random version-4 UUID

#### Scenario: Reconnect keeps the id
- **GIVEN** a config file with device id `6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b`
- **WHEN** the user connects again
- **THEN** the request carries `6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b`

#### Scenario: Deleted config
- **GIVEN** the user deleted `config.toml`
- **WHEN** the user connects again
- **THEN** the id is derived again, and equals the earlier one when it came from the machine and the home directory is the same

### Requirement: Effective API URL
The API URL SHALL be taken from `THE_AGENT_CLI_API_URL` when set, otherwise from the config file, otherwise from the build-time default. A trailing slash SHALL be ignored. `connect` SHALL store the URL it used in the config file; the environment variable SHALL never modify the stored value by itself.

#### Scenario: Environment wins
- **GIVEN** the config file has `api_url = "https://a.test"` and `THE_AGENT_CLI_API_URL=https://b.test`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** the request goes to `https://b.test/...`
- **AND** `config.toml` still has `https://a.test`

#### Scenario: Config beats build default
- **GIVEN** the config file has `api_url = "https://a.test"` and `THE_AGENT_CLI_API_URL` is unset
- **WHEN** the user runs `agent-cli chats`
- **THEN** the request goes to `https://a.test/cli/chats`

#### Scenario: Trailing slash
- **GIVEN** `THE_AGENT_CLI_API_URL=https://a.test/`
- **WHEN** the CLI calls the backend
- **THEN** the path is `/cli/chats` and not `//cli/chats`

#### Scenario: Connect stores the URL it used
- **GIVEN** `THE_AGENT_CLI_API_URL=https://staging.agent.cloud.appifyhub.com`
- **WHEN** `agent-cli connect` succeeds
- **THEN** `config.toml` has `api_url = "https://staging.agent.cloud.appifyhub.com"`

### Requirement: API URL safety
The API URL SHALL use `https`, except that `http` SHALL be accepted for loopback hosts (`localhost`, `127.0.0.1`, `::1`). URLs with embedded credentials, a query string or a fragment SHALL be rejected with exit code 2 before any request is made, and the error SHALL NOT echo embedded credentials.

#### Scenario: Cleartext to a remote host
- **GIVEN** `THE_AGENT_CLI_API_URL=http://agent.example.test`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** stderr is `error: invalid API URL "http://agent.example.test": use https (http is allowed only for localhost)`
- **AND** the exit code is 2 and no request was sent

#### Scenario: Loopback over http
- **GIVEN** `THE_AGENT_CLI_API_URL=http://127.0.0.1:8080`
- **WHEN** the user runs `agent-cli chats`
- **THEN** the request is sent to `http://127.0.0.1:8080/cli/chats`

#### Scenario: Credentials in the URL
- **GIVEN** `THE_AGENT_CLI_API_URL=https://user:secret@agent.example.test`
- **WHEN** the user runs `agent-cli chats`
- **THEN** the exit code is 2 and the error text does not contain `secret`

### Requirement: Default target resolution
The effective default notification target SHALL be, in order: `THE_AGENT_CLI_DEFAULT_CHAT`, `THE_AGENT_CLI_DEFAULT_PLATFORM`, the default stored in the config file (either a chat or a platform, never both), otherwise none. Empty variables count as unset. `THE_AGENT_CLI_DEFAULT_PLATFORM` SHALL accept `telegram` or `whatsapp` case-insensitively and any other value SHALL stop the command with exit code 2.

#### Scenario: Environment chat beats everything
- **GIVEN** `THE_AGENT_CLI_DEFAULT_CHAT=c1`, `THE_AGENT_CLI_DEFAULT_PLATFORM=whatsapp` and a config default chat `c2`
- **WHEN** the effective default is resolved
- **THEN** it is chat `c1`

#### Scenario: Environment platform beats config
- **GIVEN** `THE_AGENT_CLI_DEFAULT_PLATFORM=whatsapp` and a config default chat `c2`
- **WHEN** the effective default is resolved
- **THEN** it is platform `whatsapp`

#### Scenario: Config platform
- **GIVEN** no environment defaults and a config default platform `telegram`
- **WHEN** the effective default is resolved
- **THEN** it is platform `telegram`

#### Scenario: Case-insensitive platform
- **GIVEN** `THE_AGENT_CLI_DEFAULT_PLATFORM=WhatsApp`
- **WHEN** the effective default is resolved
- **THEN** it is platform `whatsapp`

#### Scenario: Invalid platform in the environment
- **GIVEN** `THE_AGENT_CLI_DEFAULT_PLATFORM=slack`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** stderr is `error: invalid THE_AGENT_CLI_DEFAULT_PLATFORM "slack": use telegram or whatsapp`
- **AND** the exit code is 2

#### Scenario: No default anywhere
- **GIVEN** no environment defaults and no config default
- **WHEN** the effective default is resolved
- **THEN** there is none and the latest chat is used by the server

### Requirement: Safe config updates
Config changes SHALL be written atomically so that a crash or concurrent writer cannot leave a partial file. An unreadable or malformed config file SHALL stop the command with exit code 1 and an error that names the file, and SHALL NOT be silently replaced.

#### Scenario: Malformed config
- **GIVEN** `config.toml` contains invalid TOML
- **WHEN** the user runs `agent-cli status`
- **THEN** stderr is a single line starting with `error: could not read ` that names the config file path and suggests fixing or deleting it
- **AND** the exit code is 1 and the file is unchanged

#### Scenario: Failed write keeps the old file
- **GIVEN** a valid `config.toml` and a write that fails midway
- **WHEN** the CLI tries to update the config
- **THEN** the previous file content is still intact and readable

### Requirement: Environment report
`agent-cli env` SHALL print to stdout, with exit code 0, every environment variable the CLI reads, in a fixed order (`THE_AGENT_CLI_API_URL`, `THE_AGENT_CLI_HOME`, `THE_AGENT_CLI_TOKEN`, `THE_AGENT_CLI_PASSPHRASE`, `THE_AGENT_CLI_DEFAULT_CHAT`, `THE_AGENT_CLI_DEFAULT_PLATFORM`, `NO_COLOR`), each with what it controls, whether it is set, and the value in effect. Secret variables SHALL never show their value. The command SHALL accept no arguments, work offline, and not create or modify any file.

#### Scenario: Nothing set
- **GIVEN** no `THE_AGENT_CLI_*` variable and no `NO_COLOR` is set, on a default build with the user home `/home/morpheus`
- **WHEN** the user runs `agent-cli env`
- **THEN** each variable is reported as not set together with its default, for example `THE_AGENT_CLI_API_URL` as not set and using `http://localhost`, and `THE_AGENT_CLI_HOME` as not set and using `~/.agent-cli`
- **AND** the exit code is 0

#### Scenario: Values in effect
- **GIVEN** `THE_AGENT_CLI_API_URL=https://agent.example.com` and `THE_AGENT_CLI_DEFAULT_PLATFORM=whatsapp`
- **WHEN** the user runs `agent-cli env`
- **THEN** both are reported as set with their values

#### Scenario: Secrets stay hidden
- **GIVEN** `THE_AGENT_CLI_TOKEN` and `THE_AGENT_CLI_PASSPHRASE` are set
- **WHEN** the user runs `agent-cli env`
- **THEN** both are reported as set without their values, and neither value appears anywhere in the output

#### Scenario: Untrusted values are neutralized
- **GIVEN** `THE_AGENT_CLI_DEFAULT_CHAT` contains an escape sequence
- **WHEN** the user runs `agent-cli env`
- **THEN** the printed value contains no escape characters

