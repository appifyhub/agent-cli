# Spec Delta

## Purpose

Defines proposed Go and CLI engineering principles for owner review, followed by the M12 assessment and remediation process immediately before M13 final testing and release rehearsal. This is a change-local review contract, not permanent project guidance. No code assessment or remediation is authorized by drafting these rules.

The rules synthesize primary Go guidance, Cobra's API contract and CLI usability guidance researched on 2026-11-19. Exact project constraints are identified separately; they are not claimed to be universal Go requirements. Effective Go is historical guidance: current language/API contracts and the pinned toolchain take precedence. Examples below are hypothetical, not findings against this repository.

## ADDED Requirements

### Requirement: Section 1 — Readability and proportionate simplicity

The assessment SHALL apply rules GO-01 through GO-03 only where evidence demonstrates a concrete correctness, maintenance or resource benefit.

- **GO-01 — Clear contracts:** Code MUST make meaningful inputs, outcomes, ownership and failure paths understandable. Names and necessary comments MUST explain contracts or non-obvious reasoning rather than narrate statements.
- **GO-02 — No style churn:** Changes MUST follow established formatting and naming. Reviewers MUST NOT impose arbitrary function/file/line-count limits, mandatory extraction, interfaces or dependencies. A different idiomatic spelling alone is not a violation.
- **GO-03 — Useful efficiency:** Code MUST avoid demonstrably unnecessary copying, allocation and repeated work when a simpler equivalent exists. More complex optimization MUST be justified by relevant measurement or a directly demonstrated resource problem, not hypothetical scale.

Good: keep a cohesive operation together; extract a helper when it gives an actual invariant a clear owner. Bad: add forwarding helpers solely to shorten functions, or speculative pools and caches.

