# AGENTS.md — bk-cli Agent Handbook

## Audience and document split

This file is for **coding agents and maintainers** working inside the repository.
It should contain the details that help an agent make correct changes quickly:

- architecture and package responsibilities
- command registration and extension flow
- high-level pointers to shared runtime, request, and auth contracts
- testing and verification rules
- repo-specific change checklists

Keep user-facing and contributor onboarding in:

- `README.md` — Chinese overview, installation, common usage, and contributor workflow
- `README_EN.md` — English overview, installation, common usage, and contributor workflow

Keep longer-form user and extension guides in:

- `docs/user-guide.md` — detailed user-facing workflows, examples, and troubleshooting
- `docs/develop-extension-guide.md` — developer-facing extension workflow and prompting guidance

Keep the detailed design baseline in:

- `docs/design.md` — full design contract and rationale

Keep system-extension workflow guidance in:

- `.agents/skills/create-bk-cli-system/SKILL.md` — `.agents` skill for system extension, carrying the workflow and must-check summary for adding or changing systems

Keep repo-local usage skills in:

- `skills/*/SKILL.md` — user-facing or request-executing-agent-facing command usage and troubleshooting

Do **not** turn the README files or `skills/*/SKILL.md` into implementation manuals. Detailed shared request-contract rules such as YAML param-to-flag generation, `authConfig` requirements, header precedence, and Go-implemented body synthesis should live in `AGENTS.md` and `docs/design.md`. The `.agents` skill should keep only generation-time must-check summaries, not the full contract.

## Working commands

If the repo provides `.envrc`, source it before running Go commands.

```bash
source .envrc
make fmt
make lint
make test
make test-cover
make build
```

Fallback direct commands:

```bash
go test ./... -count=1
go build -o bk-cli .
go build -ldflags="-s -w" -o bk-cli .
```

## Core mental model

`bk-cli` is a Go CLI for BlueKing APIs with an **agent-first** contract:

- default structured JSON output
- machine-readable errors and stable exit behavior
- non-interactive command design
- multi-context credential and runtime resolution
- reusable request execution shared by raw API calls and system commands

The architecture is intentionally **bottom-up**:

```text
internal/*   -> reusable core libraries
cmd/*        -> Cobra commands built on top of internal/*
main.go      -> thin entry point
```

Rules that should stay true:

- `internal/*` must never import `cmd/*`
- avoid circular dependencies
- keep command logic thin when reusable behavior can live in `internal/*`
- prefer predictable machine-facing contracts over clever CLI shortcuts

## Package map

### Root-level areas

| Path | Purpose |
|------|---------|
| `main.go` | Entry point only |
| `cmd/` | Cobra commands and command wiring |
| `internal/` | Reusable implementation libraries |
| `docs/design.md` | Detailed design contract and rationale |
| `docs/user-guide.md` | Extended user guide and examples |
| `docs/develop-extension-guide.md` | Developer-facing extension guide |
| `skills/` | System-specific and shared repo-local usage skills |
| `.agents/skills/` | Agent workflow skills for repository-specific development guidance |
| `.agents/skills/create-bk-cli-system/` | Guided workflow for adding or extending a public system |
| `tests/` | Additional higher-level test assets when needed |

### `internal/*`

| Package | Purpose |
|---------|---------|
| `internal/config` | Config model, context management, defaults, timeout settings |
| `internal/credential` | Credential types, encryption, and storage |
| `internal/api` | HTTP client, URL construction, auth header generation, request/response helpers |
| `internal/requestexec` | Shared request execution and precedence rules for timeout, tenant, and headers |
| `internal/output` | JSON envelope output and user-facing error helpers |
| `internal/system` | YAML-backed system/action models, action input spec building, runtime bridging, action execution |
| `internal/systemcmd` | Shared helpers for Go-implemented system commands: runtime resolution, request execution, flag helpers, validators, payload helpers |
| `internal/validate` | Shared validation helpers for names and header values |
| `internal/utils` | Small reusable helpers such as CSV parsing |
| `internal/auth` | OAuth device-flow placeholder |
| `internal/update` | Self-update implementation |

### `cmd/*`

| Path | Purpose |
|------|---------|
| `cmd/root.go` | Root command, global flags, shared wiring |
| `cmd/auth` | `auth login/status/check/logout` |
| `cmd/api` | Raw API command |
| `cmd/context` | Context lifecycle commands |
| `cmd/update` | Self-update command |
| `cmd/system` | Public system commands plus YAML/Go action registration |
| `cmd/system/testutil` | Shared helpers for system command tests |

