![License](https://img.shields.io/github/license/appifyhub/agent-cli?logo=github&logoColor=white&label=License&color=FA3080)
![Date](https://img.shields.io/github/release-date/appifyhub/agent-cli?display_date=published_at&logo=github&logoColor=white&label=Released&color=FA3080)
![Release](https://img.shields.io/github/v/release/appifyhub/agent-cli?sort=semver&display_name=release&logo=github&logoColor=white&label=Latest&color=FA3080)
![Code](https://img.shields.io/github/repo-size/appifyhub/agent-cli?logo=github&logoColor=white&label=Sources&color=FAFA20)
![Build](https://img.shields.io/github/actions/workflow/status/appifyhub/agent-cli/release.yml?branch=release&logo=github&logoColor=white&label=Build)
![Issues](https://img.shields.io/github/issues-closed/appifyhub/agent-cli?logo=github&logoColor=white&label=Issues&color=blue)
![PRs](https://img.shields.io/github/issues-pr-closed/appifyhub/agent-cli?logo=github&logoColor=white&label=PRs&color=blue)

# The Agent · Command Line Interface

## About the project

This repository contains the **complete** codebase of The Agent's command-line client, `agent-cli`.

The CLI connects a machine to your profile and lets scripts and terminals interact with your agent, which is handy for long builds, cron jobs and anything else that should tap you on the shoulder or talk to you. It talks to the [backend](https://github.com/appifyhub/agent-backend)'s CLI endpoints and nothing else.

The tool is generic: out of the box it talks to a backend at `http://localhost`, and `THE_AGENT_CLI_API_URL` points it anywhere else, such as your own deployment of the backend. For the hosted service, every release also ships `abot`, the same program with the hosted service's address built in.

See the rest of this document for installation and a developer's overview.

## Before you continue…

If you plan on contributing to this project in any way, please read and acknowledge the [Contributing guide](./CONTRIBUTING.md) first.

Please also take note of the [License](./LICENSE).

## Using the CLI

### Installation

Every release on the [Releases page](https://github.com/appifyhub/agent-cli/releases) has archives for macOS, Linux and Windows on amd64 and arm64, plus a `checksums.txt` with SHA-256 sums:

| Archive | What it is |
| --- | --- |
| `agent-cli_<version>_<os>_<arch>` | The generic CLI, for any backend instance |
| `abot_<version>_<os>_<arch>` | The same CLI, preconfigured for the hosted service |

Unpack the archive and put the binary on your `PATH`. On macOS, a manually downloaded binary can be quarantined by Gatekeeper; clear the flag with `xattr -d com.apple.quarantine ./agent-cli`. MacOS 13 or later is required. On Windows, SmartScreen may warn on first launch.

The hosted build is also available from package managers, which avoid both prompts:

```bash
# macOS and Linux (Homebrew)
brew install appifyhub/tap/abot

# Debian and Ubuntu, or Fedora and RHEL, with a package from the Releases page
sudo apt install ./abot_*.deb
sudo dnf install ./abot-*.rpm
```

```powershell
# Windows (Scoop)
scoop bucket add appifyhub https://github.com/appifyhub/scoop-bucket
scoop install abot
```

On macOS and Linux, Homebrew and the Linux packages also install the alias `@abot`. PowerShell cannot use it, because `@` starts argument splatting there, and shell completion is registered for `abot` only.

### Usage

```bash
# connect this machine once, with the connect key from your agent
agent-cli connect <KEY>

# send yourself a notification, for example when a long job ends
./backup.sh && agent-cli notify "Backup finished"

# or pipe the message in, for example the last line of a script's output
./backup.sh | tail -n 1 | agent-cli notify

# see which environment variables the CLI reads and what they are set to
agent-cli env
```

`abot` is the same CLI with the hosted service's address built in, so every command works the same way and only the name changes. On macOS and Linux, `@abot` is an alias for it:

```bash
# the generic CLI, pointed at the hosted service by hand
THE_AGENT_CLI_API_URL=https://agent.cloud.appifyhub.com agent-cli notify "Backup finished"

# the same thing with the hosted build, which already knows the address
abot notify "Backup finished"
@abot notify "Backup finished"
```

Each build keeps its own state (`~/.agent-cli`, `~/.abot`), so connect the one you use.

The CLI documents itself, so the help always matches the installed version: `agent-cli help` lists the commands and exit codes, and `agent-cli help <command>` shows the usage, flags and examples of one command.

## Developer's Overview

Because the complete codebase is open-source, you can inspect, build and run the CLI yourself.

### Tech Stack

The project currently uses the following tech stack:

- Runtime: **Native binary** (no cgo, one per OS and architecture)
- Language: **Go**
- Framework: **Cobra** (commands, help and shell completion)
- Persistence: **TOML** file and the OS credential store
- Build System: **Go toolchain**, **GoReleaser** & **Custom** (see `tools` and `.github` directories)
- Continuous Integration: **GitHub Actions**
- Distribution: **GitHub Releases**, **Homebrew**, **Scoop**, `.deb` and `.rpm` packages

### Builds

One codebase produces four builds that differ only in their command name and built-in backend. `tools/variants` is the only place that defines them:

| Build | Built-in backend | Released with |
| --- | --- | --- |
| `agent-cli` | `http://localhost` | every release |
| `abot` | `https://agent.cloud.appifyhub.com` | every release |
| `agent-cli-dev` | `http://localhost` | every pre-release |
| `abot-dev` | `https://staging.agent.cloud.appifyhub.com` | every pre-release |

Each build keeps its own state in `~/.<command name>`, so they can be installed side by side.

### How to build and run?

#### Dependencies

- Go, in the version from the `go` directive in `go.mod`
- `make` and a POSIX shell
- `golangci-lint` and `goreleaser`, in the versions pinned in `.github/workflows/verify.yml`

#### Building

```bash
make build                 # bin/agent-cli for this machine
make build-all             # all four builds for this machine, into bin/
./tools/build abot-dev     # one build, into bin/
make snapshot              # release archives for every platform, into dist/
```

`THE_AGENT_CLI_BUILD_API_URL` and `THE_AGENT_CLI_BUILD_PRODUCT_URL` override the built-in URLs of a local build:

```bash
THE_AGENT_CLI_BUILD_API_URL=https://agent.example.com ./tools/build agent-cli
```

#### Testing and linting

All tests run offline and on macOS, Linux and Windows:

```bash
make test    # go test -race -count=1 ./...
make lint    # golangci-lint and the final-newline check
make fmt     # gofumpt and goimports
```

Help output is pinned by golden files in `testdata/`; after an intentional change, rewrite them with `UPDATE_GOLDEN=1 go test ./...` and review the diff.

### Versioning and releases

The version is the single line in `VERSION`; the binaries embed it and the release tooling reads it. Bump it with `./tools/bump_version {major|minor|patch}`.

1. Pull requests to `main` are verified: tests on three systems, lint, configuration and vulnerability checks.
1. `./tools/create_pr` on `main` opens the release candidate pull request to `release`; it publishes the `v<version>-beta` pre-release with `agent-cli-dev` and `abot-dev`.
1. Merging it publishes the `v<version>` release with `agent-cli` and `abot`.

Each version is released once, so bump `VERSION` before opening the next release candidate.

### License

Check out the license [here](LICENSE).

---

For the service itself, see the [backend repository](https://github.com/appifyhub/agent-backend); for the web app, see the [web app repository](https://github.com/appifyhub/agent-web).
