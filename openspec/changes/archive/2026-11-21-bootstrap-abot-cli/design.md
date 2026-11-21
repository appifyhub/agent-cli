# Design

## Context

See `proposal.md` for motivation and `specs/*/spec.md` for the behavioral contract. This document records how the CLI is built.

**Starting point.** `agent-cli` is empty apart from OpenSpec scaffolding and has no `AGENTS.md`. Go 1.27.4 is already installed on the development machine (`go version go1.27.4 darwin/arm64`), so M0 can start immediately. Every other version below was read from the authoritative sources on 2026-11-14 (go.dev release feed, the Go module proxy, GitHub release APIs, GoReleaser and Homebrew documentation).

**Sibling repositories that constrain this change.**

- `agent-backend` (FastAPI). Its error handler returns `{"error_code": <int>, "message": "[E1034] ⚠️ text # Caused by: ...", "emoji": "⚠️"}` (`util/errors.py`, `main.py`); the JWT dependency raises framework `{"detail": ...}` bodies; the existing `INSUFFICIENT_CREDITS` is `1034` with HTTP 422; chat ids are UUID hex strings; chat titles are nullable; connect keys are normalized with `strip().upper()`; `credit_balance` is a float. The backend change `add-cli-access` pinned the CLI contract used here, see "Backend contract" below.
- `agent-web` (change `add-command-line-page`). It publishes install commands that must match what this change ships: `brew install appifyhub/tap/abot`, `scoop bucket add appifyhub https://github.com/appifyhub/scoop-bucket` + `scoop install abot`, `sudo apt install ./abot_*.deb`, `sudo dnf install ./abot-*.rpm`. This design adopts exactly those names.
- Pipelines of both repositories are branch-based and driven by a version file; D14 mirrors them.
- Other organization conventions: lowercase comments that are rare, no TODOs, weekly grouped Dependabot on Saturdays at 10:00 Europe/Berlin with a `Dependabot` label, a PAT secret `CI_TOKEN_ABOT` and the bot identity `the-agent-abot` for automation commits, MIT license "Copyright (c) 2024-Present AppifyHub", `SECURITY.md` and `CONTRIBUTING.md` in every repository.

**Backend contract (pinned by `add-cli-access`, D16 of that change; reconciled on 2026-11-14).** The CLI classifies by HTTP status and numeric error code and prints its own text per code.

| Situation | HTTP | Code (number) |
|---|---|---|
| Unknown or used connect key | 404 | `CONNECT_KEY_NOT_FOUND` (2015) |
| Bad TTL syntax / TTL above 2 years | 400 | `INVALID_CLI_TTL` (1043) / `CLI_TTL_TOO_LONG` (1044) |
| Bad device id, hostname or os | 422 | `INVALID_CLI_DEVICE` (1045) |
| Token invalid, expired, wrong scope | 401 | `INVALID_CLI_TOKEN` (4004) |
| Token generation revoked | 401 | `CLI_ACCESS_REVOKED` (4005) |
| Account merged away | 404 | `USER_NOT_FOUND` (2001, existing) |
| Path user differs from token user | 403 | `NOT_TARGET_USER` (3002, existing) |
| Policies not accepted (notify routes only) | 403 | `WAITLIST_INVITED_POLICIES_REQUIRED` (3008, existing) |
| Still on the waitlist (notify routes only) | 403 | `WAITLIST_ACCOUNT_NOT_ACTIVE` (3007, existing) |
| No private chat on platform (or at all) | 403 | `NO_PRIVATE_CHAT` (3004, existing) |
| Chat exists but undeliverable (WhatsApp window, blocked bot) | 403 | `CHAT_UNREACHABLE` (3010) |
| Not admin or member of `--chat` | 403 | `NOT_CHAT_ADMIN` (3001) / `NOT_CHAT_MEMBER` (3009), existing |
| Unknown chat id / malformed chat id | 404 / 422 | `CHAT_NOT_FOUND` (2002) / `MALFORMED_CHAT_ID` (1030), existing |
| Malformed user id (a CLI bug, no special text) | 422 | `MALFORMED_USER_ID` (1029, existing) |
| Insufficient credits | 422 | `INSUFFICIENT_CREDITS` (1034, existing) |
| Empty or oversized message / bad platform | 422 | `INVALID_NOTIFICATION_MESSAGE` (1046) / `INVALID_PLATFORM` (1019, existing) |
| Provider throttling | 429 | `NOTIFICATION_RATE_LIMITED` (6004) |
| Provider failure or timeout | 502 | `NOTIFICATION_DELIVERY_FAILED` (5018) |

Also pinned: token problems are always 401, never 403; connect, chat listing and revoke-all are not gated by policies or the waitlist, the two notify routes are; `/cli/connect` returns `expires_at` as RFC 3339 UTC with `Z`, `user_id` as UUID hex, a non-empty `user_display_name` and `credit_balance` (read after the new-device alert charge); both notify responses carry `credit_balance` (read after the delivery charge); `device_id` must be a non-blank string (the CLI always sends a UUID), `hostname` must match `^[A-Za-z0-9.-]{1,32}$`, `os` must match `^[A-Za-z0-9._/-]{1,32}$`; `ttl_as_text` omitted or null means 6 calendar months and a blank string is invalid; the edge 429 for `/cli/connect` has a non-JSON body while the application's 429 has JSON.

## Goals / Non-Goals

**Goals:**

- A small, boring Go program whose every behavior is covered by offline tests and traceable to a spec scenario.
- Layering that lets a Bubble Tea TUI and a chat command be added later without touching the lower layers.
- Secrets handled conservatively: never logged, never in config, never in argv, deleted when the server says they are dead.
- Identical behavior on macOS, Linux and Windows, with per-OS instructions instead of per-OS code paths wherever possible.
- A release process that feels exactly like the other agent repositories.

**Non-Goals:**

- Chat, streaming responses, a TUI, a device list, per-device revoke, telemetry, update checks, apt/yum repositories, winget, translations of the CLI text, shell completion for `@abot`, background daemons or agents for unlocking.
- Signing of any kind: no Apple code signing or notarization, no Windows Authenticode, no cosign. Checksums only.
- Multiple backend profiles as a feature: `THE_AGENT_CLI_HOME` plus `THE_AGENT_CLI_API_URL` already isolate setups on one machine.
- Parsing or validating the TTL, or the connect key format, on the client: the server owns both.
- Verifying JWT signatures on the client (impossible: the secret is server-side). Claims are read only for display and for the user id of `THE_AGENT_CLI_TOKEN`.

## Decisions

### D1. Toolchain and dependency versions

Latest stable on 2026-11-14. Direct dependencies are added in the milestone that first uses them, so every milestone's `go.mod` diff stays reviewable.

