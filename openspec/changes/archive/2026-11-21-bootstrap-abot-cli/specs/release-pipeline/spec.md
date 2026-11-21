# Spec Delta

## Purpose

Defines how changes reach users: the version file and bump script, the build variants and the local build script, verification of pull requests, beta releases from pull requests to `release`, production releases from pushes to `release`, release tags, dependency automation and the Claude workflow. The flow mirrors the agent backend's pipelines (`qa`, `beta`, `release`); like the web app's pipelines, it calls no release-summary service.

## ADDED Requirements

### Requirement: Single version file
The repository SHALL keep the release version in a root file `VERSION` that holds exactly `MAJOR.MINOR.PATCH` and a trailing newline. The binary SHALL embed that file, and the build script, the release tags and the release names SHALL read it; the version SHALL be written nowhere else.

#### Scenario: Initial version
- **WHEN** the repository is first released
- **THEN** `VERSION` contains `0.1.0`

#### Scenario: Version comes from the file
- **GIVEN** `VERSION` contains `0.2.0`
- **WHEN** the production pipeline runs
- **THEN** the release tag is `v0.2.0` and every released binary reports `0.2.0`

#### Scenario: Malformed version file
- **GIVEN** `VERSION` contains `0.2`
- **WHEN** a build or a release runs
- **THEN** it fails before building or publishing anything and the message names the file

### Requirement: Build variants and the build script
The repository SHALL define the four builds in one place: `agent-cli` and `agent-cli-dev` (generic, `http://localhost`), `abot-dev` (`https://staging.agent.cloud.appifyhub.com`) and `abot` (`https://agent.cloud.appifyhub.com`). `tools/build <name>` SHALL build one of them for the current system into `bin/`, deriving the commit and the build date from the built commit, with `THE_AGENT_CLI_BUILD_API_URL` and `THE_AGENT_CLI_BUILD_PRODUCT_URL` overriding the URLs of that local build, and SHALL fail for an unknown name.

#### Scenario: Each build
- **WHEN** the script builds `agent-cli`, `agent-cli-dev`, `abot-dev` and `abot`
- **THEN** each binary is named after its build, prints that name in `--version`, and carries its own backend URL and no other hosted URL

#### Scenario: URL from the environment
- **GIVEN** `THE_AGENT_CLI_BUILD_API_URL=https://example.test`
- **WHEN** the script builds any variant
- **THEN** that binary uses `https://example.test` as its built-in backend

#### Scenario: Unknown variant
- **WHEN** the script runs with `staging`
- **THEN** it prints the usage and exits with code 1

### Requirement: Pull requests are verified
Every pull request to `main` or `release` SHALL run the linters, the newline check, the GoReleaser configuration check and a dependency vulnerability scan on Linux, and the full offline test suite on Linux, macOS and Windows. Pull requests to `main` SHALL publish nothing.

#### Scenario: Passing pull request
- **WHEN** a pull request with working code is opened against `main`
- **THEN** lint, newline check, configuration check, vulnerability scan and tests on all three systems succeed, and no tag or release is created

#### Scenario: Failure on one system
- **GIVEN** a test fails only on Windows
- **WHEN** the verification runs
- **THEN** the run fails and names the Windows job

#### Scenario: Dependabot pull requests are verified too
- **WHEN** Dependabot opens a pull request against `main`
- **THEN** the same verification runs

### Requirement: Pull requests to release produce a beta
For each pull request to `release` that is not opened by Dependabot, the pipeline SHALL run the verification and then tag the pull request head `v<VERSION>-beta`, publish a GitHub pre-release with the `abot-dev` and `agent-cli-dev` archives and `checksums.txt`, and comment on the pull request with a link. A beta SHALL be published once per version.

#### Scenario: Beta published
- **GIVEN** `VERSION` contains `0.1.0` and no tag `v0.1.0-beta` exists
- **WHEN** a pull request to `release` is opened
- **THEN** the pre-release `v0.1.0-beta` exists with twelve archives and `checksums.txt`
- **AND** the released `abot-dev` binary prints `abot-dev 0.1.0 (<os>/<arch>, <date>)` for `--version`

#### Scenario: Staging by default
- **GIVEN** a beta `abot-dev` binary, `THE_AGENT_CLI_TOKEN` set, `THE_AGENT_CLI_API_URL` unset and no config file
- **WHEN** the user runs `abot-dev status`
- **THEN** the `Backend:` line shows `https://staging.agent.cloud.appifyhub.com`

