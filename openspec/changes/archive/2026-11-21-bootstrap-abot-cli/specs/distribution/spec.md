# Spec Delta

## Purpose

Defines what users receive and where: the four builds of the tool, release artifacts, build-time values, the Homebrew tap, the Scoop bucket, Linux packages, the `@abot` alias, the absence of signing, and the per-OS documentation. How artifacts get built and published is defined by the release-pipeline capability.

The tool itself is generic (`agent-cli`, default backend `http://localhost`). `abot` and `abot-dev` are convenience builds of the same code with the backend URLs of the hosted service baked in, and only they are named after the hosted service.

## ADDED Requirements

### Requirement: Four builds in two releases
Each production release SHALL contain the `abot` build (backend `https://agent.cloud.appifyhub.com`) and the generic `agent-cli` build. Each beta release SHALL contain the `abot-dev` build (backend `https://staging.agent.cloud.appifyhub.com`) and the generic `agent-cli-dev` build. Both generic builds SHALL default to `http://localhost`. All four SHALL report the same plain version from `VERSION`.

#### Scenario: Production release contents
- **GIVEN** `VERSION` contains `0.1.0`
- **WHEN** the production release is published
- **THEN** it holds `abot_0.1.0_<os>_<arch>` and `agent-cli_0.1.0_<os>_<arch>` archives, and both binaries print `0.1.0` in their version line

#### Scenario: Beta release contents
- **GIVEN** `VERSION` contains `0.1.0`
- **WHEN** the beta release is published
- **THEN** it holds `abot-dev_0.1.0_<os>_<arch>` and `agent-cli-dev_0.1.0_<os>_<arch>` archives

### Requirement: Release artifacts for every desktop platform
Every release SHALL publish to GitHub Releases at `https://github.com/appifyhub/agent-cli/releases` statically linked binaries of both of its builds for darwin, linux and windows on amd64 and arm64, packaged as `<name>_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows), and a `checksums.txt` file with a SHA-256 for every archive and package.

#### Scenario: Archives and checksums
- **WHEN** the production release for version `0.1.0` is built
- **THEN** it contains `agent-cli_0.1.0_darwin_amd64.tar.gz`, `agent-cli_0.1.0_darwin_arm64.tar.gz`, `agent-cli_0.1.0_linux_amd64.tar.gz`, `agent-cli_0.1.0_linux_arm64.tar.gz`, `agent-cli_0.1.0_windows_amd64.zip`, `agent-cli_0.1.0_windows_arm64.zip` and the same six for `abot`
- **AND** `checksums.txt` lists a SHA-256 for each of them

#### Scenario: Archive contents
- **WHEN** an archive is unpacked
- **THEN** it holds the binary named after its build (`.exe` on Windows), the license and the README

### Requirement: Build-time values come from the pipeline
Release binaries SHALL carry the version from `VERSION`, the short commit hash, the UTC date of the released commit, the command name and default API URL of their build, and the product page URL `https://agent.appifyhub.com`.

#### Scenario: Version output of a production binary
- **GIVEN** `VERSION` contains `0.1.0` and the released commit is dated 2026-11-14
- **WHEN** the user runs the released `agent-cli --version` on darwin/arm64
- **THEN** stdout is `agent-cli 0.1.0 (darwin/arm64, 2026-11-14)`

#### Scenario: Hosted build
- **GIVEN** the released `abot` binary, `THE_AGENT_CLI_TOKEN` set, `THE_AGENT_CLI_API_URL` unset and no config file
- **WHEN** the user runs `abot status`
- **THEN** the `Backend:` line shows `https://agent.cloud.appifyhub.com`

### Requirement: Homebrew tap
Production releases SHALL publish the hosted build as the formula `abot` in the tap `appifyhub/tap` (repository `appifyhub/homebrew-tap`) for macOS and Linux on amd64 and arm64, installable with `brew install appifyhub/tap/abot`. The formula SHALL also install the `@abot` alias.

#### Scenario: Install from the tap
- **WHEN** the user runs `brew install appifyhub/tap/abot` on a supported machine
- **THEN** `abot --version` prints the released version
- **AND** `@abot` is a symlink to `abot` in the same directory