| Item | Version (date) | Role and justification |
|---|---|---|
| Go | **1.27.4** (go.dev feed head; 1.27.0 shipped Aug 2026; installed locally) | `go 1.27.4` in `go.mod`, no separate toolchain line, so contributors auto-download exactly this patch. Gives the new stdlib `uuid` package (no `google/uuid`). Go 1.27 binaries need macOS 13 or later; recorded in the README. |
| `github.com/spf13/cobra` | **v1.10.2** (2025-12-03) | Commands, help, `--version`, shell completion for bash, zsh, fish and PowerShell. Used by most Go CLIs and compatible with Charm tooling later. `-v` stays free for `--verbose` because Cobra only adds the `-v` version shorthand when it is unused. |
| `github.com/spf13/pflag` | **v1.1.0** (2026-11-06), indirect | Latest. Cobra v1.10.2 itself requires v1.0.9; if a regression shows up, pin v1.0.10 as pflag's release notes advise. |
| `github.com/pelletier/go-toml/v2` | **v2.4.3** (2026-07-05) | `config.toml` and the strings catalog. Native `time.Time`, struct tags with `omitempty`, strict decoding available. |
| `github.com/zalando/go-keyring` | **v0.2.8** (2026-03-23) | OS credential store without cgo, so one Linux runner cross-compiles every target. macOS through `/usr/bin/security -i` (secret sent on stdin, base64-wrapped), Linux through the Secret Service on D-Bus (`godbus/dbus/v5` v5.2.4), Windows through Credential Manager (`danieljoos/wincred` v1.2.3, 2560-byte limit). Same library as GitHub's CLI. `MockInit` exists but is not used; the wrapper takes an interface instead. |
| `filippo.io/age` | **v1.3.2** (2026-08-29) | Passphrase file: ASCII-armored age file with a scrypt recipient (work factor 18, about one second; ChaCha20-Poly1305). Standard audited format, versioned header, wrong passphrase reported as `age.ErrIncorrectIdentity`, `SetMaxWorkFactor` caps the cost of hostile files. Indirect: `filippo.io/edwards25519` v1.2.0, `filippo.io/hpke` v0.4.0, `filippo.io/nistec` v0.0.4, `golang.org/x/crypto` v0.59.0. Its docs discourage scrypt for automation; automation uses `THE_AGENT_CLI_TOKEN`. |
| `golang.org/x/term` | **v0.48.0** (2026-11-09) | TTY detection and raw-mode input for the masked passphrase prompt on all three OSes. |
| `golang.org/x/sys` | **v0.50.0** (2026-11-06) | Windows console: ANSI processing, `GetConsoleOutputCP` and `SetConsoleOutputCP` (all present in this version). |
| golangci-lint | **v2.15.0** (2026-10-16) | Lint and format, v2 config. |
| GoReleaser | **v2.20.2** (2026-11-12) | Cross-compiles the release builds, archives, checksums, release notes and the GitHub release (D14); M11 adds the Homebrew, Scoop and nFPM packages. Run through `tools/release`, pinned as `~> v2` in the action. |
| govulncheck (`golang.org/x/vuln`) | **v1.9.0** (2026-10-11) | Dependency vulnerability gate. |
| GitHub Actions | `actions/checkout` v7, `actions/setup-go` v7, `golangci/golangci-lint-action` v9, `goreleaser/goreleaser-action` v7, `anthropics/claude-code-action` v1 (as in the sibling repos) | Current major tags, consistent with the org's major-tag pinning. |
| Future TUI (not added in v1) | `charm.land/bubbletea/v2` v2.1.1, `charm.land/lipgloss/v2` v2.1.0 | Noted only to confirm the layering in D2 fits them. |

Rejected: `99designs/keyring` (heavier, cgo options), `BurntSushi/toml` (equivalent; go-toml chosen for strict decoding and wider use), `spf13/viper` (implicit layering is hard to test; our precedence rules are explicit and tiny), `urfave/cli` and `kong` (Cobra has built-in completion and is the Charm-friendly default), `fatih/color` (about 40 lines of our own color policy replace it), `stretchr/testify` (stdlib `testing` plus helpers keeps the dependency list short), a hand-built Argon2id plus AEAD file format (custom crypto and file format to maintain for no gain over age).

### D2. Package layout and dependency rules

```
agent-cli/
├── cmd/agent-cli/main.go       wiring only: real dependencies, console setup, signal context, os.Exit(cli.Run(...))
├── internal/
│   ├── buildinfo/              ldflags variables, defaults, version line
│   ├── text/                   strings.toml (go:embed), catalog, {placeholder} rendering, OS variants
│   ├── term/                   streams, TTY detection, color policy, masked and line prompts, text sanitization, Windows console
│   ├── config/                 home directory, config.toml model and atomic IO, device id, API URL and default target
│   ├── token/                  unverified JWT claims (sub, exp) and the shape check
│   ├── credentials/            keychain, plain file, passphrase file; store plan; resolver; removal
│   ├── api/                    HTTP client, DTOs, error decoding and classification, code constants
│   ├── session/                authenticated calls: token resolution plus rejected-token handling
│   ├── render/                 relative and local time, display paths, platform names, credits, chat table
│   ├── agentskill/             agent table, user/project skill locations, stub ownership, install and removal (M10)
│   ├── cli/                    Cobra commands, help templates, exit-code mapping, Run
│   └── testkit/                test-only fakes: backend, keychain, terminal, clock, golden files, CLI runner
├── e2e/                        tests of the four real builds from tools/build, run by scripts/e2e.sh on each OS
├── version.go, VERSION         the release version, MAJOR.MINOR.PATCH, embedded: the single source of truth
├── tools/                      POSIX shell scripts: variants, build, release, bump_version, create_pr, messages
├── .goreleaser.yaml            release builds, archives, checksums, notes (packages from M11)
├── .golangci.yml, Makefile
├── .github/                    workflows (D14), dependabot.yml, pull request template
└── (later) internal/tui/       Bubble Tea models, a sibling of cli
```

Rules, enforced by review and by a small import test in M0: dependencies point downward only (`cli` → `session` → `credentials`, `api`, `config`, `token`); `term` and `agentskill` import no domain package; `api` imports only `buildinfo`; nothing below `cli` imports Cobra or writes to stdout or stderr; `testkit` is imported only from tests. A later `tui` package is a sibling of `cli` over the same lower layers (`session`, `api`, `config`, `render`, `text`); `cmd/abot` decides which front end handles a command. There is deliberately no separate "use case" layer: each command's orchestration is one function in `cli`, which avoids single-use abstractions until a second front end actually needs sharing.

### D3. Testing strategy

All tests are offline and deterministic on all three operating systems.

- **Seams instead of mocks.** `cli.Deps` carries the environment lookup, stdin/stdout/stderr, a `term.Terminal`, a clock, a `*time.Location`, GOOS/GOARCH, the hostname function, the home directory function, the keychain interface, the HTTP transport and the scrypt work factor. No globals are touched; `t.Setenv` is used only for process state where Deps is not involved.
- **Fakes in `internal/testkit`.** `Backend` is an `httptest.Server` that records requests (method, path, headers, body) and answers in the real shapes, including `[E1034] ⚠️ ...` messages, framework `{detail}` bodies, non-JSON 429 and 502 bodies, and `credit_balance`. `FakeKeychain` implements the keychain interface with injectable failures. `FakeTerm` scripts TTY states and prompt answers and records prompts. Builders create DTOs (`ConnectResponse`, `Chat`) so tests override only what a scenario needs. `Run(t, deps, args...)` returns exit code, stdout and stderr.
- **Levels.** Pure units (relative time, credits formatting, hostname cleanup, shell quoting, URL checks, classification table, masked-input reader); component tests with temp directories (config, credentials, api); command tests through `cli.Run` asserting stdout, stderr, exit code and the requests the backend saw; one process-level test in `cmd/abot` that re-executes the test binary to prove the exit code reaches the OS and that panics never print traces.
- **Built binaries.** `e2e/e2e_test.go` runs every build from `tools/variants`, built by `tools/build` with its build-time values, on Linux, macOS and Windows in CI (`scripts/e2e.sh`, `make e2e`). It covers what in-process tests cannot: the values passed at build time, the real `main` with real streams and exit codes, and the per-build names and URLs. Each milestone that adds a command adds that command's offline basics there; the scripted journey of task 12.7 stays in process.
- **Shell tools.** `tools/bump_version`, `tools/build`, `tools/release` and `tools/create_pr` are tested by Go tests (`tools/tools_test.go`, skipped on Windows) that run the scripts with `sh` inside temporary git repositories and a local bare repository standing in for `origin`; `create_pr` is tested on its paths that do not need the GitHub CLI.
- **Golden files** (`testdata/*.golden`, `go test -update`) pin help output per command and OS variant, so a catalog edit shows up as a reviewable diff.
- **Spec traceability.** Every scenario in the specs maps to at least one test; tasks name the cases. Secret leakage is checked by `testkit.AssertNoSecret(t, secret, outputs...)` in every command test that handles a token, key or passphrase.
- **Race detector** on Linux and macOS in CI; Windows runs the same tests without `-race` (it needs cgo there). Tests that need POSIX permission bits skip on Windows.
- **Not automatable offline** (recorded as manual checks in the owning milestone): the real keychain on each OS, real masked prompts, Ctrl-C echo restore, the Windows console code page, and the smoke run against the staging backend in M13. An opt-in test guarded by `THE_AGENT_CLI_TEST_KEYCHAIN=1` exercises the real credential store.