## Current public command surface

### Root commands

- `auth`
- `api`
- `completion`
- `context`
- `help`
- `update`
- `version`
- registered public systems via `cmd/system/register.go`

### Public systems

These are the current registered systems in `cmd/system/register.go`:

| Public command | Module directory | Notes |
|----------------|------------------|-------|
| `apigateway` | `cmd/system/apigateway` | mixed YAML + Go |
| `bcs` | `cmd/system/bcs` | subsystem-based YAML; `cluster_manager` uses `bcs-api-gateway` |
| `paas` | `cmd/system/paas` | YAML-driven PaaS deployment actions |
| `cmdb` | `cmd/system/cmdb` | mixed YAML + Go |
| `job` | `cmd/system/job` | mixed YAML + Go |
| `sops` | `cmd/system/sops` | mixed YAML + Go |
| `gse` | `cmd/system/gse` | Go-implemented |
| `devops` | `cmd/system/devops` | subsystem-based mixed YAML + Go |
| `nodeman` | `cmd/system/nodeman` | Go-implemented |

## Command registration model

### System registration flow

Public systems are registered centrally in `cmd/system/register.go`.

The flow is:

1. add a thin wrapper file at `cmd/system/<system>.go` in package `system`
2. implement `NewSystemSpec()` in `cmd/system/<system>/spec.go`
3. add the new system spec to `systemCatalog()`
4. if system-level YAML actions exist, keep them at `cmd/system/<system>/actions.yaml`
5. if system-level Go actions exist, register them from the system spec
6. if the system has one-level subsystems, add subsystem specs under `SystemSpec.Subsystems`; each subsystem keeps its own `spec.go`, optional `actions.yaml`, and optional Go actions under `cmd/system/<system>/<subsystem>/`

A system with no subcommands is skipped instead of being added as an empty parent.

### Embedded YAML loading

`cmd/system/register.go` uses:

```go
//go:embed */actions.yaml */*/actions.yaml
```

That means new YAML action files must live exactly at one of these paths:

```text
cmd/system/<system>/actions.yaml
cmd/system/<system>/<subsystem>/actions.yaml
```

### Duplicate protection

Registration validates:

- duplicate top-level system names
- duplicate subsystem names under the same system
- nil child commands
- empty child command names
- duplicate direct child command names under the same parent, including action/subsystem conflicts

YAML action generation problems that are specific to one action are skipped with a warning instead of crashing the whole CLI.

## System implementation patterns

A public system can be **YAML-driven**, **Go-implemented**, or **mixed**. Use the simplest model that preserves a clean CLI contract.

For detailed step-by-step workflows, YAML field tables, `spec.go` templates, and `common.go` patterns, see `.agents/skills/create-bk-cli-system/SKILL.md`. For detailed shared request, auth, and I/O contract rules, treat `docs/design.md` as the source of truth. This section covers the architectural rules that stay true across all systems.

### Subsystem command groups

A public system may optionally expose one subsystem level:

```text
bk-cli <system> <subsystem> <action>
```

Use this when a large system has clear modules, such as the existing `devops pipeline`, `devops codecc`, and `devops stream` groups. Do not create deeper nesting. A parent system may still have its own YAML or Go actions, and the parent may also have subsystem children. Each subsystem owns its own YAML and Go action registration; subsystem YAML must declare its own `gateway_name` because different subsystems can call different API gateways.

Direct child names under the same parent must not conflict. A system action named `pipeline` conflicts with a subsystem named `pipeline`, but `devops pipeline list` and `devops codecc list` may coexist.

### YAML-driven action contract

YAML actions are good when the command mostly maps flags to a single remote request. Key rules:

- every YAML action must declare `authConfig`
- `authConfig.resourcePermissionRequired: true` requires `authConfig.appVerifiedRequired: true`
- `params` support `in: path`, `in: query`, and help-only `in: header`
- `in: body` is not allowed; request bodies come from the shared request layer instead
- generated flags come only from `path` and `query` params
- header params can enrich help text but must not create standalone flags
- complex request bodies should stay on the shared `--body '<json>'` input and expose body examples through `examples`; default help shows `Usage`, `Examples`, then a `body_schema` hint; full schema is shown only by `-h --body-schema`
- if the upstream OpenAPI request body is required, set `body_required: true` so YAML actions fail locally when `--body` is missing
- `--body-schema` is a help modifier, not an execution flag; `bk-cli ... --body-schema` without `-h` must fail before auth or request execution
- param names must not collide with built-in or reserved flags such as `body`, `body-schema`, `header`, `stage`, `help`, `context`, `dry-run`, `format`, `verbose`, and `insecure`
- `format` is currently reserved for future CLI use; do not treat it as an exposed flag today
- duplicate path/query names or generated-flag collisions skip that action with a warning
- path placeholders are escaped as single URL path segments

