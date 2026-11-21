# Spec Delta

## Purpose

Defines how the CLI talks to the agent's backend: the five endpoints it uses, request and response conventions, and how every error outcome is classified into a message, an exit code and a local-token action.

## ADDED Requirements

### Requirement: Endpoints and payloads
The CLI SHALL use exactly these backend endpoints: `GET /health` (no authentication, used only by `agent-cli server`), `POST /cli/connect` (no authentication), and with the token `DELETE /cli/user/{user_id_hex}/devices`, `GET /cli/chats`, `POST /cli/notify/user/{user_id_hex}` and `POST /cli/notify/chat/{chat_id}`. Requests and responses SHALL be JSON.

#### Scenario: Connect request and response
- **WHEN** the CLI connects with key `K7Q2-M9XA-P3RT`
- **THEN** it sends `POST /cli/connect` with a JSON body holding `connect_key`, `device_id`, `hostname` and `os`, plus `ttl_as_text` only when `--ttl` was given
- **AND** it expects `token`, `expires_at`, `user_id` and `user_display_name` in the response, and reads the optional `credit_balance`

#### Scenario: Revoke request
- **WHEN** the CLI revokes all access for user `u1`
- **THEN** it sends `DELETE /cli/user/u1/devices` without a body and expects `{"status": "OK"}`

#### Scenario: Chat list request
- **WHEN** the CLI lists chats
- **THEN** it sends `GET /cli/chats` and expects a JSON array of objects with `chat_id`, `platform`, `title` (possibly null) and `is_private`

#### Scenario: Notify to the user
- **WHEN** the CLI notifies user `u1` on platform `whatsapp`
- **THEN** it sends `POST /cli/notify/user/u1` with `{"message": "...", "platform": "whatsapp"}`
- **AND** with no platform it sends `"platform": null`
- **AND** it expects a chat object (`chat_id`, `platform`, `title`, `is_private`) and reads the optional `credit_balance` in the response

#### Scenario: Notify to a chat
- **WHEN** the CLI notifies chat `c1`
- **THEN** it sends `POST /cli/notify/chat/c1` with `{"message": "..."}`

### Requirement: Bearer token only where needed
Authenticated calls SHALL send the token in an `Authorization: Bearer <token>` header and nowhere else. The connect call SHALL NOT send any `Authorization` header, and tokens SHALL NOT appear in URLs.

#### Scenario: Authenticated call
- **WHEN** the CLI calls `GET /cli/chats` with token `T`
- **THEN** the request has the header `Authorization: Bearer T` and the URL has no query string

#### Scenario: Connect call
- **WHEN** the CLI calls `POST /cli/connect`
- **THEN** the request has no `Authorization` header

### Requirement: Bounded and conservative requests
Each request SHALL time out after 30 seconds, SHALL NOT follow redirects, SHALL honor the system proxy settings, SHALL NOT be retried automatically, and SHALL send `User-Agent: <name>/<version> (<os>/<arch>)`, where `<name>` is the command name of the build. A redirect response SHALL be treated as an unexpected response.

#### Scenario: Redirect is not followed
- **GIVEN** the backend answers `302` with a `Location` header
- **WHEN** the CLI makes a call
- **THEN** exactly one request is made, the command fails with `error: unexpected response from the server` and exit code 1

#### Scenario: No automatic retry
- **GIVEN** the backend answers `502` once
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** exactly one request reached the backend

#### Scenario: Slow server
- **GIVEN** the backend does not answer within 30 seconds
- **WHEN** the CLI makes a call
- **THEN** it stops waiting, prints `error: could not reach the server` and exits with code 1

### Requirement: Error bodies of any shape
The CLI SHALL understand error responses of the form `{"error_code": <int>, "message": "...", "emoji": "..."}`, framework-style `{"detail": ...}` bodies, and non-JSON or empty bodies (for example from a proxy). A missing or unreadable body SHALL NOT change the classification that the HTTP status alone allows.

#### Scenario: Structured error
- **GIVEN** the backend answers `422` with `{"error_code": 1034, "message": "[E1034] ⚠️ Not enough credits", "emoji": "⚠️"}`
- **WHEN** the CLI classifies the response
- **THEN** it is an insufficient-credits outcome

#### Scenario: Plain-text rate limit from the proxy
- **GIVEN** the proxy answers `429` with the plain text body `Too Many Requests`
- **WHEN** the CLI classifies the response
- **THEN** it is a rate-limited outcome

#### Scenario: HTML gateway error
- **GIVEN** a gateway answers `502` with an HTML page
- **WHEN** the CLI classifies the response
- **THEN** it is a server-fault outcome

### Requirement: Rate limiting is exit code 6
An HTTP 429 response, with any body, SHALL produce `error: rate limited; wait a moment and try again`, exit code 6, and SHALL leave the stored token untouched.

