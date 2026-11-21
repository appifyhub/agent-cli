# Spec Delta

## Purpose

Defines `agent-cli server`, which asks the backend whether it is up and prints what the backend reports about itself (its health status and version), so a user can tell a server problem from a local one, and see which server and release the CLI is talking to. It needs no connection to a profile.

## ADDED Requirements

### Requirement: Server report
`agent-cli server` SHALL call `GET /health` on the effective API URL without authentication, and print to stdout a report with one `Label  value` line per item, labels in a left-aligned column, in this order: `Server` (the API URL in use without its scheme, so `https://agent.example.com/` is shown as `agent.example.com`), `Status` (the reported status), `Version` (the reported version, omitted when the backend sends none), then any other top-level scalar field of the answer (string, number or boolean) as its own line in alphabetical order with the field name as label, and finally `Response` (the time the call took, in milliseconds, for example `123 ms`). Nested objects and arrays SHALL be ignored. Every value that came from the server SHALL pass through the terminal sanitizer. The command SHALL accept no arguments, need no token, passphrase or connection, create no file, and exit 0 when the reported status is `ok`.

#### Scenario: Healthy server
- **GIVEN** the API URL is `https://agent.example.com` and the backend answers `{"status": "ok", "version": "5.41.0"}` after 123 ms
- **WHEN** the user runs `agent-cli server`
- **THEN** stdout is
  ```
  Server    agent.example.com
  Status    ok
  Version   5.41.0
  Response  123 ms
  ```
- **AND** stderr is empty and the exit code is 0

#### Scenario: Extra fields are shown
- **GIVEN** the backend answers `{"status": "ok", "version": "5.41.0", "region": "eu", "uptime_seconds": 86400, "debug": false, "checks": {"db": "ok"}}`
- **WHEN** the user runs `agent-cli server`
- **THEN** the report lists `debug`, `region` and `uptime_seconds` between `Version` and `Response` in alphabetical order, and does not mention `checks`

#### Scenario: Version missing
- **GIVEN** the backend answers `{"status": "ok"}`
- **WHEN** the user runs `agent-cli server`
- **THEN** the report has no `Version` line and the exit code is 0

#### Scenario: No profile needed
- **GIVEN** the machine is not connected and no home directory exists
- **WHEN** the user runs `agent-cli server`
- **THEN** the report is printed, the home directory still does not exist, and the exit code is 0

#### Scenario: Server text is neutralized
- **GIVEN** the backend answers a `version` that contains an escape sequence
- **WHEN** the user runs `agent-cli server`
- **THEN** the printed value contains no escape characters

### Requirement: Unhealthy and failing servers
When the backend answers with a status other than `ok`, the command SHALL still print the report and exit 1. When the call cannot be made or answered, it SHALL print nothing on stdout, one `error: ` line on stderr and exit with the code of the shared classification: 1 when the server cannot be reached, times out, answers with an HTTP error status or with something that is not a JSON object with a string `status`, and 6 for HTTP 429. The error line SHALL name the server address (with its scheme, so the problem is unambiguous) and SHALL NOT contain credentials. The API URL SHALL be validated before the call, as for every command that sends a request (exit 2).

#### Scenario: Status is not ok
- **GIVEN** the backend answers `{"status": "degraded", "version": "5.41.0"}`
- **WHEN** the user runs `agent-cli server`
- **THEN** the report shows `Status    degraded` and the exit code is 1

#### Scenario: Server unreachable
- **GIVEN** nothing listens at the API URL
- **WHEN** the user runs `agent-cli server`
- **THEN** stdout is empty, stderr is a single `error: ` line that names the API URL and the reason, and the exit code is 1

#### Scenario: HTTP error
- **GIVEN** the backend answers HTTP 503 with a non-JSON body
- **WHEN** the user runs `agent-cli server`
- **THEN** stderr is a single `error: ` line that names the API URL and HTTP 503, and the exit code is 1

#### Scenario: Not the expected answer
- **GIVEN** the address answers HTTP 200 with a web page
- **WHEN** the user runs `agent-cli server`
- **THEN** stderr says that the address did not answer like the agent's server, and the exit code is 1

#### Scenario: Rate limited
- **GIVEN** the backend answers HTTP 429
- **WHEN** the user runs `agent-cli server`
- **THEN** the exit code is 6

#### Scenario: Invalid API URL
- **GIVEN** `THE_AGENT_CLI_API_URL=http://agent.example.test`
- **WHEN** the user runs `agent-cli server`
- **THEN** the exit code is 2 and no request was sent

#### Scenario: Extra argument
- **WHEN** the user runs `agent-cli server now`
- **THEN** the exit code is 2