### D4. One strings catalog

All user-facing text lives in `internal/text/strings.toml`, embedded with `go:embed`.

- **Shape.** Tables per area (`root`, `connect`, `status`, `chats`, `notify`, `unlock`, `error`, `warning`, `store`, `glyph`...), leaf strings, multi-line help in TOML `'''` strings, comments allowed.
- **Addressing.** Dotted keys like `connect.success_title`. Per-OS variants are sibling leaves with a suffix: `hint_windows`, `hint_darwin`, `hint_linux`, `hint_unix`. Resolution tries `<key>_<goos>`, then `<key>_unix` (every OS except windows), then `<key>`.
- **Rendering.** `{name}` placeholders filled from name/value pairs; `{{` and `}}` are literal braces. The Go code passes data, never English sentences.
- **Checks run as tests (M0):** the file decodes strictly; every leaf is a non-empty string; every key literal passed to the catalog in non-test code exists (AST scan with `go/parser`); no key is orphaned; placeholders used at call sites match the entry; every entry renders for GOOS darwin, linux and windows. The root help layout (`Usage:`, `Commands:`, `Flags:`, `Environment:`, `Exit codes:`) is a Go template whose labels and contents come from the catalog.
- **Exception.** Messages produced by the flag parser for malformed invocations are passed through, reduced to their first line.
- **Placeholders for build values.** `{product_url}`, `{version}` and the like come from `buildinfo`, never hard-coded in the file.
- **Decision: the "2 years" in the TTL-too-long message** lives in this catalog like every other sentence. If the backend maximum ever changes, the maintainer edits one catalog entry.

Default texts for every approved example are listed in the specs and are the golden expectations; the catalog is the single place to reword them. Usage documentation lives in the help texts of this catalog, not in the README, so it always matches the installed version.

### D5. Output, errors and exit codes

- **Streams.** Results to stdout. Prompts, warnings, errors, verbose lines and `unlock` confirmations to stderr, so `eval "$(agent-cli unlock)"` can only ever evaluate the assignment line.
- **One error path.** Domain packages return typed errors (`session.ErrNotConnected`, `credentials.ErrLocked`, `credentials.ErrWrongPassphrase`, `api.Error{Kind, Status, Code}`, ...). `cli` maps each to `{exit code, catalog key, args}` in one table and prints `error: <text>`; the not-connected guidance is the only line without the prefix. Cobra runs with `SilenceUsage` and `SilenceErrors`. Flag, argument and unknown-command failures become usage errors (exit 2), reduced to their first line.
- **Exit codes.** `0 ok`, `1 failure`, `2 usage or validation`, `3 not connected, locked or revoked`, `4 chat unreachable`, `5 insufficient credits`, `6 rate limited`, as constants in `cli`. Aborted prompts and declined confirmations exit 1.
- **Warnings** (`warning: ...`) are not errors: they go to stderr and never change the exit code. The only warnings are the low-credit notice, the stdin truncation notice and the environment-default override notice.
- **Robustness.** `Run` recovers panics: exit 1 and a one-line error, the stack only with `--verbose`. `--verbose` lines are `Calling backend…` before each request and `  (HTTP <status>, code <n>)` after an API error; never headers, bodies or secrets.
- **Terminal safety.** Everything that originates outside the process (titles, names, server messages) passes through `term.Clean`: control characters and escape sequences removed, whitespace collapsed, server messages cut to 200 characters. Tables use `text/tabwriter`.

### D6. Local state

```toml
# managed by the command-line client; contains no secrets
device_id = "6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b"
api_url = "https://agent.cloud.appifyhub.com"

[token]                       # metadata only; the token lives in the chosen store
store = "keychain"            # keychain, passphrase or unsafe
user_id = "9f1c2d3e4a5b6c7d8e9f0a1b2c3d4e5f"
user_name = "Morpheus"
hostname = "morpheus-mbp"
expires_at = 2027-02-14T09:21:00Z

[default]                     # chat_* keys or platform, never both
chat_id = "3f2a9c1e7b6d4a58b0c1d2e3f4a5b6c7"
chat_platform = "telegram"
chat_title = "Morpheus"
chat_private = true
```

- **Home.** `THE_AGENT_CLI_HOME` (made absolute) or `<user home>/.<name>` (`<name>` is the command name of the build, so builds with different names installed side by side never share state); if the user home cannot be determined the command fails with exit 1 and tells the user to set `THE_AGENT_CLI_HOME`. Created with `MkdirAll(0700)` only by commands that write state; `connect` creates it in its pre-flight (and proves it writable with a temporary file) so a failed connect can leave an empty directory but never a state file.
- **Writes.** Temporary file in the same directory (`0600`), fsync, rename over the target, so readers never see a partial file; last writer wins, no file locking in v1. A missing file reads as empty; malformed TOML or an invalid `device_id` is an error naming the file; unknown keys are ignored on read and dropped on the next write. A fixed header comment is prepended because the TOML encoder does not emit comments.
- **Device id.** Derived inside `connect` only, so `--help`, `--version`, `completion` and `status` never touch the disk, and a failed connect leaves no state file; the record is saved with the first successful connect. The id is a UUID (version 8, RFC 9562, custom data) taken from the first 16 bytes of a SHA-256 hash over a fixed namespace string, the seed and the absolute home directory. The seed is, in order: the identifier the OS keeps for the installation (macOS hardware UUID through `sysctl kern.uuid`, Linux `/etc/machine-id`, `/var/lib/dbus/machine-id` or FreeBSD `/etc/hostid`, Windows `MachineGuid` in the registry; placeholder values such as all zeros are rejected); else the sorted universally administered MAC addresses of the network interfaces plus host name, user name, `GOOS` and `GOARCH` (locally administered addresses are skipped because virtual interfaces, VPNs and per-network privacy features invent and change them; without any hardware address the properties alone are not used, since host names like `ubuntu` collide); else a random v4 UUID. Hashing keeps the raw identifier and addresses off the wire, and the home directory in the hash keeps `abot` and `agent-cli` (and any `THE_AGENT_CLI_HOME`) apart on one machine. The id is read once and stored, so a later change of network hardware never changes it; a deleted config yields the same id again.
- **Effective settings** are computed by pure functions of (environment, config, build defaults): API URL (`THE_AGENT_CLI_API_URL` > config > build default, trailing slash trimmed, `https` or loopback `http`, no userinfo, query or fragment) and the default target (`THE_AGENT_CLI_DEFAULT_CHAT` > `THE_AGENT_CLI_DEFAULT_PLATFORM` > config). The API URL is validated where it is used, so informational commands never fail on a bad `THE_AGENT_CLI_API_URL`.
- **Display paths** replace the user home prefix with `~` using the OS separator.
- **`env` command.** Because usage lives in the help and not in the README, `env` is how users discover the variables they can set: it prints each `THE_AGENT_CLI_*` variable (and `NO_COLOR`) with its purpose, whether it is set and the value in effect, computed by the same effective-settings functions as every other command. Secrets show only that they are set. It is informational like `--help`: offline, no files touched.