#### Scenario: Proxy rate limit on connect
- **GIVEN** the proxy answers `429` to a connect call
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT`
- **THEN** stderr is `error: rate limited; wait a moment and try again` and the exit code is 6

#### Scenario: Provider rate limit on notify
- **GIVEN** the backend answers `429` with error code `6004`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** the same message is printed, the exit code is 6 and the token is kept

### Requirement: Server faults are exit code 1
HTTP 5xx responses and transport failures SHALL produce exit code 1. A 5xx response SHALL print `error: the server had a problem; try again later`, and a transport failure (DNS, connection, TLS, timeout) SHALL print `error: could not reach the server`.

#### Scenario: Server error
- **GIVEN** the backend answers `500`
- **WHEN** the user runs `agent-cli chats`
- **THEN** stderr is `error: the server had a problem; try again later` and the exit code is 1

#### Scenario: Server unreachable
- **GIVEN** nothing is listening at the API URL
- **WHEN** the user runs `agent-cli chats`
- **THEN** stderr is `error: could not reach the server` and the exit code is 1

### Requirement: Token rejection is exit code 3
On an authenticated call, HTTP 401, a 404 with error code `2001`, and any 403 whose error code is not a chat-target or account-gate code (`3001`, `3004`, `3007`, `3008`, `3009`, `3010`) SHALL mean the token is no longer valid for this user and SHALL produce exit code 3.

#### Scenario: Revoked token
- **GIVEN** the backend answers `401` with error code `4005`
- **WHEN** the user runs `agent-cli chats`
- **THEN** the outcome is token rejection and the exit code is 3

#### Scenario: Invalid token
- **GIVEN** the backend answers `401` with error code `4004`
- **WHEN** the user runs `agent-cli chats`
- **THEN** the outcome is token rejection and the exit code is 3

#### Scenario: Merged-away account
- **GIVEN** the backend answers `404` with error code `2001`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** the outcome is token rejection and the exit code is 3

#### Scenario: Token for another user
- **GIVEN** the backend answers `403` with error code `3002`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** the outcome is token rejection and the exit code is 3

#### Scenario: Unknown 403
- **GIVEN** the backend answers `403` with a body that has no error code
- **WHEN** the user runs `agent-cli chats`
- **THEN** the outcome is token rejection and the exit code is 3

### Requirement: Rejected tokens are removed locally
When a stored token is rejected, the CLI SHALL remove the local token and print ``This CLI is not connected. Run `agent-cli connect <KEY>`.``, exit 3. When the rejected token came from `THE_AGENT_CLI_TOKEN`, the CLI SHALL NOT touch stored credentials and SHALL print `error: the token in THE_AGENT_CLI_TOKEN was rejected by the server (expired or revoked)`, exit 3.

#### Scenario: Stored token rejected
- **GIVEN** a keychain machine and the backend answers `401`
- **WHEN** the user runs `agent-cli chats`
- **THEN** stderr is ``This CLI is not connected. Run `agent-cli connect <KEY>`.``
- **AND** the exit code is 3, the credential store entry is gone and `config.toml` has no token metadata

#### Scenario: Environment token rejected
- **GIVEN** `THE_AGENT_CLI_TOKEN` is set, a different stored token exists and the backend answers `401`
- **WHEN** the user runs `agent-cli chats`
- **THEN** stderr is `error: the token in THE_AGENT_CLI_TOKEN was rejected by the server (expired or revoked)` and the exit code is 3
- **AND** the stored token is still present

### Requirement: Chat unreachable is exit code 4
A 403 with error code `3001`, `3004`, `3009` or `3010`, and a 404 with error code `2002`, SHALL produce exit code 4 and SHALL keep the token. Each code SHALL have its own message starting with `error: chat unreachable: `.

#### Scenario: No private chat
- **GIVEN** the backend answers `403` with error code `3004`
- **WHEN** the user runs `agent-cli notify --platform whatsapp "hi"`
- **THEN** stderr is `error: chat unreachable: no private chat found; start a conversation with the agent on Telegram or WhatsApp first`
- **AND** the exit code is 4

#### Scenario: Delivery refused
- **GIVEN** the backend answers `403` with error code `3010`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** stderr is `error: chat unreachable: the platform refused the message; try another chat or platform` and the exit code is 4

#### Scenario: Not an admin
- **GIVEN** the backend answers `403` with error code `3001` or `3009`
- **WHEN** the user runs `agent-cli notify --chat c1 "hi"`
- **THEN** stderr is `error: chat unreachable: you are not an admin of that chat`, the exit code is 4 and the token is kept

#### Scenario: Unknown chat
- **GIVEN** the backend answers `404` with error code `2002`
- **WHEN** the user runs `agent-cli notify --chat c1 "hi"`
- **THEN** stderr is ``error: chat unreachable: no such chat; run `agent-cli chats` to list yours`` and the exit code is 4

### Requirement: Insufficient credits is exit code 5
A 422 with error code `1034` SHALL produce `error: insufficient credits; top up in the agent's chat settings` and exit code 5, and SHALL keep the token.

#### Scenario: Out of credits
- **GIVEN** the backend answers `422` with error code `1034`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** stderr is `error: insufficient credits; top up in the agent's chat settings`
- **AND** the exit code is 5