For detailed shared input, override, and preview semantics, prefer checking `docs/design.md` before changing the behavior.

### `authConfig` contract

`authConfig` controls the minimum identity material needed for CLI-generated `X-Bkapi-Authorization` headers:

- `appVerifiedRequired` means the request needs app identity, such as `bk_app_code` + `bk_app_secret`, or an `access_token` that satisfies the request
- `userVerifiedRequired` means the request needs user identity, such as `bk_token`, `bk_ticket`, or an `access_token` that satisfies the request
- when both are `true`, the generated auth header must contain both app and user identity
- when both are `false`, the CLI must not auto-generate `X-Bkapi-Authorization` and must not require local credentials for that request

Exact header payload shapes, validation, and redaction behavior remain defined in `docs/design.md` and the shared request/runtime packages.

### Go-implemented action contract

Use Go-implemented actions when you need body synthesis, JSON validation, pagination, multi-request orchestration, or richer UX. Constructor pattern:

```go
func newSomeActionCmd(deps systemcmd.BuildDeps) *cobra.Command
```

Typical `RunE` flow:

1. call `systemcmd.ResolveRuntime(deps)`
2. validate local flags
3. optionally synthesize a request body unless `--body` overrides it
4. call `systemcmd.ExecuteRequest(...)` for the actual request

### Canonical import aliases

When importing these packages, always use these aliases:

```go
syslib "github.com/TencentBlueKing/bk-cli/internal/system"
systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
```

Only import the ones you actually need.

### Shared helpers for Go-implemented actions

Prefer the shared helpers instead of rebuilding command plumbing:

- `systemcmd.ResolveRuntime(deps)`
- `systemcmd.ExecuteRequest(cmd, runtime, actionName, spec, mutate)`
- `systemcmd.EnsureEnvelope(actionName, env)`
- `systemcmd.AddCommonRequestFlags(...)`
- `systemcmd.AddCommonRequestFlagsWithoutBody(...)`
- `systemcmd.MarshalJSON(payload)`
- `systemcmd.ValidatePositiveIntFlag(...)`
- `systemcmd.ValidatePositiveIntFlagIfChanged(...)`
- `systemcmd.ValidateNonNegativeIntFlag(...)`
- `systemcmd.ValidateNonEmptyStringFlag(...)`
- `systemcmd.ParseJSONObjectFlag(flagName, raw)`
- `syslib.ExecuteRequest(runtime, spec)` for lower-level orchestration

### Local body synthesis rules

When a Go-implemented action supports both named flags and raw `--body`, treat `--body` as the explicit override. Skip local body synthesis if `--body` is provided; otherwise validate enough local inputs to build a correct request. When a flag accepts structured JSON, parse and validate it locally before putting it into the final payload.

## Runtime, auth, and request behavior

Detailed shared request-contract rules live in `docs/design.md`. When you add or change request behavior, treat `docs/design.md` as the source of truth for:

- context, stage, and tenant semantics
- timeout precedence and per-request overrides
- `X-Bkapi-Authorization` generation and redaction
- `X-Bk-Tenant-Id` and `--header` override behavior
- `--dry-run` / `--verbose` output semantics
- YAML action input rules, reserved flags, and path escaping

At the implementation level, context, credentials, and request defaults should still flow through the shared runtime helpers instead of being reconstructed ad hoc in each command.

## Testing conventions

- framework: **Ginkgo v2 + Gomega**
- colocate tests with source in `*_test.go`
- package suites live in `*_suite_test.go`
- use temp dirs plus `DeferCleanup` for cleanup
- set `BK_CLI_CONFIG_DIR` in tests when config isolation matters
- mock at the HTTP boundary with `httptest`
- reuse `cmd/system/testutil` before creating new helpers
- do not add duplicate per-system helper files for logic already covered by shared test helpers

Useful commands:

```bash
go test ./... -count=1
go test ./cmd/system/... -count=1
go test ./internal/config/ -v -count=1
```