#### Scenario: Formula updated by the release
- **WHEN** a production release is published
- **THEN** the formula in the tap points at the new archives and checksums

### Requirement: Scoop bucket
Production releases SHALL publish the hosted build as the manifest `abot` in the bucket `appifyhub/scoop-bucket`, installable on Windows with `scoop bucket add appifyhub https://github.com/appifyhub/scoop-bucket` followed by `scoop install abot`. No `@abot` alias SHALL be installed on Windows.

#### Scenario: Install from the bucket
- **WHEN** the user runs the two Scoop commands on windows/amd64 or windows/arm64
- **THEN** `abot --version` prints the released version

#### Scenario: No alias on Windows
- **WHEN** the Scoop package is installed
- **THEN** only `abot.exe` is added to the path

### Requirement: Debian and RPM packages
Production releases SHALL attach Linux packages of the hosted build named `abot_<version>_<arch>.deb` and `abot-<version>-1.<arch>.rpm` for amd64 and arm64. Each SHALL install `/usr/bin/abot` and a symlink `/usr/bin/@abot` to it, carry the package name `abot`, and list the license and the product page as homepage.

#### Scenario: Package files
- **WHEN** a production release for version `0.1.0` is built
- **THEN** it contains `abot_0.1.0_amd64.deb`, `abot_0.1.0_arm64.deb`, `abot-0.1.0-1.x86_64.rpm` and `abot-0.1.0-1.aarch64.rpm`

#### Scenario: Installing a package
- **WHEN** the user runs `sudo apt install ./abot_0.1.0_amd64.deb`
- **THEN** `/usr/bin/abot` exists and `/usr/bin/@abot` is a symlink to `abot`

### Requirement: The @abot alias is Unix-only and documented
The alias `@abot` SHALL be installed only by the Homebrew formula and the Linux packages. The documentation SHALL state that PowerShell cannot use `@abot` because `@` starts argument splatting there, and that shell completion is registered for `abot` only.

#### Scenario: Documentation of the alias
- **WHEN** the README is read
- **THEN** it explains `@abot` for macOS and Linux and states that on Windows PowerShell only `abot` works

### Requirement: Artifacts are unsigned
Release artifacts SHALL be published without any signature: no Apple code signing or notarization, no Windows Authenticode signature and no cosign signatures. Integrity SHALL rest on `checksums.txt`, and the README SHALL explain the macOS Gatekeeper and Windows SmartScreen prompts for manually downloaded binaries and recommend the package managers.

#### Scenario: Release contents
- **WHEN** a release is published
- **THEN** it contains `checksums.txt` and no signature or certificate files

#### Scenario: Gatekeeper guidance
- **WHEN** the README is read by a macOS user who downloaded an archive manually
- **THEN** it shows how to remove the quarantine attribute with `xattr -d com.apple.quarantine ./agent-cli` and recommends Homebrew for the hosted build

#### Scenario: SmartScreen guidance
- **WHEN** the README is read by a Windows user who downloaded an archive manually
- **THEN** it explains the SmartScreen warning and recommends Scoop for the hosted build

### Requirement: The CLI documents itself
How to use the tool SHALL be documented by the tool's own help (`help`, `help <command>`, `<command> --help`), never duplicated in the README, so the documentation always matches the installed version. The README SHALL cover only what is needed before the binary runs (what the project is, installation per OS, the unsigned-binary prompts, the `@abot` alias note) and how to develop and release the project, describing the finished tool without project status or planning notes.

#### Scenario: Usage lives in the help
- **WHEN** a user looks for how to connect, unlock, notify, the storage modes, the environment variables or the exit codes
- **THEN** the README points to `agent-cli help` and the information is in the help output, not in the README

#### Scenario: Windows reader
- **WHEN** a Windows user follows the README installation section
- **THEN** every command shown works in PowerShell without POSIX-only syntax

#### Scenario: Install commands match the web page
- **WHEN** the README install commands for the hosted build are compared with the web Command Line page
- **THEN** they use the same tap, bucket and package names

#### Scenario: Contributor flow
- **WHEN** a maintainer reads the README or `AGENTS.md`
- **THEN** both explain bumping with `tools/bump_version`, opening the release candidate with `tools/create_pr`, and what the beta and production pipelines do
