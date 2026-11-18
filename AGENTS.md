## Code Style

### Go

- Format with `gofumpt` and group imports with `goimports` (local prefix `github.com/appifyhub/agent-cli`); `make fmt` does both. Everything else follows the language's best practices and the surrounding code.
- Keep the layering strict: `cli` → `session` → `credentials`, `api`, `config`, `token`. `term` imports no domain package, `api` imports only `buildinfo`, nothing below `cli` imports Cobra or writes to stdout or stderr, and `testkit` is imported only from tests. `internal/arch_test.go` enforces it.
- The tool is generic. Use `agent-cli` or the `{name}` placeholder in code, catalog, tests and docs; the name `abot` belongs only where the hosted builds are defined or distributed (`tools/variants`, the release configuration, the README's installation section).
- Do not change working code without a concrete need. Avoid single-use abstractions, unnecessary files, and helpers that only forward another call. No globals: everything that touches the outside world goes through `cli.Deps`.
- All user-facing text (help, hints, messages, errors, glyphs) lives in `internal/text/strings.toml` and is reached only through the catalog; Go code passes keys and data, never English sentences. Per-OS variants are sibling keys with a `_<goos>` or `_unix` suffix. The command name is always the `{name}` placeholder, because the name is a build-time value.
- Results go to stdout; prompts, warnings, errors and verbose lines go to stderr. Only `internal/term` and `cmd/agent-cli` touch `os.Stdout` or `os.Stderr`, and nothing uses `fmt.Print*` (`forbidigo` enforces it).
- Secrets (tokens, passphrases, connect keys) are never logged, never printed in errors or verbose output, never written to `config.toml` and never passed in argv. Text that originates outside the process goes through `term.Clean` before printing.

### Testing

- All tests are offline and deterministic on macOS, Linux and Windows. Use the fakes in `internal/testkit` (`NewDeps`, `FakeTerm`, `FakeClock`, `Run`, `Golden`, `AssertNoSecret`), `httptest`, temporary directories and temporary git repositories; never touch the real keychain, network or home directory.
- `cli.Deps` is the only seam: environment, streams, terminal, clock, location, GOOS/GOARCH, hostname, home directory, keychain, HTTP transport, scrypt work factor and the build identity. It grows with each milestone.
- Golden files live in `testdata/*.golden`; rewrite them with `UPDATE_GOLDEN=1 go test ./...` and review the diff.
- The `e2e` package runs the four real builds from `tools/build` on each OS (`make e2e`, `scripts/e2e.sh`); without the script it skips. Each milestone that adds a command adds that command's offline basics there: help, exit codes, and the output that differs per build.
- Every spec scenario maps to at least one test. Tests that need POSIX permission bits or the shell tools skip on Windows.
- Test behavior, not construction or wiring. Ask before creating new test files outside the ones a task names.
- Always run the whole gate before a stop or a pull request: `go build ./... && go vet ./... && go test -race -count=1 ./... && golangci-lint run && sh scripts/e2e.sh`. Windows runs the tests without `-race`.

### Comments

- For new code, avoid comments unless the logic is genuinely complex or the block is long
- When editing existing code, prefer updating comments over deleting them
- Start short ordinary comments with lowercase, except where grammar requires otherwise; doc comments of exported identifiers begin with the identifier's name. No `TODO` or `FIXME` comments.

### Error Handling

Domain packages return typed errors; `cli` maps each to an exit code and a catalog key in one table and prints it through `cli.Present` (`error: <text>`). Exit codes are the constants in `internal/cli`: 0 ok, 1 failure, 2 usage or validation, 3 not connected, locked or revoked, 4 chat unreachable, 5 insufficient credits, 6 rate limited; no other code is used. Never print an error anywhere else, and wrap with `%w` to keep the chain.

---

## MANDATORY PROJECT RULES

### Environment Management

- ALWAYS use the Go toolchain pinned by `go.mod` (the `go` directive is the single source of truth for the Go version; CI reads it from there)
- ALL commands must be run from the project root (where `go.mod` exists)
- Use `go get` and `go mod tidy` for dependencies; commit `go.mod` and `go.sum` together

### Development Workflow

- Use `make build`, `make test`, `make lint` and `make fmt` for everyday work; they wrap the Go tools and the scripts
- Builds: `tools/variants` is the only definition of the four builds: the generic `agent-cli` and `agent-cli-dev` (backend `http://localhost`) and the hosted-service conveniences `abot` (production) and `abot-dev` (staging). `./tools/build <name>` builds one for this machine into `bin/`; `THE_AGENT_CLI_BUILD_API_URL` and `THE_AGENT_CLI_BUILD_PRODUCT_URL` override its URLs. `make snapshot` builds the release archives into `dist/` without publishing
- The version is the single line in `VERSION` and nowhere else; the binary embeds it, and the pipelines and tags read it. For version bumps, run `./tools/bump_version {major|minor|patch}`; major and minor bumps reset lower version segments
- Work follows `openspec/changes/bootstrap-abot-cli/tasks.md`: groups are milestones (group N is M(N-1)), strictly sequential, with tests and docs inside the group that owns the behavior. The last task of every group is a STOP: present what changed, the files touched, the verification output and what is pending on the owner, then wait. Never start the next group before approval, and tick a task only when its specified behavior is fully done and verified

### Code Quality

- Run `make lint` (golangci-lint plus `scripts/check-eof.sh`) before commits and at the end of tasks; every text file ends with a newline and empty lines carry no indentation
- Tool versions: `golangci-lint`, `goreleaser` and `govulncheck` are pinned in `.github/workflows/verify.yml`; install the same versions locally

### Project Structure

- Entry point `cmd/agent-cli/main.go` (wiring only); everything else is under `internal/`: `buildinfo`, `text`, `term`, `config`, `token`, `credentials`, `api`, `session`, `render`, `cli`, `testkit`
- All scripts are in `tools` (POSIX `sh`, common `messages` for colored output): `variants`, `build`, `release`, `bump_version`, `create_pr`; repository checks are in `scripts`
- A release that failed after its tag was pushed is cleaned up by `tools/release`; if the runner died in between, delete the tag and any partial release by hand (`gh release delete <tag> --cleanup-tag`) and re-run the workflow
- Scripts validate project root location and fail safely if run from the wrong directory
- You can see the CI/CD pipeline in `.github/workflows` directory: `qa.yml` (pull requests to `main`, verification only), `beta.yml` (pull requests to `release`, `tools/release beta`: the `v<VERSION>-beta` pre-release with `abot-dev` and `agent-cli-dev`) and `release.yml` (push to `release`, `tools/release ga`: the `v<VERSION>` release with `abot` and `agent-cli`); all three call the reusable `verify.yml`. Releases are built by GoReleaser from `.goreleaser.yaml`, always through `tools/release`
- Behavior is specified in `openspec/`; read the change's `design.md` and the specs before changing behavior, and keep them in sync with the backend's API docs (`../agent-backend/docs/open-api-docs.yaml`)
