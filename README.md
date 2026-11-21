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

The CLI connects a machine to your profile and lets scripts and terminals interact with your agent, which is handy for long builds, cron jobs and anything else that should tap you on the shoulder or talk to you. It talks to the [backend](https://github.com/appifyhub/agent-backend)'s API.

The tool is generic: out of the box it talks to a backend at `http://localhost`, and it can be pointed at any other instance, such as your own deployment of the backend. For the hosted service, every release also ships `abot`, the same program with the hosted service's address built in.

See the rest of this document for installation and a developer's overview.

## Before you continue…

If you plan on contributing to this project in any way, please read and acknowledge the [Contributing guide](./CONTRIBUTING.md) first.

Please also take note of the [License](./LICENSE).

## Using the CLI

### Installation

#### Generic CLI

Download the `agent-cli_<version>_<os>_<arch>` archive for your machine from the [Releases page](https://github.com/appifyhub/agent-cli/releases). Archives cover macOS, Linux and Windows on amd64 and arm64 (`.tar.gz` on macOS and Linux, `.zip` on Windows). Unpack the archive and put the binary on your `PATH`. macOS 13 or later is required.

Release artifacts are unsigned: there is no Apple code signing or notarization, Windows Authenticode signature, or separate signature file. Verify downloads against the SHA-256 sums in `checksums.txt`.

On macOS, a manually downloaded binary can be quarantined by Gatekeeper; after verifying the download, clear the flag with `xattr -d com.apple.quarantine ./agent-cli`. On Windows, SmartScreen may warn on first launch; use its **More info → Run anyway** option only after verifying the download and its source.

#### Hosted service

The optional `abot` build is the same CLI with the hosted service's address built in. Production releases provide matching archives and `.deb` and `.rpm` packages for Linux on amd64 and arm64. Pre-releases contain only `agent-cli-dev` and `abot-dev` archives, with the latter pointing at staging.

For the hosted build, prefer Homebrew on macOS and Linux or Scoop on Windows to avoid the manual-download prompts:

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

The CLI documents itself, so the help always matches the installed version: `agent-cli help` lists the commands and exit codes, and `agent-cli help <command>` shows the usage, flags and examples of one command. For the hosted build, use `abot help` instead.

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

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
brew install goreleaser    # only needed for make snapshot
export PATH="$HOME/go/bin:$PATH"    # where go install puts tools
```

#### Building

```bash
make build                 # bin/agent-cli for this machine
make build-all             # all four builds for this machine, into bin/
./tools/build abot-dev     # one build, into bin/
make snapshot              # cross-platform archives and Linux packages, into dist/
```

`THE_AGENT_CLI_BUILD_API_URL` and `THE_AGENT_CLI_BUILD_PRODUCT_URL` override the built-in URLs of a local build:

```bash
THE_AGENT_CLI_BUILD_API_URL=https://agent.example.com ./tools/build agent-cli
```

#### Testing and linting

All tests run offline and on macOS, Linux and Windows:

```bash
make test    # go test -race -count=1 ./...
make e2e     # builds every variant and runs the e2e tests against the binaries
make lint    # golangci-lint and the final-newline check
make fmt     # gofumpt and goimports
```

Help output is pinned by golden files in `testdata/`; after an intentional change, rewrite them with `UPDATE_GOLDEN=1 go test ./...` and review the diff.

### Versioning and releases

The version is the single line in `VERSION`; the binaries embed it and the release tooling reads it.

1. Maintainers commit and push changes directly to `main`. Pull requests from contributors are verified without publishing anything.
1. On `main`, run `./tools/bump_version {major|minor|patch}`, commit the change as `Bump version`, and push it.
1. Run `./tools/create_pr` on `main` to open `Release Candidate v<VERSION>` from `main` to `release`, labeled `Release`. After verification, it publishes the `v<VERSION>-beta` pre-release: `agent-cli-dev` and `abot-dev` archives and checksums only, with no packages or tap/bucket updates.
1. Merge the release candidate with a **merge commit**, not squash or rebase. The push to `release` verifies and publishes `v<VERSION>`: `agent-cli` and `abot` archives, checksums and hosted Linux packages, then updates the Homebrew formula and Scoop manifest.

[`tools/release`](./tools/release) drives GoReleaser; [`.goreleaser.yaml`](./.goreleaser.yaml) defines the archives, Linux packages, formula in [`appifyhub/homebrew-tap`](https://github.com/appifyhub/homebrew-tap) and manifest in [`appifyhub/scoop-bucket`](https://github.com/appifyhub/scoop-bucket). Release publishing uses `GITHUB_TOKEN` for this repository's tags, releases and PR comments; only the cross-repository tap and bucket publishers use `CI_TOKEN_ABOT`, passed to the production job only.

Each version is released once, so bump `VERSION` before opening the next release candidate.

### License

Check out the license [here](LICENSE).

---

For the service itself, see the [backend repository](https://github.com/appifyhub/agent-backend); for the web app, see the [web app repository](https://github.com/appifyhub/agent-web).
