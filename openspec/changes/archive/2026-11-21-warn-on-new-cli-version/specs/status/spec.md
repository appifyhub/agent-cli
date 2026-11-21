# Spec Delta

## Purpose

Report local CLI readiness, backend status, CLI version availability and last online check in one command. This replaces standalone `server` behavior and the offline-by-default `status` contract.

## ADDED Requirements

### Requirement: Local readiness remains authoritative

`agent-cli status` SHALL evaluate readiness once and print the local subsection first. Regardless of the remote result, the normal exit SHALL be 0 when ready and 3 when disconnected, locked or expired.

#### Scenario: Local assessment in either mode

- **WHEN** plain `status` or `status --offline` evaluates readiness
- **THEN** it parses any environment token locally without prompting, reading or unlocking a stored token, accessing the keychain, verifying credentials remotely or changing credentials or configuration.
- **AND** the local report shows the connection, default target and guidance applicable to that state, the selected backend without its HTTP(S) scheme, and the installed CLI version. When connected, token storage and expiry are separate rows.

#### Scenario: Report is independent of remote availability

- **WHEN** a locally connected user runs status with a usable stored or environment token
- **THEN** the local subsection is printed once on stdout and exit is 0, even if the backend is unavailable.

#### Scenario: Remediation remains visible

- **WHEN** a disconnected, locked or expired user runs status and an advertised CLI version differs
- **THEN** the corresponding connect, unlock or reconnect guidance remains on stdout and exit is 3; the version comparison is separate from readiness.

#### Scenario: Local errors win

- **WHEN** status encounters malformed config, an invalid environment token or an invalid effective default that previously failed local status
- **THEN** the original diagnostic and exit code are returned without a lookup, regardless of what version the backend could return.

### Requirement: One public lookup and an offline opt-out

After writing a normal local subsection, default `status` SHALL make one public `GET /cli/status` at the effective backend, with at most three seconds including body reads, bounded by the caller's context. It SHALL NOT query `/health` or fall back to other endpoints.

#### Scenario: Effective target is independent of credentials

- **WHEN** a connected, disconnected, locked or expired user runs plain status with a valid effective URL
- **THEN** the URL is selected from the environment, then config, then the build default. That backend receives one GET with no Authorization header, body, query parameters, JWT, API key, source URL or account data, even when an environment token is set.
- **AND** status does not follow redirects, retry, contact GitHub or cache remote status or versions locally.

#### Scenario: Invalid override is not a fallback address

- **WHEN** the highest-precedence URL is invalid, or all URL sources are empty
- **THEN** status makes zero requests, does not try a lower-priority address, shows the backend as unavailable online and retains the local exit code.

#### Scenario: Strictly offline operation

- **WHEN** a user runs `status --offline` with a configured backend, even with `--verbose`
- **THEN** there are zero requests, zero request diagnostics, no remote status or version comparison, no state writes and no change to the local exit code. Status displays the selected backend's last check time, or `never checked`.

### Requirement: Report backend and CLI versions with readiness-independent results

Default `status` SHALL use the required `status` and `service_version` strings and optional, nullable `cli_version` string from a usable HTTP 200 response. Keep rows aligned and readiness independent. A ready named connection gets a checkmark. With color enabled, that heading, healthy backend status and `latest` are green; unhealthy or unavailable backend status and `update available` are yellow. Other rows remain uncolored.

#### Scenario: Usable combined response

- **WHEN** `/cli/status` returns `{"status":"degraded","service_version":"5.41.0","cli_version":"1.2.3"}` to a locally ready CLI
- **THEN** stdout reports `degraded`, backend version `5.41.0`, the installed CLI version, advertised CLI version `1.2.3`, and the response latency. Exit remains 0. External values are terminal-sanitized and unrelated fields are ignored.

#### Scenario: Compare available versions by SemVer precedence

- **WHEN** the installed and advertised CLI versions have equal SemVer precedence
- **THEN** stdout shows both and says `latest`.
- **WHEN** the advertised version has higher SemVer precedence
- **THEN** stdout shows both and says `update available`, linking to `https://github.com/appifyhub/agent-cli/releases`, without changing the local exit code.
- **WHEN** the installed version is ahead or either version cannot be compared
- **THEN** stdout reports the available versions without an update claim.

#### Scenario: Unavailable result or version

- **WHEN** a usable response omits `cli_version` or returns it as `null`
- **THEN** stdout still reports the backend status and version, but the latest CLI is unavailable and no comparison is claimed.
- **WHEN** the lookup fails at the HTTP, transport, body or deadline level, or the response is unusable
- **THEN** status shows the backend and remote versions as unavailable, does not retry or fall back, and retains the local exit code. Ordinary stderr is quiet.

#### Scenario: Caller cancels or output fails

- **WHEN** the caller cancels or an output or required state write fails
- **THEN** the command propagates the cancellation or write error instead of reporting success; any started request has finished before return.

### Requirement: Remember the last completed check per backend

Status SHALL persist the local completion time of each completed online check, including a failed check, under the selected backend. The record SHALL contain no credentials or remote response data.

#### Scenario: Remember a failed or successful attempt without leaking across backends

- **WHEN** an online check completes, including an HTTP, transport, body or timeout failure
- **THEN** its new completion time is shown and saved atomically. Subsequent `status --offline` runs for the same backend show that time without sending a request or changing it.
- **WHEN** the selected backend changes
- **THEN** its own record is used, or `never checked` if it has none. Switching back preserves the earlier backend's record. An invalid or missing URL or caller-cancelled check does not update a record.

### Requirement: Status replaces the standalone server command

`status` SHALL be the only command for the merged report; `server` SHALL NOT be registered, advertised or accepted as an alias. Help and embedded agent guidance SHALL distinguish plain `status` for backend diagnostics from `status --offline` for local preflight, and SHALL NOT present the readiness exit as a backend health probe.

#### Scenario: Discover and use the replacement

- **WHEN** a caller lists commands, requests shell completion or invokes `server`
- **THEN** help and completion omit `server`, and invoking it yields the normal unknown-command outcome without either request. Both `status --help` and `help status` describe the merged report and offline opt-out.