## Extension workflow for agents

Before adding or changing a public system:

1. check whether the target system already exists
2. if the user's requirement or API list is naturally split by modules or sub-systems, stop and offer two command-shape choices before implementing:
   - flat actions under one system, such as `bk-cli devops get_build_list`
   - one-level subsystem grouping, such as `bk-cli devops pipeline get_build_list`
3. prefer reusing shared helpers and request contracts
4. decide whether the action should be YAML, Go-implemented, or mixed
5. add or update integration coverage under `tests/integration/cases/system/<system>/` when the public behavior changes
6. update or add system skill docs in the same change when the public surface changes
7. update user-facing docs if command names, flags, or examples changed

Use these repo-specific resources when relevant:

- `.agents/skills/create-bk-cli-system/SKILL.md`
- `docs/design.md`
- `tests/integration/AGENTS.md`

## Documentation policy

### What belongs in `README.md` and `README_EN.md`

Keep those files focused on:

- what the CLI is
- who it is for
- installation
- quick start
- common commands
- high-level usage rules
- contributor workflow and entry points

### What belongs in `docs/user-guide.md`

Keep that file focused on:

- longer walkthroughs and worked examples for CLI users
- task-oriented usage guidance that would make the README files too long
- troubleshooting flows and scenario-based guidance

### What belongs in `docs/develop-extension-guide.md`

Keep that file focused on:

- developer-facing extension workflow and preparation steps
- prompting patterns and repo-specific expectations for agents extending the CLI
- guardrails for when to stop and raise a proposal instead of changing shared contracts

### What belongs in `skills/*/SKILL.md`

Keep those files focused on:

- command usage from a user or request-executing agent perspective
- prerequisites, common workflows, examples, and troubleshooting steps
- shared usage behaviors that point back to `skills/bk-cli-shared/SKILL.md`
- explicit handoff to `AGENTS.md` and `docs/design.md` when the topic becomes implementation or extension work

Do not put implementation-only details here, such as YAML loader internals, `authConfig` field semantics, generated-flag rules, package wiring, or Go helper contracts.

### What belongs in `.agents/skills/create-bk-cli-system/SKILL.md`

Keep that file focused on:

- system creation/extension workflow
- must-check summaries for shared request/auth behavior while adding or changing systems
- extension-specific reminders for YAML action input rules and precedence for `stage`, `header`, `body`, `tenant`, and timeout
- common troubleshooting steps that are too low-level or repetitive for the README files
- generation-time checklists that point back to `docs/design.md` instead of restating the full contract

### What belongs in `AGENTS.md`

Keep this file focused on:

- implementation structure
- architectural invariants
- system registration flow
- YAML and Go-implemented action contracts
- testing and change-management rules
- short pointers to `docs/design.md` for detailed shared request contracts
- short pointers to `tests/integration/AGENTS.md` for integration-case authoring guidance

### Documentation sync rules

If you change any public command behavior, update the relevant docs in the same change:

- `README.md`
- `README_EN.md`
- `docs/user-guide.md` when longer user workflows, examples, or troubleshooting changed
- `docs/develop-extension-guide.md` when developer extension workflow or prompting guidance changed
- `AGENTS.md` when internal guidance also changed
- `tests/integration/AGENTS.md` when integration-case authoring or layout guidance changed
- `.agents/skills/create-bk-cli-system/SKILL.md` when system-extension workflow or must-check guidance changed
- related skill docs under `skills/`

If `README.md` changes, update `README_EN.md` too.

## Change checklist

When finishing a code or CLI-surface change, verify all relevant items:

- run `make fmt`
- run `make lint`
- run `make test`
- run `make test-cover` when coverage-sensitive code or thresholds changed
- run `make build`
- update user-facing docs for command/flag/example changes
- update agent guidance when architectural or extension rules changed
- update `tests/integration/AGENTS.md` or add system cases when system behavior changes
- add a matching skill when introducing a new public system under `cmd/system/`
- write new files under `skills/` in Chinese

## Known gaps

The repo still has some intentionally incomplete areas:

- OAuth device-flow placeholder in `internal/auth/device_flow.go`
- `--quiet` / `-q` flag is not yet implemented (noted in `docs/design.md`)

The design intent for current work should be inferred from `docs/design.md`, the README files, existing tests, and the current implementation.

If you are changing one of those areas, make the implementation status explicit in the docs or final handoff instead of implying the feature is already complete.