Sources: [Effective Go](https://go.dev/doc/effective_go), [Code Review Comments — line length](https://go.dev/wiki/CodeReviewComments#line-length), [Go diagnostics](https://go.dev/doc/diagnostics).

#### Scenario: Equivalent idiomatic code
- **GIVEN** working code and a proposal that only substitutes another idiomatic spelling
- **WHEN** the judge evaluates the proposal
- **THEN** it is rejected without changing the code

### Requirement: Section 2 — Architecture and dependency ownership

The assessment SHALL apply ARCH-01 through ARCH-03 against the existing package contracts, without imposing a new architecture.

- **ARCH-01 — Preserve boundaries:** Code MUST preserve the declared package dependency graph. Cobra and command presentation MUST stay in `cli`; `term` MUST remain domain-independent, `api` MUST retain its restricted internal dependencies, and production code MUST NOT import `testkit`.
- **ARCH-02 — Explicit capabilities:** Command-level runtime capabilities covered by `cli.Deps` MUST originate at that boundary. Lower layers MUST receive required capabilities or values without importing `cli` or treating `Deps` as a service locator. Concrete production adapters may perform the OS operations they encapsulate; this is not a ban on real I/O inside those adapters.
- **ARCH-03 — Earn abstractions:** New interfaces, wrappers, frameworks and packages MUST serve an actual consumer or responsibility. They MUST NOT be introduced solely for hypothetical reuse or mocking; existing suitable interfaces and concrete types remain valid.

Good: pass context and the required store/client capability into an operation. Bad: pass `*cobra.Command` into domain logic to retrieve flags and print results. Neither interfaces everywhere nor no interfaces is a rule.

Sources: [Code Review Comments — interfaces](https://go.dev/wiki/CodeReviewComments#interfaces), [Effective Go — package names](https://go.dev/doc/effective_go#package-names). Exact layering and `Deps` boundaries are existing project contracts.

#### Scenario: Proposed abstraction
- **GIVEN** a proposal to add an interface or package
- **WHEN** the assessor cannot identify a real consumer requirement or responsibility boundary
- **THEN** the proposal is not reported as a necessary improvement

### Requirement: Section 3 — Errors as contracts

The assessment SHALL apply ERR-01 through ERR-03 to expected failures and the information their callers need.

- **ERR-01 — Honest outcomes:** Expected operational failures MUST return errors rather than panic or terminate the process. Failures affecting required persistence, writes or finalization MUST affect the outcome. Explicitly best-effort operations are not automatically violations when their contract permits failure.
- **ERR-02 — Preserve semantics:** Domain errors MUST retain typed information required by callers. Wrapping MUST preserve the relevant chain through `%w` or `Unwrap`; wrapped matching MUST use `errors.Is` or `errors.As`. Error-message strings MUST NOT become a classification protocol.
- **ERR-03 — One presentation boundary:** Established CLI translation and `Present` MUST own user-facing error presentation and exit codes. Lower layers MUST NOT print-and-return the same failure; raw underlying errors MUST NOT bypass the applicable catalog or secret-protection contract.

Good: `errors.Is(err, context.Canceled)`. Bad: `err.Error() == "context canceled"`. Wrapping exposes a programmatic contract, not sanitization; not every internal implementation error needs to become public API.

Sources: [Working with Errors in Go](https://go.dev/blog/go1.13-errors), [Handle errors](https://go.dev/wiki/CodeReviewComments#handle-errors), [Don't panic](https://go.dev/wiki/CodeReviewComments#dont-panic). Catalog and exit mapping are existing project contracts.

#### Scenario: Required operation fails
- **GIVEN** an operation reports success after a required write failed
- **WHEN** an assessor traces the caller and observes that the failure cannot reach its outcome
- **THEN** the finding includes that concrete path and the required error behavior

### Requirement: Section 4 — Context and resource lifetimes

The assessment SHALL apply LIFE-01 through LIFE-03 where cancellation or resource ownership is relevant, not to every pure helper.

- **LIFE-01 — Propagate cancellation:** Operation-scoped blocking work MUST propagate its caller's context, conventionally as the first parameter. It MUST NOT replace that context with `context.Background()` for convenience, pass nil, or use context values as a dependency container or optional parameters.
- **LIFE-02 — Own cleanup:** Acquired resources and derived cancellation functions MUST have clear owners and release paths on success, failure and cancellation. Caller-owned resources MUST NOT be closed without an ownership contract.
- **LIFE-03 — Honor I/O contracts:** HTTP handling MUST distinguish transport, status and application outcomes and close owned response bodies. Required write/finalization errors MUST affect the operation's outcome.

Good: derive a deadline from the parent context and defer its cancellation. Bad: abandon cancellation with a new background context or retain loop resources until a much later function exit without justification. `defer` is function-scoped, not block-scoped.

Sources: [Context contract](https://go.dev/src/context/context.go), [Effective Go — defer](https://go.dev/doc/effective_go#defer), [HTTP client contract](https://go.dev/src/net/http/client.go).

#### Scenario: Cancellation path
- **GIVEN** a cancellable operation owns a resource
- **WHEN** an assessor evaluates an early return or cancellation
- **THEN** the finding identifies a missing release or blocked operation on that path, not merely a preferred cleanup spelling

### Requirement: Section 5 — Concurrency and mutable ownership

The assessment SHALL apply CONC-01 through CONC-03 only to actual concurrency and ownership requirements.

- **CONC-01 — Account for goroutines:** Each goroutine MUST have a justified purpose, clear lifetime and termination mechanism. Its owner MUST arrange completion where subsequent work depends on its effects or release of resources. Cancellation alone MUST NOT be mistaken for joining.
- **CONC-02 — Synchronize mutable state:** Shared mutable access MUST follow an understandable ownership or synchronization protocol. Active synchronization primitives MUST NOT be copied. Channel closure and blocking operations MUST remain safe when consumers stop early.
- **CONC-03 — No concurrency dogma:** Synchronous code MUST remain acceptable. Reviewers MUST NOT demand channels over mutexes, atomics over locks, or parallelism without a concrete need. Increasing buffers MUST NOT substitute for fixing a demonstrated lifetime problem.

Good: a cancellable producer selects between sending and context cancellation when its consumer can stop. Bad: an unconditional blocking send after that consumer abandons the stream.

Sources: [Go memory model](https://go.dev/ref/mem), [Goroutine lifetimes](https://go.dev/wiki/CodeReviewComments#goroutine-lifetimes), [Synchronous functions](https://go.dev/wiki/CodeReviewComments#synchronous-functions), [Pipelines and cancellation](https://go.dev/blog/pipelines).

#### Scenario: Suspected goroutine leak
- **GIVEN** an assessor suspects a blocked goroutine
- **WHEN** the actual producer, consumer and cancellation paths are traced
- **THEN** the report identifies the blocking condition and observable lifetime impact, or records no finding

### Requirement: Section 6 — Cobra as an adapter

The assessment SHALL apply CMD-01 through CMD-03 using the installed Cobra version and approved command contracts.

- **CMD-01 — Validate before effects:** Commands MUST enforce declared argument and flag contracts before effects that require valid input. Validation MUST retain the project's usage outcome and cataloged application messages; already-approved framework parser output is not a violation merely because it originates in Cobra.
- **CMD-02 — Return failures:** Fallible command handlers MUST propagate errors through error-returning execution. Execution MUST use injected streams and the caller's context and reach the established presenter without duplicate printing.
- **CMD-03 — Respect lifecycle semantics:** Code MUST NOT assume every ancestor hook runs or that post-run hooks provide unconditional cleanup. Required cleanup MUST remain correct if parsing, validation, pre-run or execution fails.

Good: acquire and release resources inside the owning operation. Bad: acquire in `PreRunE` and depend exclusively on `PostRunE` when execution may fail. Cobra examples using globals, `init` or direct printing demonstrate its API, not this project's architecture.

Sources: [Cobra command guide](https://cobra.dev/docs/how-to-guides/working-with-commands/), [Cobra execution implementation](https://github.com/spf13/cobra/blob/main/command.go); installed-version behavior and project command specs are controlling evidence.

#### Scenario: Hook-based cleanup
- **GIVEN** a resource is acquired before command execution
- **WHEN** execution fails
- **THEN** assessment checks the actual release path rather than assuming a post-run hook executes

### Requirement: Section 7 — CLI usability and presentation

The assessment SHALL apply UX-01 through UX-03 without adding unrequested CLI features.

- **UX-01 — Separate streams:** Results MUST go to stdout; prompts, warnings, errors and verbose messages MUST go to stderr through the established boundary. Output consumed by scripts MUST NOT contain incidental diagnostics or inappropriate terminal decoration.
- **UX-02 — Centralize text:** Application-facing text, help, hints, errors and glyphs MUST use the catalog and build identity under existing contracts. Command names in catalog templates MUST use `{name}`, not hard-coded distribution names.
- **UX-03 — Respect automation:** Supported noninteractive paths MUST NOT block on prompts. Help MUST remain available without unrelated authentication or network work. Existing flags, configuration precedence, exit codes and output contracts MUST NOT change as cleanup.

Good: results on stdout and cataloged warnings on stderr. Bad: progress text mixed into script-consumed results. Use the relevant stream's terminal status; stdout and stderr need not match. This does not mandate new JSON modes, aliases, progress bars, flags or configuration sources.

Sources: [CLI basics](https://clig.dev/#the-basics), [Help](https://clig.dev/#help), [Output](https://clig.dev/#output), [Interactivity](https://clig.dev/#interactivity), [Future-proofing](https://clig.dev/#future-proofing). Catalog and build identity are existing project contracts.

#### Scenario: Scripted invocation
- **GIVEN** a supported command is invoked with piped streams
- **WHEN** an assessor reviews output and prompting
- **THEN** findings identify a violated stream or noninteractive contract, not a missing unrequested feature

### Requirement: Section 8 — Secret and terminal trust boundaries

The assessment SHALL apply SEC-01 through SEC-03 while preserving explicitly approved secret-export behavior.

- **SEC-01 — Prevent disclosure:** Tokens, passphrases and connect keys MUST NOT enter logs, diagnostic payloads, ordinary config or error text. Secrets MUST NOT be passed in child-process argv. The specified connect-key argument and `unlock` token-export stdout are narrow explicit exceptions, not permission for other disclosures.
- **SEC-02 — Sanitize display:** External text MUST pass through `term.Clean` before terminal display. Cleaning MUST occur at presentation, without corrupting protocol/domain values, and MUST NOT be treated as secret redaction.
- **SEC-03 — Appropriate primitives:** Security-sensitive randomness MUST use approved cryptographic randomness rather than `math/rand`. Approved storage and crypto mechanisms MUST NOT be weakened for test convenience or dependency reduction.

Good: render a cleaned non-secret display name through the catalog. Bad: print `term.Clean(token)` and assume the token is protected. Prompt masking does not protect subsequent logging.

Sources: [CLI arguments and flags](https://clig.dev/#arguments-and-flags), [Secret prompting](https://clig.dev/#interactivity), [Crypto randomness](https://go.dev/wiki/CodeReviewComments#crypto-rand). Exact secret exceptions, storage and terminal cleaning are project contracts.

#### Scenario: Sanitized secret
- **GIVEN** terminal cleaning removes control characters from a secret
- **WHEN** that secret is printed outside the specified export behavior
- **THEN** the disclosure remains a violation; the report must not include the secret value itself

### Requirement: Section 9 — Deterministic behavioral tests

The assessment SHALL apply TEST-01 through TEST-04 to consumer-visible behavior and actual isolation requirements.

- **TEST-01 — Isolate state:** Offline tests MUST use existing injected fakes, controlled transports or `httptest`, and temporary resources. They MUST NOT depend on live services, real credentials, home directories or uncontrolled clock/environment state. Explicit opt-in real-store integration checks and owner-run smoke checks remain separate authorized exceptions.
- **TEST-02 — Assert behavior:** Relevant success/failure behavior MUST be checked through meaningful outcomes: error semantics, exit codes, stream placement and secret absence. Tests MUST NOT pin incidental construction or call order unless that order is itself a required safety contract.
- **TEST-03 — Deterministic concurrency:** Tests MUST synchronize on completion rather than sleep and hope. Common behavior MUST remain cross-platform; genuinely OS-specific cases may be isolated rather than weakening common assertions.
- **TEST-04 — Match the contract:** Domain-error tests MUST compare semantics; contractual output/catalog tests MAY compare exact text. Tables, helpers, goldens or assertion libraries MUST NOT be demanded without a demonstrated benefit. New regression coverage MUST catch a plausible consumer-visible failure and follow the repository's test-file approval rules.

Good: trigger cancellation, await completion and check its returned error. Bad: sleep then inspect unsynchronized state. Race-detector success covers only executed paths and does not prove every concurrent path safe.

Sources: [Go Test Comments](https://go.dev/wiki/TestComments), [Testing time](https://go.dev/blog/testing-time), [Race detector](https://go.dev/doc/articles/race_detector).

#### Scenario: Proposed regression test
- **GIVEN** a candidate improvement needs regression coverage
- **WHEN** its proposed test is evaluated
- **THEN** it must distinguish the plausible broken behavior from the correct behavior without real network, home or credential dependencies

### Requirement: Section 10 — Owner approval before assessment

Rules and scope SHALL be explicitly approved by the owner before any violation assessment begins. Stable rule IDs SHALL identify the approved criteria. Material rule changes SHALL require renewed approval. Drafting the rules now SHALL NOT advance implementation into the future milestone.

#### Scenario: Rules await review
- **GIVEN** the rules have been drafted but not approved
- **WHEN** the planning work finishes
- **THEN** the owner receives the rules for review and no assessment, judge or remediation agents are launched

### Requirement: Section 11 — Independent evidence-backed assessment

At M12, fresh independent read-only agents SHALL cover each approved section or rule family against the same code baseline. Each SHALL write only its assigned `VIOLATIONS-{N}.md` in this change directory, or an explicit clean result. Findings SHALL name the rule, exact file/line, trigger, traced consuming behavior, impact, smallest useful fix and proposed verification, distinguishing observation from inference. Agents SHALL NOT invent issues, edit production code or run gates.

#### Scenario: Code satisfies a section
- **GIVEN** an assessor finds no concrete high-value problem under its assigned rules
- **WHEN** it completes its report
- **THEN** it records a clean result without manufacturing findings

### Requirement: Section 12 — Independent judgment

A new judge agent SHALL evaluate every report and verify its evidence against the actual code and approved contracts, including consuming dispatch paths. It SHALL deduplicate and retain only important, actionable, high-confidence improvements in `VIOLATIONS-FINAL.md`, with stable finding IDs, bounded fixes and verification. It SHALL document rejected findings and reasons, rejecting preference-only churn, speculative risks and disproportionate fixes. No findings SHALL be a valid successful outcome.

#### Scenario: Low-value proposal
- **GIVEN** a report proposes a stylistic rewrite without demonstrated impact
- **WHEN** the independent judge evaluates it
- **THEN** it is rejected and excluded from the accepted findings list

### Requirement: Section 13 — Bounded remediation and report removal

One new implementation agent, distinct from the assessors and judge, SHALL fix all accepted findings without opportunistic redesign. A finding disproved during implementation SHALL return for explicit judge disposition. After integration, the main agent SHALL run the required verification and confirm each disposition; the implementation agent SHALL then delete all temporary violation reports only when findings are resolved and verification passes. Reviewed principles SHALL remain. No automatic second audit or expanded checklist SHALL be required.

#### Scenario: Accepted fixes complete
- **GIVEN** each accepted finding is fixed or explicitly dismissed by the judge
- **WHEN** integration verification succeeds
- **THEN** the implementation agent removes `VIOLATIONS-{N}.md` and `VIOLATIONS-FINAL.md`, and the owner receives the concise M12 review before M13 begins

#### Scenario: No accepted findings
- **GIVEN** the judge accepts no findings
- **WHEN** M12 concludes
- **THEN** correct production code is unchanged, temporary reports are removed and the clean result is recorded