### Requirement: Account gating is exit code 1
A 403 with error code `3008` or `3007` (the backend gates the notification routes on policies and waitlist state) SHALL keep the stored token, produce exit code 1 and print a message that tells the user what to do: for `3008` to accept the policies in the agent's settings, for `3007` that the account is still on the waitlist.

#### Scenario: Policies not accepted
- **GIVEN** the backend answers `403` with error code `3008`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** stderr is `error: accept the agent's policies in chat settings, then try again`
- **AND** the exit code is 1 and the stored token is kept

#### Scenario: Account still on the waitlist
- **GIVEN** the backend answers `403` with error code `3007`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** stderr is `error: your account is still on the waitlist; try again once the agent has activated it`
- **AND** the exit code is 1 and the stored token is kept

### Requirement: Validation errors are exit code 2
An HTTP 400 or 422 not covered by a more specific rule SHALL produce exit code 2. Known codes SHALL have catalog messages; other validation errors SHALL print the cleaned server message.

#### Scenario: Known validation code
- **GIVEN** the backend answers `422` with error code `1019`
- **WHEN** the user runs `agent-cli notify --platform telegram "hi"`
- **THEN** stderr is `error: invalid platform; use telegram or whatsapp` and the exit code is 2

#### Scenario: Malformed chat id
- **GIVEN** the backend answers `422` with error code `1030`
- **WHEN** the user runs `agent-cli notify --chat zzz "hi"`
- **THEN** stderr is ``error: malformed chat id; run `agent-cli chats` to list yours`` and the exit code is 2

#### Scenario: Unknown validation code
- **GIVEN** the backend answers `422` with error code `1999` and message `[E1999] ✏️ Something is off`
- **WHEN** the CLI prints the error
- **THEN** stderr is `error: Something is off` and the exit code is 2

#### Scenario: Framework validation body
- **GIVEN** the backend answers `422` with `{"detail": [{"loc": ["body", "message"], "msg": "field required", "type": "missing"}]}`
- **WHEN** the CLI prints the error
- **THEN** stderr is `error: the server rejected the request` and the exit code is 2

### Requirement: Unclassified failures are exit code 1
Any HTTP error response that no other rule covers SHALL produce `error: unexpected response from the server`, exit code 1, and SHALL keep the stored token.

#### Scenario: Unknown route
- **GIVEN** the backend answers `404` with `{"detail": "Not Found"}` to `GET /cli/chats`
- **WHEN** the user runs `agent-cli chats`
- **THEN** stderr is `error: unexpected response from the server`, the exit code is 1 and the token is kept

#### Scenario: Unexpected status with a structured body
- **GIVEN** the backend answers `409` with error code `1999`
- **WHEN** the user runs `agent-cli notify "hi"`
- **THEN** stderr is `error: unexpected response from the server` and the exit code is 1

### Requirement: Server messages are cleaned
When the CLI prints server-provided error text, it SHALL remove a leading `[E<digits>]` marker and any emoji or symbols that follow it, remove a trailing ` # Caused by:` part, collapse whitespace, and apply the untrusted-text rules.

#### Scenario: Marker and cause removed
- **GIVEN** the message `[E1999] ✏️ Something is off # Caused by: ValueError('x')`
- **WHEN** the CLI prints it
- **THEN** the printed text is `Something is off`

### Requirement: Response validation
A success response of the connect call or the chat list that is missing a required field or holds an unusable value (for example an `expires_at` that is not a timestamp) SHALL produce `error: unexpected response from the server` and exit code 1. Unknown extra fields SHALL be ignored, and the optional `credit_balance` SHALL be ignored when it is missing, null or not a number.

#### Scenario: Missing token
- **GIVEN** the connect response is `{"expires_at": "2027-02-14T09:21:00Z", "user_id": "u1", "user_display_name": "Morpheus"}`
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT`
- **THEN** stderr is `error: unexpected response from the server`, the exit code is 1 and nothing is stored

#### Scenario: Extra fields
- **GIVEN** the chat list response has an additional field on each chat
- **WHEN** the user runs `agent-cli chats`
- **THEN** the list prints normally

#### Scenario: Unusable balance
- **GIVEN** the connect response holds `"credit_balance": "lots"` next to valid fields
- **WHEN** the user runs `agent-cli connect K7Q2-M9XA-P3RT`
- **THEN** the connect succeeds and no balance warning is printed

### Requirement: Verbose backend diagnostics
With `--verbose` the CLI SHALL print `Calling backend…` to stderr before each request and, when the backend answers with an error, one extra stderr line `  (HTTP <status>, code <error_code>)` omitting the code if there is none. It SHALL NOT print headers or bodies.

#### Scenario: Verbose error line
- **GIVEN** the backend answers `422` with error code `1034`
- **WHEN** the user runs `agent-cli notify "hi" -v`
- **THEN** stderr shows `Calling backend…`, then the error line, then `  (HTTP 422, code 1034)`