### D7. Credential storage

- **Interface.** `credentials.Store` has `Save`, `Load`, `Delete`; implementations: `keychain` (go-keyring behind a small interface; service `<name>`, account = the device id, so separate `THE_AGENT_CLI_HOME` directories never overwrite each other), `plainFile` (`credentials`, token plus newline, `0600`) and `passphraseFile` (`credentials`, ASCII-armored age file, scrypt work factor 18 for writes, decrypt cap 20, injectable factor for tests).
- **Selection** (`credentials.Plan`, run before the connect request):
  1. explicit `--store` value is validated, then honored with no silent fallback: `keychain` requires a successful probe, `passphrase` requires a terminal or `THE_AGENT_CLI_PASSPHRASE`, `unsafe` is always possible;
  2. auto: probe the keychain (set, then delete a throwaway entry; any error means unusable); else `passphrase` if a terminal or `THE_AGENT_CLI_PASSPHRASE` exists; else exit 2 with the `--store unsafe` / `THE_AGENT_CLI_TOKEN` suggestion;
  3. for `passphrase`, the new passphrase is collected now: from `THE_AGENT_CLI_PASSPHRASE`, or two masked prompts (mismatch and empty are exit 2).
- **Resolution** (`credentials.Resolve`): `THE_AGENT_CLI_TOKEN` (shape-checked) > stored token. A passphrase token is unlocked with `THE_AGENT_CLI_PASSPHRASE`, else a masked `Passphrase: ` prompt if stdin is a terminal, else `ErrLocked` immediately (stdin is never read when it is not a terminal, so pipelines cannot hang). Missing secrets map to `ErrNotConnected` plus metadata cleanup; keychain failures other than not-found map to `ErrStore` (exit 1, token kept); undecryptable files map to `ErrDamaged` (exit 3, file kept).
- **Persistence order at connect:** save secret, then config, then delete the secret left in the previous store if the mode changed; a config failure rolls the new secret back; a secret failure leaves the config untouched.
- **Forget** deletes the secret (not-found is success) and clears the `[token]` table, keeping `device_id`, `api_url` and `[default]`.
- **Why the OS store is reached through `/usr/bin/security` on macOS.** The unsigned binary never triggers Keychain access prompts, at the price that the item is readable by any process of the same user that runs `security find-generic-password` while the keychain is unlocked. This is the same model as GitHub's CLI and is documented in the README.

### D8. Backend client and classification

- **Transport.** One `http.Client` per run: 30 s timeout, redirects not followed (3xx is unexpected), default transport so `HTTPS_PROXY` is honored, response bodies capped at 4 MiB, headers `Accept`, `Content-Type` and `User-Agent: <name>/<version> (<os>/<arch>)`, `Authorization` only on authenticated calls, no retries (connect keys are single-use and notifications are billed).
- **Decoding.** Error bodies are tried as `{error_code, message}`, then `{detail}`, else treated as opaque. Server text is cleaned: leading `[E<digits>]` marker plus following symbols removed, trailing ` # Caused by:` part removed, then `term.Clean`.
- **Success bodies.** `connect` and the chat list are validated strictly (required fields, `expires_at` as RFC 3339). A notify call is different: the message may already be sent and billed, so any 2xx is success and body problems only degrade the `-v` line. `credit_balance` is read through `json.RawMessage` and accepted only when it is a JSON number; anything else (missing, null, string) is treated as absent, never as an error.
- **Classification** is one table in `internal/api` (`Classify(authenticated bool, status int, code int)`), first match wins:

| Condition | Outcome | Exit | Local token |
|---|---|---|---|
| transport failure, timeout | Unreachable | 1 | kept |
| 429 | RateLimited | 6 | kept |
| 3xx | Unexpected | 1 | kept |
| 5xx (5018 gets its own text in `notify`) | ServerFault | 1 | kept |
| 403 + {3008, 3007} | AccountGate (policies not accepted / still on the waitlist) | 1 | kept |
| authenticated: 401 any; 404 + 2001; 403 with code not in {3001, 3004, 3007, 3008, 3009, 3010} (including no code and 3002) | TokenRejected | 3 | stored token removed, `THE_AGENT_CLI_TOKEN` left alone |
| 403 + {3001, 3004, 3009, 3010}; 404 + 2002 | ChatUnreachable | 4 | kept |
| 422 + 1034 | InsufficientCredits | 5 | kept |
| connect: 404 + 2015, any 401 or 403 | InvalidConnectKey | 1 | n/a |
| connect: 400 + 1043 / 1044 | InvalidTTL / TTLTooLong | 2 | n/a |
| connect: 422 + 1045 | DeviceRejected | 1 | n/a |
| other 400, 422 | Validation (catalog text for 1019, 1030, 1046; else cleaned server text; `{detail}` bodies get a generic text) | 2 | kept |
| anything else (unknown route, 409, ...) | Failure, text `error: unexpected response from the server` | 1 | kept |

  This refines the approved rule "any 401 or 403 on an authenticated call removes the local token and exits 3": the chat-target codes, the two account-gate codes and 404/2002 describe the target or the account state, not the token, and deleting a valid token because the user mistyped a chat id or has not accepted the policies would be wrong. Every other 401 and 403 keeps the approved behavior. The backend promises that token problems are always 401.
- **Code constants** live in `internal/api/codes.go` by name. M3 re-reads the backend's `util/error_codes.py` and updates numbers if they moved; the numbers in the context table above were confirmed against the backend change on 2026-11-14.

- **Server health.** `agent-cli server` is the one call that needs no token and no connection: `GET /health`, answered by the backend (`src/main.py`) as `{"status": "ok", "version": ...}`. The route is not in the backend's OpenAPI document, so the contract is pinned from its source, and the decoder is tolerant on purpose: only a string `status` is required, `version` is optional, other top-level scalars are shown as they come (alphabetical, so a future `region` or `uptime` appears without a CLI release), nested values are ignored. A status other than `ok` prints the report and exits 1; transport and HTTP failures go through the same `Classify` table as every other call. It is also the first command that works against a real backend before any connect key exists, so M3's review includes a real call to staging.

### D9. Token claims

`internal/token` splits a JWT, base64url-decodes the payload and reads `sub`, `exp` (plus `device_id`, `hostname` when present). There is no signature check and no authorization decision depends on it. Uses: the user id for `THE_AGENT_CLI_TOKEN` (stored tokens use the `user_id` saved at connect), the expiry for `status` with `THE_AGENT_CLI_TOKEN`, and the shape check (three dot-separated non-empty segments, decodable payload) that turns garbage into exit 3 before any request.

### D10. Command orchestration

