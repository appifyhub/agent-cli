# Design

## Context

`internal/cli/status.go` loads config once, parses an environment token locally, and checks file presence (not contents) for file-backed stores. It writes the report to stdout. Disconnected status returns a silent exit 3 before backend resolution; locked and expired status return the same silent outcome after the full report. A malformed environment token also exits 3 but is an error, not a report. The new check must distinguish these cases without rerunning status.

## Goals / Non-Goals

- Keep readiness local, independent of the public result. Offline mode reads but never updates check history.
- Do not authenticate, verify revocation, fetch a raw `VERSION` file from the CLI, cache remote metadata locally or update the binary. The backend owns the shared CLI version cache; `/health` neither fetches CLI metadata nor depends on this cache.

## Decisions

1. **Finish the local report first.** Evaluate readiness once and report it before network work. A normal exit 3 for disconnected, locked or expired still allows an online check. Hard local errors and failed local writes stop before any network work. Choose the backend with `config.APIURL` (environment > config > build); a missing or invalid address skips the check and never falls through to another address. Display valid addresses without their HTTP(S) scheme and do not echo an invalid address.
2. **Make one bounded public call.** Use the existing client transport, build User-Agent, size limit and redirect refusal for `GET /cli/status` with no token, request body or query. Require HTTP 200 and a flat JSON object with a string `status` and string `service_version`. The optional `cli_version` accepts a string or null. Ignore unknown fields. Time the call through the body read; a three-second child context covers the whole call. Propagate caller cancellation, and treat timeout within the caller's live context as an unavailable result. There are no retries or `/health` requests.
3. **Persist completed attempts independently.** Store only the completion timestamp, in the CLI home, keyed to the normalized selected URL. Use a separate atomic state file per backend so switching backends retains history and connection or default changes cannot overwrite it. Record transport, HTTP, timeout and body failures as completed checks. Missing/invalid addresses and caller-cancelled checks do not update history. Store before rendering remote results; propagate required persistence and output errors. Offline reads the same record but never creates, changes or refreshes it.
4. **Compare SemVer precedence.** Show the installed and advertised CLI versions in aligned rows. Equal precedence says `latest`; a strictly newer advertised version says `update available` with a Releases link. Do not claim an update when the installed version is ahead or either version cannot be compared. Green highlights healthy backend status and `latest`; yellow highlights unavailable or unhealthy backend status and available updates. Other rows are uncolored. Keep separate token storage and expiry rows, backend latency when usable, and the last-checked timestamp or `never checked`. Sanitize remote values before rendering. Verbose stderr contains only existing request and numeric HTTP diagnostics.
5. **Keep one command and canonical guidance.** No hidden `server` command, alias or shim. Document status modes in catalog-backed help, and retain `{name} status --offline` for readiness preflight in the embedded guide. Plain `{name} status` performs backend diagnostics and records check time. Leave `skill status` (installation status) alone.

## Risks / Trade-offs

- A slow or missing backend can delay command completion → print local stdout first, enforce the overall deadline, and document the zero-request opt-out.
- A backend can advertise a version before matching binaries are downloadable → link to Releases without probing it or claiming an asset exists.
- Tests that formerly invoked plain status against build defaults could contact real services → use `--offline` for no-request guarantees and loopback/fake transports for every online case, including fresh-machine e2e runs.

## Migration Plan

**BREAKING:** Replace `server` invocations with `status` for human-readable diagnostics. Scripts must not substitute `status` as a backend health exit-code probe: disconnected plus healthy exits 3; connected plus degraded, unreachable or rate-limited still exits 0. Probes that need backend health must check `/health` directly. Use `status --offline` when no request or state write is allowed. Neither archived spec is edited.

An older backend without the merged `/cli/status` contract shows unavailable remote status, with no fallback request. No data migration is required; histories are created on the first completed online check for each backend.
