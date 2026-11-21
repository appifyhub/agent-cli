# Tasks

## 1. Public status contract

- [x] 1.1 Use one anonymous `GET /cli/status` with required `status` and `service_version` and optional, nullable `cli_version`. Remove the unused health client. Exercise the request and failure contract in the existing API tests.

## 2. Aligned status and local history

- [x] 2.1 Preserve local readiness and output order, use one bounded lookup without fallback, and show the backend, local CLI and remote CLI in aligned, consistently styled rows. Separate token storage and expiry. Cover readiness, matching and mismatched versions, invalid targets, cancellation, timeout, safe diagnostics and write failures in existing CLI tests.
- [x] 2.2 Save completion times for successful and failed online attempts atomically per backend. Show the time offline without requests or writes, or `never checked` if absent. Cover separate backends, failures, state errors and timestamps in existing tests.
- [x] 2.3 Update catalog-backed help, golden files, agent guidance and the existing e2e loopback fixture. Preserve removal of `server` and distinguish `status --offline` from online diagnostics without treating the status exit as a backend health probe.

## 3. Verification and owner review

- [x] 3.1 Run the project build, vet, tests (race detector except on Windows), lint/EOF and e2e gates and validate this change's OpenSpec artifacts.
- [x] 3.2 **STOP for owner review.** Present the verified default and offline behavior and results. Do not archive or proceed automatically.