- **`connect`:** parse and normalize key; load config; validate the API URL; pre-flight (create the home directory with `EnsureHome` and prove it is writable with a temporary file, then build the store plan from D7); `api.Connect`; persist (D7); print; low-credit check. TTL is forwarded verbatim. The hostname is cleaned (runs of characters outside `[A-Za-z0-9.-]` become one `-`, leading and trailing `-` or `.` trimmed, 32 characters max, `unknown` if empty) so the displayed host equals the one the server records. Everything that can fail locally is therefore decided before the single-use key is spent.
- **`revoke-all`:** connection check (no token metadata and no `THE_AGENT_CLI_TOKEN` is exit 3, before any prompt); confirmation (terminal prompt, `--yes`, or exit 2); token resolution (may prompt for the passphrase); `DELETE /cli/user/{user_id}/devices`; success and token rejection both remove the local token; a rejected `THE_AGENT_CLI_TOKEN` is an error that leaves stored credentials alone; a successful call made with `THE_AGENT_CLI_TOKEN` also removes the stored token.
- **`status`:** config plus environment only. Connected means token metadata exists and, for file stores, the `credentials` file exists; the keychain is never touched. Exit 3 for not connected, locked (passphrase store, no `THE_AGENT_CLI_TOKEN`) and expired, so `agent-cli status` is a usable readiness probe in scripts; the report is still printed in those states.
- **`chats`:** list through `session`; `default`, `default --platform` and `--clear` are local; `default --set <id>` validates against the server list (exact match, case-insensitive) and caches id, platform, title and privacy so `status` stays offline. Without flags, display the effective default; accept at most one mutation form.
- **`notify`:** usage validation first (conflicting flags, platform value, message), then token resolution, then the call, so usage errors win over connection errors. Target resolution is a pure function of flags, environment and config (the precedence table in the spec). stdin is read only when no argument exists and stdin is not a terminal, through a bounded reader (at most 4 × 4096 bytes), invalid UTF-8 replaced, a leading BOM and surrounding whitespace removed, truncated at 4096 characters with a stderr warning. After a 2xx the low-credit check runs even though the command is otherwise silent.
- **Low-credit warning** (shared by `connect` and `notify`): `cli.LowCreditThreshold = 100` credits is a named constant. When `credit_balance < LowCreditThreshold` the catalog entry `warning.low_credit` is printed to stderr with `{credits}`, truncated (not rounded) to two decimals so a balance of 99.999 never prints as `100.00`. Amounts are shown only in credits, with no currency conversion. The exit code is unchanged. The formatting helper lives in `internal/render`.
- **`unlock`:** resolves the token like any command but never calls the API; prints exactly one assignment line chosen by `--shell` (`sh`, `bash`, `zsh`, `fish`, `pwsh`, `powershell`; default `pwsh` on Windows, `sh` elsewhere; explicit beats guessing from `$SHELL`) with single-quote escaping per shell.

### D11. Terminal handling

- **Interface.** `term.Terminal` exposes `IsTTY(in|out|err)`, `ANSI()` (whether the console renders escape sequences), `ReadSecret(ctx, prompt)` and `ReadLine(ctx, prompt)`. `term` imports no other package of the module; the mask glyph is passed in from the catalog.
- **Masked secret input.** `ReadSecret` switches stdin to raw mode with `term.MakeRaw` (which also disables Ctrl-C signal generation on Unix and clears `ENABLE_PROCESSED_INPUT` on Windows), prints the prompt on stderr and reads runes itself: printable runes append to the secret and echo the catalog mask once; Backspace and DEL remove the last rune and erase one mask; Enter finishes with a line break; Ctrl-C (byte `0x03`), Ctrl-D on an empty buffer and the end of input return `term.ErrAborted`; Ctrl-U clears the line; ESC sequences (arrow keys, Home, End, Delete) are consumed and ignored; other control bytes are ignored. The saved terminal state is restored on every exit path by the function that entered raw mode, including a cancelled context while the read is still blocked, so a failed read cannot leave echo off. `cli.Present` prints `Aborted.` for `term.ErrAborted` (exit 1), so prompts never print their own error text. The reader works on a `bufio.Reader` and an injectable raw-mode function so unit tests drive it with scripted bytes.
- **Line input** (`ReadLine`) is plain canonical input for the revoke confirmation.
- **Color** is on only when the stream is a terminal, `NO_COLOR` is unset or empty and `TERM` is not `dumb`; on Windows ANSI processing is enabled through `x/sys/windows` and color stays off if that fails. The success glyph `✓` and the other symbols are catalog entries.
- **Decision: Windows console encoding.** At startup, when stdout or stderr is a console, `main` switches the output code page to UTF-8 (65001) with `SetConsoleOutputCP`, enables ANSI processing, and restores the previous page and console modes before exiting (explicitly, not with `defer`, because `os.Exit` skips defers). There is no ASCII fallback. Redirected output never touches the console.

### D12. Relative time

For `d = target - now`: under 1 minute `less than a minute`; under 1 hour minutes (rounded); under 24 hours hours (rounded); under 30 days days (rounded, 24-hour days); otherwise calendar months between the two dates rounded to the nearest month (stepping with `AddDate`, comparing the remainder with half a month), shown as years when the count is a multiple of 12 and at least 12. Future prefixes `in `, past suffixes ` ago`, singular for 1. The clock and the `*time.Location` are injected, so the approved `2027-02-14 10:21 (in 3 months)` is reproducible.

### D13. Build identity and the four builds

The tool is generic: an open-source command-line client whose command name, default backend and product page are build-time values. The defaults describe a development setup (`agent-cli`, `http://localhost`). The hosted service gets convenience builds with its URLs baked in, released next to a generic build of the same commit:

| Build | Backend baked in | Released by |
|---|---|---|
| `agent-cli` | `http://localhost` | production release (push to `release`) |
| `abot` | `https://agent.cloud.appifyhub.com` | production release (push to `release`) |
| `agent-cli-dev` | `http://localhost` | beta release (pull request to `release`) |
| `abot-dev` | `https://staging.agent.cloud.appifyhub.com` | beta release (pull request to `release`) |

`tools/variants` is the only place that defines this table; `tools/build <name>` (local builds) and `tools/release` (GoReleaser) source it. `THE_AGENT_CLI_BUILD_API_URL` and `THE_AGENT_CLI_BUILD_PRODUCT_URL` override the URLs of a local build; releases always use the table.

`internal/buildinfo` exposes `Name` (`agent-cli`), `Commit` (`unknown`), `Date` (`unknown`), `APIURL` (`http://localhost`) and `ProductURL` (`https://agent.appifyhub.com`) as string variables set with `-X`. An empty value counts as unset, so a missing value can never blank a default. `Date` accepts RFC 3339 or `YYYY-MM-DD` and is shown as a date; builds pass the commit date of the built commit, so rebuilding the same commit reproduces the same binary.

**The version is not a build variable.** The repository root package `agentcli` embeds `VERSION` with `go:embed` and `buildinfo` reads it, so `go build`, `go install`, `tools/build` and GoReleaser all report the same plain version, and all four builds of a release share it. `VERSION` stays the single source of truth: the tags and release names are derived from it.

