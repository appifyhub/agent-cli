# Proposal

## Why

Users currently need `status` for local readiness and `server` for backend health, and neither shows which CLI version the backend advertises. One report can distinguish local readiness from backend availability and show whether the installed version matches the advertised version.

## What Changes

- **BREAKING:** Remove `server` with no alias. Use `status` for the backend report, but not as a health probe: its exit code describes local readiness, not backend health.
- **BREAKING for offline callers:** Plain `status` prints local readiness, then makes one public `GET /cli/status` with a three-second budget. It never falls back to `/health`. `status --offline` shows local readiness and the selected backend's last check time without sending requests or writing state.
- Show the local and advertised CLI versions. Equal SemVer precedence is `latest`; only a newer advertised version is `update available`, with a link to Releases. These results do not change the readiness exit.
- Record the completion time of each online check, successful or failed, separately for each backend. Keep normal local exits (0 or 3) for ready, disconnected, locked and expired states. Do not authenticate, unlock credentials, contact GitHub or install updates.

## Capabilities

### New Capabilities

- `status`: local readiness, backend status and version, CLI version comparison, and last online check for the selected backend. This durable spec replaces the archived bootstrap contracts for offline-by-default `status` and standalone `server`; historical archives are unchanged.

### Modified Capabilities

None. The durable spec inventory is empty.

## Impact

The CLI change affects command registration and completion, the public status client, status rendering, local check history, catalog/help, embedded agent guidance, and unit, golden and binary tests. At `/cli/status`, the backend requires `status` and `service_version` and may return a nullable `cli_version`. `/health` stays independent. No credential or configuration schema changes are required.
