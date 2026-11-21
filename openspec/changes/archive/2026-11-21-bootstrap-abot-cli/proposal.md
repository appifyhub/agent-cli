# Proposal

## Why

The agent can already reach users on Telegram and WhatsApp, but nothing a user runs on their own computer can reach the agent: scripts, long builds, deploy jobs and cron tasks have no way to say "tell me when this finishes" without opening a chat by hand. A command-line client closes that gap, and it is also the first non-chat platform client, so its foundations (identity, token storage, backend client, strings, packaging, release pipeline) should be right before a chat TUI is added later.

`agent-cli` is an empty repository with only OpenSpec scaffolding. This change plans the whole of v1 (authentication plus notifications) as one reviewable sequence of vertical milestones, each fully tested offline and ending with an owner review stop. The release pipeline mirrors the branch-based pipelines of `agent-backend` and `agent-web`.

## What Changes

- Create the generic Go command-line client `agent-cli` (module `github.com/appifyhub/agent-cli`, latest stable Go, Cobra) with these commands: `connect`, `revoke-all`, `status`, `server` (the backend's health and version), `chats` (list and `default` with `--set`, `--platform`, `--clear`), `notify`, `unlock`, `env` (the environment variables the CLI reads and their values), `skill` (teach AI coding agents to use the CLI), plus the built-in `completion` and `help`, and the global flags `-v/--verbose`, `-h/--help`, `--version`.
- Define the shell of the tool: help that explains what the agent is and links the product page, per-OS instructions chosen at run time, one embedded editable strings file for all user-facing text, build-time version/commit/date/API URL/product URL variables, a fixed exit-code contract (0 to 6), stdout/stderr and color rules, `*`-masked passphrase entry, UTF-8 console output on Windows, and untrusted-text sanitization.
- Define local state under `~/.<name>` (`~/.agent-cli` by default; override `THE_AGENT_CLI_HOME`): a secret-free `config.toml` (device id, API URL, store mode, token metadata, default target) and an optional `credentials` file; plus the environment variables `THE_AGENT_CLI_TOKEN`, `THE_AGENT_CLI_DEFAULT_CHAT`, `THE_AGENT_CLI_DEFAULT_PLATFORM`, `THE_AGENT_CLI_PASSPHRASE`, `THE_AGENT_CLI_API_URL`, `THE_AGENT_CLI_HOME` and `NO_COLOR`.
- Define credential storage in three modes (`keychain`, `passphrase`, `unsafe`), automatic selection with safe fallbacks, passphrase unlocking (prompt, `THE_AGENT_CLI_PASSPHRASE`, or `eval "$(agent-cli unlock)"` once per terminal), and clean local removal on revoke or rejection.
- Define the backend client for the five CLI endpoints of the backend change `add-cli-access`: error decoding, the mapping of every outcome to a message and exit code (including the account gates for policies and the waitlist), and a low-credit warning on stderr after `connect` and `notify` when the reported balance is under 100 credits.
- Define release engineering that mirrors the existing repositories: a `VERSION` file that is the single version source (embedded in the binary, read by the pipelines) and is bumped by `tools/bump_version`; four builds of one codebase defined in one place (the generic `agent-cli` and `agent-cli-dev` defaulting to `http://localhost`, and the hosted-service conveniences `abot` and `abot-dev` with the production and staging URLs baked in); the backend's three workflows mirrored without its release-summary step: pull requests to `main` are verified only, pull requests to `release` publish the `vX.Y.Z-beta` pre-release with `abot-dev` and `agent-cli-dev`, a push to `release` publishes the `vX.Y.Z` release with `abot` and `agent-cli`, all through `tools/release` and GoReleaser; M11 adds the package managers (Homebrew tap `appifyhub/tap`, Scoop bucket `appifyhub/scoop-bucket`, `.deb`/`.rpm`) for the hosted `abot` build only; Dependabot for Go modules and Actions with the same auto-merge workflow, and the Claude workflow.
- Ship unsigned: no Apple code signing or notarization, no Windows Authenticode, no cosign; `checksums.txt` is the integrity story and the README explains the OS prompts.
- Deliver as milestones M0 to M13: setup, config, credentials, backend client, one per command, then the agent skill (M10), packaging, the README installation section and end-to-end acceptance (M11), an owner-reviewed Go quality assessment with independent assessors, a judge and one remediation agent (M12), and finally the manual prerequisites, the beta rehearsal, the staging smoke test and the first production release (M13). Every milestone ends with a STOP for user review before the next one starts; M12 also requires approval of its principles before assessment begins.
- No breaking changes: this is a greenfield repository.

Explicitly out of scope for v1: chatting, a TUI, streaming responses, a device table or per-device revoke, telemetry, self-update, any kind of signing, an apt/yum repository, winget, and first-class multiple backend profiles (`THE_AGENT_CLI_HOME` plus `THE_AGENT_CLI_API_URL` already isolate setups).

## Capabilities

### New Capabilities

- `cli-foundation`: The observable shell of the tool: help and version output, global flags, per-OS instructions, text catalog, build-time identity, exit codes, output streams, color, masked secret prompts, Windows console encoding, text sanitization, shell completion, side-effect-free informational commands.
- `local-config`: Where non-secret local state lives, the config file contents, device identity, the effective API URL and its safety rules, default target resolution, and safe config updates.
- `credential-storage`: The three token storage modes, default selection, passphrase protection and unlocking, `THE_AGENT_CLI_TOKEN` precedence, locked behavior, local token removal, and the guarantee that secrets never leak.
- `backend-client`: Request and response conventions for the five CLI endpoints, error body handling, and the classification of every error outcome (including account gates) into a message, exit code and local-token action.
- `connect`: `agent-cli connect`, which links this machine to the user's profile with a connect key, stores the issued token and warns about low credit.
- `revoke-all`: `agent-cli revoke-all`, which revokes every CLI token of the user and removes the local one.
- `status`: `agent-cli status`, the offline report of this machine's connection.
- `server`: `agent-cli server`, which calls the backend's health endpoint and prints its status, version and any other metadata it reports; no connection to a profile needed.
- `chats`: `agent-cli chats`, `chats default` with `--set`, `--platform` and `--clear`: listing notifiable chats and managing the default target.
- `notify`: `agent-cli notify`, sending a notification to the user's chats with target selection, stdin input, a low-credit warning and distinguishable failures.
- `unlock`: `agent-cli unlock`, exporting a passphrase-protected token into one terminal session.
- `agent-skill`: `agent-cli skill` (`status`, `install <agent>`, `remove`, `show`): a short skill that teaches Claude Code, Codex, opencode, pi, omp, Windsurf, Cursor, Gemini CLI and Antigravity to use the CLI by pointing at a guide it prints itself. Installation explicitly selects an agent and defaults to `--user`; `--project` uses that harness's native project path under the exact cwd, without Git-root lookup. `show` prints the guide to stdout without writing files; status and removal remain available.
- `go-quality-review`: Sourced Go and CLI engineering principles, reviewed by the owner before a late assessment; independent section assessments, high-value finding adjudication, bounded remediation and removal of temporary violation reports before final testing.
- `distribution`: What users receive: the generic build and the hosted builds, release artifacts, build-time values, the Homebrew tap, the Scoop bucket, `.deb`/`.rpm`, the `@abot` alias, unsigned artifacts, and the per-OS documentation.
- `release-pipeline`: How changes reach users: the version file and bump script, the build variants and build script, verification of pull requests, beta releases from pull requests to `release`, production releases from pushes to `release`, semantic-version tags, Dependabot automation and the Claude workflow.

### Modified Capabilities

None. `openspec/specs/` is empty in this repository.

## Impact

- **New code and files** in `agent-cli`: `cmd/agent-cli`, `internal/*` packages, `go.mod`/`go.sum`, `VERSION`, `tools/` (shell scripts), `.golangci.yml`, `.goreleaser.yaml`, `Makefile`, `.github/` workflows and Dependabot config, `README.md`, `AGENTS.md`, `GEMINI.md`, `.claude/CLAUDE.md`, `LICENSE`, `SECURITY.md`, `CONTRIBUTING.md`.
- **Dependencies** (all latest stable at planning time, exact versions in `design.md`): Go 1.27.4 (already installed on the development machine), Cobra, go-toml/v2, go-keyring, age, x/term, x/sys; tooling golangci-lint, GoReleaser and govulncheck.
- **Backend dependency:** end-to-end use needs the backend change `add-cli-access` (endpoints under `/cli/*`, the error codes it allocates, `credit_balance` in the connect and notify responses). The CLI is fully buildable and testable without it because every test runs against an in-process fake of the contract.
- **Web dependency:** the web change `add-command-line-page` shows install commands that this change publishes (`brew install appifyhub/tap/abot`, the Scoop bucket, `abot_*.deb`, `abot-*.rpm`) and the `abot connect XXXX-XXXX-XXXX` command of the hosted build.
- **Manual prerequisites outside this repository** (the last milestone, listed as owner TODOs at the end of `tasks.md`): the tap repository `appifyhub/homebrew-tap`, the bucket repository `appifyhub/scoop-bucket`, the `release` branch, and the repository secrets `CI_TOKEN_ABOT` and `CLAUDE_CODE_OAUTH_TOKEN`.
- **Platform floor:** Go 1.27 binaries require macOS 13 or later.