**The command name is everywhere the tool names itself.** Cobra's `Use`, every usage line and help text (catalog placeholder `{name}`), the version line, shell completion and the user agent use `Name`; the default home directory is `~/.<name>` and the keychain service is `<name>`, so differently named builds on one machine never touch each other's state. The environment variables are `THE_AGENT_CLI_*` in every build. The name `abot` appears only where the hosted builds are defined or distributed (`tools/variants`, the release workflows, M11's packages); code, catalog, specs and docs use `agent-cli` or the `{name}` placeholder.

### D14. Release engineering (mirrors the backend's pipelines)

The backend (`agent-backend`) has three workflows driven by one version file: `qa.yml` (pull requests to `main`), `beta.yml` (pull requests to `release`) and `release.yml` (push to `release`). Each lints, tests, builds and publishes, creating a GitHub release whose notes list the commits since the previous tag of the same quality; the beta and production runs then wait for the deployed service and ask it for a release summary. The CLI mirrors the triggers and the release flow; like the web app, it has no service to wait for and calls no summary endpoint.

| Trigger | `agent-backend` | `agent-cli` |
|---|---|---|
| Pull request to `main` | `qa.yml`: lint, tests, PR images and a PR pre-release | `qa.yml`: `verify.yml` only; nothing is published |
| Pull request to `release` | `beta.yml`: lint, tests, staging images, pre-release `v<ver>.beta`, PR comment, health wait, summary | `beta.yml`: `verify.yml`, then `tools/release beta`: pre-release `v<ver>-beta` with `abot-dev` and `agent-cli-dev`, PR comment |
| Push to `release` | `release.yml`: lint, tests, production images, release `v<ver>.ga`, health wait, summary | `release.yml`: `verify.yml`, then `tools/release ga`: release `v<ver>` with `abot` and `agent-cli` |
| Dependabot to `main` | `dependabot-main-pr.yml` | `dependabot-main-pr.yml` with `go mod tidy` |
| Mentions of `@claude` | `claude.yml` | identical |
| Version | `pyproject.toml`, `tools/bump_version` | `VERSION`, `tools/bump_version` |
| Release candidate PR | `tools/create_pr` | `tools/create_pr` |
| Release notes and tags | `tools/publish_github_release.py` | `tools/release` and GoReleaser |

**Semantic-version tags.** All builds of a release share the plain version, because their command names already tell them apart. GitHub tags are repository-wide, so the two releases of a version still need distinct tags; the CLI uses semantic-version forms of the backend's quality suffixes: `v<VERSION>-beta` for the beta and `v<VERSION>` for production. GoReleaser accepts both, marks the beta as a pre-release (`prerelease: auto`) and only the production release as latest. Each tag is published once, so a new release candidate needs a bumped `VERSION`, exactly like the backend.

**`tools/release {beta|ga} [--check|--snapshot]`** validates `VERSION`, refuses with a "bump the version" message when the tag exists locally or on `origin`, finds the previous tag of the same kind for the release notes, creates the annotated tag on the checked-out commit (`HEAD`) as `the-agent-abot`, pushes it, and runs `goreleaser release --clean` with `GORELEASER_CURRENT_TAG` and `GORELEASER_PREVIOUS_TAG` and the names and URLs of both builds from `tools/variants`. When GoReleaser fails it deletes the partial release and the tag so the run can be retried. It writes `release_tag` and `release_version` to `$GITHUB_OUTPUT` for the PR comment. `--check` stops after the validation; `--snapshot` builds everything into `dist/` without tagging or publishing. The GoReleaser and GitHub CLI executables are injectable (`GORELEASER`, `GH`) so the tests run it against a local bare repository and stubs.

GoReleaser refuses a tag that does not point at `HEAD`, so `beta.yml` checks out the pull request head instead of GitHub's default merge commit.

**`.goreleaser.yaml`** (`version: 2`) builds two binaries per target (`branded` and `generic` ids) for darwin, linux and windows on amd64 and arm64 with `CGO_ENABLED=0`, `-trimpath`, `-s -w`, the D13 ldflags and `mod_timestamp` from the commit; archives `<name>_<VERSION>_<os>_<arch>` (tar.gz, zip on Windows) with `LICENSE` and `README.md`; `checksums.txt` (SHA-256); a git changelog with the backend's ignore patterns for version-bump and merge-request commits; release name `The Agent CLI <VERSION>` (plus `(beta)`). `verify.yml` runs `goreleaser check` on every pull request.

**Workflows** (all under `.github/workflows/`): `verify.yml` (reusable, `workflow_call`: tests on Linux, macOS and Windows with `-race` on Linux and macOS, then the built-binary tests from `scripts/e2e.sh` on each; lint, newline check, `goreleaser check` and `govulncheck` on Linux), `qa.yml`, `beta.yml` (skipped for Dependabot), `release.yml`, `dependabot-main-pr.yml` (`pull_request_target` on `main`, only for `dependabot[bot]`: checkout of the head with `CI_TOKEN_ABOT`, `go mod tidy`, commit `go.mod` and `go.sum` as `Auto-lock: Update dependencies` as `the-agent-abot`, approve, enable auto-merge), `claude.yml` (byte-identical to the siblings, needs `CLAUDE_CODE_OAUTH_TOKEN`) and `.github/dependabot.yml` (`gomod` and `github-actions`, weekly Saturday 10:00 Europe/Berlin, grouped, label `Dependabot`, 5 open pull requests, 7-day cooldown). Releases use only `GITHUB_TOKEN` until M11 adds the tap and bucket.

**Version bumps and release candidates.** `tools/bump_version {major|minor|patch}` is a port of the backend script (POSIX sh, shared `tools/messages`, refuses to run without `go.mod`, validates the file, writes through a temporary file, verifies the result, prints `Version bumped: a -> b`). `tools/create_pr` is a port of the backend script: on `main` it opens `Release Candidate v<VERSION>` from `main` to `release` with label `Release` through the GitHub CLI, on another branch it prints the compare link, on `release` it refuses.

**Packaging (M11).** M11 adds the hosted production build to the package managers through GoReleaser's `brews`, `scoops` and `nfpms` sections, restricted to the `branded` build ids and skipped for betas:

- **Homebrew:** formula `abot` in `appifyhub/homebrew-tap` (user-facing tap name `appifyhub/tap`), `extra_install` adds `bin.install_symlink "abot" => "@abot"`, `test` runs `abot --version`, `skip_upload: auto` keeps pre-releases out. Users run the fully qualified `brew install appifyhub/tap/abot`; since Homebrew 6 that form trusts only this formula (a plain `brew tap` followed by a short name would need `brew trust`).
- **Why a formula and not a cask.** GoReleaser deprecated `brews` in v2.10 in favor of `homebrew_casks` (removal only in a future v3, undated). Casks are installed with the macOS quarantine attribute and Homebrew deprecated `--no-quarantine`, so an unsigned binary would need an `xattr -dr com.apple.quarantine` post-install hook that both GoReleaser and Homebrew call a security bypass. Formula downloads are not quarantined, so the unsigned binary just runs, on macOS and Linux with one definition. Migration trigger: GoReleaser v3 removes `brews`; GoReleaser documents the move (`tap_migrations.json`).
- **Scoop:** manifest `abot.json` in `appifyhub/scoop-bucket`, `skip_upload: auto`; Windows archives are zip; no alias.
- **Packages:** nFPM `deb` and `rpm`, package name `abot`, `/usr/bin/abot` plus a `type: symlink` content `/usr/bin/@abot` → `abot`, default conventional file names (`abot_<v>_<arch>.deb`, `abot-<v>-1.<arch>.rpm`) which match the web page's globs.
- **No signing.** No `signs`, `binary_signs`, `notarize` or SBOM sections; checksums are the integrity story. The README explains Gatekeeper (`xattr -d com.apple.quarantine ./agent-cli` for manual downloads) and SmartScreen and recommends the package managers, which avoid both.

**Recovery.** `tools/release` removes the tag after a failed publish; if a run dies in between (runner lost), the maintainer deletes the tag and any partial release by hand and re-runs the workflow. This is documented in `AGENTS.md`.

### D15. Lint, formatting and hygiene

`.golangci.yml` (v2 schema): default `standard` linters plus `bodyclose`, `errorlint`, `forbidigo`, `gosec`, `misspell`, `noctx`, `unconvert`, `usestdlibvars`; formatters `gofumpt` and `goimports` with `github.com/appifyhub/agent-cli` as the local prefix. `forbidigo` forbids `fmt.Print*`, and direct use of `os.Stdout` or `os.Stderr` anywhere except `internal/term` and `cmd/abot`, so output can only leave through the terminal abstraction. `gosec` G304 (variable file paths) is excluded because the CLI deliberately opens files under the user's home, and subprocess findings are excluded in the shell-tool tests. A `Makefile` offers `build`, `test`, `lint`, `fmt` and `snapshot`; CI calls the Go tools directly.

### D16. Repository conventions (`AGENTS.md`, created in M0)

Go formatting is whatever `gofumpt` produces; comments are rare and start lowercase except for exported identifiers whose doc comment must begin with the name; no TODO comments; every text file ends with a newline; user-facing text only through the catalog; secrets never in logs or argv; offline tests with the D3 seams; one vertical milestone at a time with an owner stop after each, as in the OpenSpec workflow. The project-structure section mirrors the siblings: versions are bumped with `./tools/bump_version {major|minor|patch}` (major and minor reset lower segments), the CI/CD pipeline lives in `.github/workflows`, the build variants and the single version source are explained, and the release flow is described in `README.md`.

### D17. Milestones

| M | Content | Capabilities |
|---|---|---|
| M0 | Project setup, skeleton, strings catalog, terminal layer, test kit, lint, `VERSION` (embedded), build variants, bump, build and release scripts, GoReleaser configuration, verification, beta and production workflows, Dependabot, Claude workflow | `cli-foundation`, first part of `release-pipeline` and `distribution` |
| M1 | Local configuration and the `env` command | `local-config` |
| M2 | Credential storage | `credential-storage` |
| M3 | Backend client, error handling and the `server` command | `backend-client`, `server` |
| M4 | `connect` (also the shared time and credits rendering) | `connect` |
| M5 | `revoke-all` | `revoke-all` |
| M6 | `status` | `status` |
| M7 | `chats` | `chats` |
| M8 | `notify` | `notify` |
| M9 | `unlock` | `unlock` |
| M10 | Agent skill: `skill status`, `install <agent>`, `remove`, `show` | `agent-skill` |
| M11 | Packaging (Homebrew, Scoop, deb, rpm), README installation section, end-to-end acceptance | `distribution`, rest of `release-pipeline` |
| M12 | Owner-reviewed Go principles, independent section assessments, judge-filtered findings and bounded remediation | `go-quality-review` |
| M13 | Manual prerequisites, beta rehearsal, staging smoke test, first production release | acceptance of `distribution` and `release-pipeline` |

### D18. Conservative choices beyond the approved context

Each is small, reversible, and recorded here so the owner can veto it at the matching milestone stop:

1. The device id is created at the first successful-path `connect`, not on any first run, so read-only commands have no side effects.
2. At connect, `THE_AGENT_CLI_PASSPHRASE` counts as a passphrase source (no terminal needed); an explicit `--store` is never silently replaced.
3. Store selection, passphrase entry and a home-directory writability check happen before the single-use key is spent.
4. The 403 allowlist (`3001`, `3004`, `3007`, `3008`, `3009`, `3010`) and 404/`2002` keep the token; a 404/`2001` (merged account) counts as rejection.
5. A rejected `THE_AGENT_CLI_TOKEN` never deletes stored credentials; a successful revoke through it does.
6. `status` exits 3 for locked and expired states.
7. Declining the revoke confirmation exits 1; `--yes` is the only non-interactive path.
8. `notify` takes exactly one message argument, trims input, truncates stdin at 4096 characters with a warning and rejects longer arguments; any 2xx answer counts as sent.
9. `--version --verbose` also prints the commit.
10. `chats default --set`, `--platform` and `--clear` print confirmations and warns when an environment default overrides it; `chats default` and the platform and clear forms are offline.
11. Reconnecting as a different user clears the stored default.
12. No automatic retries; API URLs must be `https` except loopback.
13. `THE_AGENT_CLI_TOKEN` that is not token-shaped is exit 3 without a request.
14. The keychain account is the device id; the keychain is probed before the key is spent.
15. The date in the version line is the built commit's date; the version itself is the embedded `VERSION`.
16. Release tags are `v<VERSION>-beta` and `v<VERSION>`, the semantic-version forms of the backend's `.beta` and `.ga`; each is published once, so a release candidate needs a bumped `VERSION`; pull requests to `main` are verified but publish nothing; the web's label-based lock and verify workflows are not mirrored; the three pipelines share one reusable `verify.yml`.
20. The command name, the default home directory (`~/.<name>`) and the keychain service follow the build, so differently named builds never share state; the environment variables are `THE_AGENT_CLI_*` in every build.
21. The generic builds default to `http://localhost`, which the loopback rule of the API URL check accepts.
17. `credit_balance` is optional and ignored when unusable; the threshold is the named constant 100 credits; amounts are truncated to two decimals.
18. Windows console output is switched to UTF-8 and restored on exit; there is no ASCII fallback.
19. The TTL-too-long text keeps "2 years" in the editable catalog.

### D19. Agent skill (M10)

`agent-cli skill` takes inspiration from Rex's `rex skill`: install a thin skill that asks the CLI for its current guide. Our interface is `install <agent>`, `show`, `status` and `remove`; `install` replaces `setup`, without an alias. The explicitly selected agent need not be installed. `show` only prints the full guide to stdout and writes nothing.

- **Nine agents, native scopes.** Support `claude`, `codex`, `opencode`, `pi`, `omp`, `windsurf`, `cursor`, `gemini` and `antigravity`; exclude Devin. The location table in the agent-skill spec is authoritative. Claude uses its own skill directory, most harnesses support `.agents/skills`, and Antigravity uses `~/.gemini/config/skills` for user skills versus `.agents/skills` for project skills. Gemini's shared alias and Antigravity's locations were checked against [Gemini CLI skills](https://geminicli.com/docs/cli/skills/) and [Antigravity skills](https://antigravity.google/docs/skills/) on 2026-11-19; the other harness locations retain the earlier research. User and project relative paths need not match. Do not write duplicate copies for one installation.
- **Scope and selection.** `install [agent]` and `remove [agent]` require one positional supported name or explicit `--all`; `status` and `remove` retain optional repeatable `--agent` filtering. All three accept mutually exclusive `--user` and `--project`; user is the default. User scope uses the harness's user location, including Claude's configuration override. Project scope uses the harness's native project location under the exact cwd, ignores user overrides, never walks to a Git root, and works outside repositories. Multiple selected agents sharing a path operate on one file; help explains that removing it affects all harnesses reading that path.
- **The stub only points.** The body tells the agent to run `{name} skill show` and follow the guide, with `{name} --help` as the fallback. Every harness has a shell tool, so this works everywhere; the cost is one permission prompt in harnesses that ask before running commands, which is why `show` is offline, read-only and credential-free. Frontmatter holds only `name` (equal to the directory, which opencode and Cursor require), a short `description` (Claude Code cuts descriptions at 1,536 characters and the Agent Skills limit is 1,024; the stub stays under 300 and avoids `<` and `>`) and a `metadata` key as the ownership and version marker; unknown frontmatter keys are ignored by the harnesses that document it.
- **Ownership and states.** A file is ours when its frontmatter carries the `agent-cli-stub` marker. `current` means byte-equal to what this version would write, `outdated` means ours but different (older version or edited), `foreign` means a skill of that name without the marker, so it is never overwritten without `--force`.
- **No hooks, no instruction files.** Rex also installs session-start hooks because it must tell the agent which terminal it is in; this CLI has no per-session fact to inject, and hooks would add real costs: Codex skips a new hook until the user trusts it, Windows runs hook commands through PowerShell or Git Bash depending on the harness, and opencode, pi and omp can only hook through TypeScript code. The tool also never edits `AGENTS.md`, `CLAUDE.md` or rules files, which belong to the user and have size limits; for agents without skill support, `skill show --pointer` prints a block of at most 6 lines to paste. If a hook is wanted later, it is a separate change.
- **Name.** The skill directory and every command in the text use the build's command name (`Deps.Build.Name`), like the state directory, so `abot` and `agent-cli` can be installed together and removed independently. The name always matches `^[a-z0-9]+(-[a-z0-9]+)*$`.
- **No autodetection.** Installation targets the named harness regardless of binary or configuration-directory presence. Status reports skill file state at the selected scope's locations, not whether a harness is installed. No `LookPath` dependency or repository-root detection is needed.
- **The guide.** `internal/text/skill.md`, next to `strings.toml`, is the one editable guide shared by all agents. It is embedded as the catalog's `skill.guide` entry; only the build name and a shell-specific instruction fragment from `strings.toml` are substituted. The installed stub's wording is also in `strings.toml`. Rebuilding picks up text edits; no command output generation or copies per harness are needed. The guide stays under 200 lines, critical rules first. A test walks the operational command tree so omitted commands fail the build. Installation and removal details and numeric exit codes stay in command help; the guide gives only concise success and failure rules.
- **Layers.** `internal/agentskill` holds the agent table, user/project locations, stub states and atomic install and removal, imports no other package of the module (stub and guide text arrive from the catalog through `cli`), and writes nothing to standard streams. `cli.Deps` gains `Getwd` for exact-cwd project scope. Atomic writes use a temporary file in the target directory then rename, and are not shared with `config`, whose file needs mode 0600 and a header.

## Risks / Trade-offs

- **[macOS keychain access model]** Items created through `/usr/bin/security` are readable without a prompt by any same-user process while the keychain is unlocked. → Documented; `passphrase` mode exists for users who want more; never claim stronger.
- **[Unsigned binaries]** Manually downloaded archives get quarantined on macOS (Gatekeeper) and may trigger SmartScreen on Windows. → Package-manager installs avoid both; the README documents `xattr -d com.apple.quarantine` and the SmartScreen prompt; checksums are published.
- **[GoReleaser `brews` deprecated]** Works through v2, removal only in v3. → Migration trigger and path recorded in D14.
- **[Homebrew tap trust]** Short-name installs need `brew trust` on Homebrew 6 and later. → Docs and the web page use the fully qualified install command.
- **[Backend code numbers shift]** Numbers are allocated when the backend change is implemented. → Names are authoritative, constants are in one file, M3 re-reads the backend source, and the fake backend uses the same constants.
- **[Server message format coupling]** The `[E####]` prefix and `# Caused by` suffix are backend implementation details. → Cleaning is tolerant, and known failures use catalog text keyed by code, not server text.
- **[Environment token exposure]** `THE_AGENT_CLI_TOKEN` exported by `unlock` is inherited by child processes and visible to same-user tools, and `agent-cli unlock` without `eval` prints the token to the terminal scrollback. → Accepted by the approved design; hints push users to `eval`; documented.
- **[scrypt cost]** Work factor 18 needs roughly 256 MiB and a second per decrypt, and applies on every call when `THE_AGENT_CLI_PASSPHRASE` is used. → Acceptable for interactive use; `unlock` once per terminal is the recommended path.
- **[Raw-mode prompt edge cases]** Pasted escape sequences, input methods and unusual terminals can behave differently from line input. → ESC sequences and unknown control bytes are ignored, the terminal is always restored, unit tests cover the byte sequences, and real terminals are checked by hand on each OS.
- **[Windows specifics]** PowerShell 5.1 pipes non-ASCII text through the ANSI code page; `-race` is unavailable; the console code page change is visible to other programs in the same console until it is restored. → The `notify` help on Windows recommends Windows Terminal or PowerShell 7; the code page is restored on every exit path; tests cover the same logic without `-race`.
- **[pflag v1.1.0 with Cobra v1.10.2]** Cobra has not shipped against pflag 1.1.0. → The command tests exercise help, flags and completion on every OS; fallback to v1.0.10 is a one-line change.
- **[Offline-only tests]** A fake cannot prove the real backend agrees. → The contract table above came from the backend change; M13 includes a smoke test against staging with the beta binaries.
- **[Beta churn and partial releases]** A beta tag exists once per version, so a second push to a release pull request fails unless the version is bumped, and a failure after the release was created leaves a tag behind. → The failure message says to bump the version, pre-releases are clearly marked and deletable, the release is created only after verification and the build pass, and the recovery steps are documented.
- **[Tags pushed before publishing]** GoReleaser needs the tag to exist, so a crash between the push and the publish leaves a tag behind. → `tools/release` removes the tag and partial release when GoReleaser fails; the manual recovery for a lost runner is documented.
- **[Agent harness formats change]** Harness skill discovery and frontmatter support can change. → The spec records separate native user and project locations per supported agent; `install <agent>` updates the small owned stub, and task 11.8 verifies discovery in both scopes in the available real harnesses. Devin is not a supported target.
- **[Fourteen milestones]** More review stops than a typical slice. → That is the requirement; each stop is small and independently reviewable.

## Migration Plan

Greenfield; there is nothing to migrate. Order of work is M0 to M13 with a review stop after each. M0 can start right away because Go 1.27.4 is installed.

Everything automatable is built and tested before the last milestone, including the build and publishing scripts and the PR, beta and production workflows; M11 adds the packaging. M12 applies the owner-reviewed rules in `specs/go-quality-review/spec.md`: independent read-only assessors write numbered violation reports, a fresh judge retains only important evidence-backed improvements, and one fresh implementation agent fixes the accepted list. Good code stays unchanged, zero findings is valid, and temporary reports are deleted after verification. Owner approval of the principles is required before assessment, and approval of the completed review is required before final testing. The last milestone (M13) holds the work that needs GitHub-side setup and real machines:

1. Create the public repositories `appifyhub/homebrew-tap` and `appifyhub/scoop-bucket` with a default branch `main`.
2. Add the repository secrets `CI_TOKEN_ABOT` (a token with content write access to those two repositories) and `CLAUDE_CODE_OAUTH_TOKEN` to `appifyhub/agent-cli`.
3. Create the `release` branch from `main` and protect `main` and `release` like the sibling repositories (required checks from `verify.yml`).
4. Rehearse: open the release candidate pull request with `tools/create_pr`, let the beta pipeline publish `v0.1.0-beta`, check that tap and bucket are untouched, and run the staging smoke test with those binaries.
5. Merge the release candidate: the production pipeline creates `v0.1.0` and publishes the GitHub Release, the tap formula, the Scoop manifest and the packages; install each channel and run `abot --version`.

Rollback: delete the tag and release, revert the tap and bucket commits, and tell users to `brew uninstall`, `scoop uninstall` or remove the package; local state in `~/.<name>` is inert and can be deleted.