#### Scenario: Pull request comment
- **WHEN** a beta is published
- **THEN** the pull request gets a comment that links to the pre-release

#### Scenario: Dependabot
- **WHEN** a pull request to `release` is opened by Dependabot
- **THEN** no beta is built

### Requirement: Pushes to release publish production
A push to `release` SHALL run the verification, then tag the pushed commit `v<VERSION>` and publish the GitHub release with the `abot` and `agent-cli` archives and `checksums.txt` as the latest release, with notes that list the commits since the previous production tag.

#### Scenario: First production release
- **GIVEN** `VERSION` contains `0.1.0` and no tag `v0.1.0` exists
- **WHEN** a commit is pushed to `release`
- **THEN** the tag `v0.1.0` exists on that commit and the release `The Agent CLI 0.1.0` has twelve archives and `checksums.txt`

#### Scenario: Released binaries
- **WHEN** the user runs the released `agent-cli --version` and `abot --version` on darwin/arm64
- **THEN** stdout is `agent-cli 0.1.0 (darwin/arm64, <commit date>)` and `abot 0.1.0 (darwin/arm64, <commit date>)`

#### Scenario: Release notes
- **WHEN** a release is published
- **THEN** its notes list the commits since the previous tag of the same kind (`v<x.y.z>` for production, `v<x.y.z>-beta` for betas), without version-bump and merge-request commits

### Requirement: A version is released once
When the tag of a beta or production release already exists locally or on `origin`, the release SHALL fail before tagging or building and SHALL say that the version must be bumped. When publishing fails after the tag was pushed, the tag and any partial release SHALL be removed so the run can be retried.

#### Scenario: Version was not bumped
- **GIVEN** the tag `v0.1.0` exists and `VERSION` still contains `0.1.0`
- **WHEN** a commit is pushed to `release`
- **THEN** the run fails with a message that tells the maintainer to bump the version
- **AND** no release is created

#### Scenario: Publishing fails
- **GIVEN** the tag `v0.1.0` was pushed and GoReleaser fails
- **WHEN** the release step ends
- **THEN** the tag no longer exists locally or on `origin`

### Requirement: Dependency updates are automated
Dependabot SHALL open grouped weekly pull requests to `main` for the Go modules and for the GitHub Actions used by the workflows. For such a pull request a workflow SHALL tidy the module files and commit the result if it changed, approve the pull request and enable auto-merge, which completes only after the required checks pass.

#### Scenario: Module files change on tidy
- **GIVEN** a Dependabot pull request whose `go.sum` is out of date
- **WHEN** the auto-merge workflow runs
- **THEN** it commits the tidied `go.mod` and `go.sum` as `Auto-lock: Update dependencies` and enables auto-merge

#### Scenario: Nothing to tidy
- **GIVEN** a Dependabot pull request with consistent module files
- **WHEN** the auto-merge workflow runs
- **THEN** no commit is added and auto-merge is enabled

#### Scenario: Pull requests from people
- **WHEN** a person opens a pull request against `main`
- **THEN** the auto-merge workflow does nothing

### Requirement: Release candidate pull request helper
`tools/create_pr` run on `main` SHALL open a pull request from `main` to `release` titled `Release Candidate v<VERSION>` with the label `Release`. On any other branch it SHALL print the link for opening a pull request against `main`, and on `release` it SHALL refuse.

#### Scenario: On main
- **GIVEN** `VERSION` contains `0.2.0` and the GitHub CLI is available
- **WHEN** the user runs `tools/create_pr` on `main`
- **THEN** a pull request `Release Candidate v0.2.0` from `main` to `release` with the label `Release` is created

#### Scenario: On a feature branch
- **WHEN** the user runs `tools/create_pr` on branch `fix-typo`
- **THEN** the last line of the output is the compare link `https://github.com/appifyhub/agent-cli/compare/main...fix-typo?quick_pull=1`

#### Scenario: On release
- **WHEN** the user runs `tools/create_pr` on `release`
- **THEN** it fails because the release branch can only be a pull request destination

### Requirement: Claude workflow
Issue comments, pull request comments, reviews and issue text that mention `@claude` SHALL run the Claude Code workflow, configured exactly like in the other agent repositories.

#### Scenario: Mention
- **WHEN** a pull request comment contains `@claude please review`
- **THEN** the Claude Code job runs

#### Scenario: No mention
- **WHEN** a pull request comment does not contain `@claude`
- **THEN** the job does not run
