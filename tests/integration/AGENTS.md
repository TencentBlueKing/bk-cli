# AGENTS.md — Integration Harness Guide

This file is for coding agents working inside `tests/integration/`.
It explains how to add or update integration coverage without drifting
from the current harness contract.

## Purpose

The integration harness runs the real built `bk-cli` binary inside
Docker Compose and validates behavior through declarative YAML cases.

Coverage expectation:

- keep one happy-path case for every public system action
- add extra negative cases only for selected risky mutating actions

Use this area when you need to:

- add coverage for a newly introduced public system
- add coverage for a new action under an existing system
- add more CLI regression coverage for `context`, `auth`, or `api`
- add mock-backed edge-case coverage that `httpbin` cannot express

## Mental Model

Keep these directory roles distinct:

- `cases/`: checked-in runnable scenarios, one `.yaml` file per scenario
- `fixtures/`: shared seed config and environment defaults reused by many cases
- `mock_api/`: centralized synthetic upstream behavior
- `runner/`: case loader, selector, executor, and report writer
- `artifacts/latest/`: generated outputs only, never author source here

When you are adding coverage, the usual edit surface is:

- one new file under `cases/`
- sometimes `mock_api/app.py`
- sometimes `README.md` when the workflow changes

Do not invent extra ad hoc helper directories unless the current runner
really cannot support the new case shape.

## File Placement Rules

### Non-system commands

Use the existing top-level coverage folders:

- `cases/context/`
- `cases/auth/`
- `cases/api/`

### System commands

System scenarios must live under:

```text
tests/integration/cases/system/<system>/
```

Examples:

- `tests/integration/cases/system/devops/`
- `tests/integration/cases/system/apigateway/`

If you add a new public system command to `cmd/system/<system>/`, add
its integration cases under the matching system folder here.

## One Scenario Per File

Each YAML file is one scenario and must contain:

- `id`: stable scenario identifier
- `name`: human-readable scenario name
- `steps`: ordered CLI invocations plus expectations

Recommended filename pattern:

```text
<SCENARIO_ID>-<system>-<action>-<behavior>.yaml
```

Examples:

- `SYSGO-001-devops-start-build.yaml`
- `SYSYAML-001-apigateway-list-gateways.yaml`

## How To Add Coverage For A New System

1. Confirm the public command exists under `cmd/system/<system>/` and is registered.
2. Create `tests/integration/cases/system/<system>/` if it does not exist.
3. Add one baseline happy-path YAML scenario for every public action.
4. Use `${MOCK_API_BASE_URL}` when the action expects domain-shaped data.
5. Prefer `${HTTPBIN_BASE_URL}` only for protocol-style request/response checks.
6. Run `make test-integration SCENARIO=<SCENARIO_ID>`.

For a brand-new public system, a good first case should prove:

- context initialization works with the target base URL
- auth is present
- the system command executes against the real `bk-cli`
- the returned JSON envelope includes the expected status and key fields

## How To Add Coverage For A New Action In An Existing System

1. Place the new case in the existing system folder.
2. Reuse the smallest possible setup:
   usually `context init`, then `auth login`, then the target action.
3. Choose one scenario file per user-visible behavior.
4. Keep one happy-path case first; add edge/failure cases only when they add value.
5. If the action has synthetic upstream behavior requirements, add a scenario-keyed branch in `mock_api/app.py`.
6. Run the case in isolation by scenario ID and by case path.

## When To Use `httpbin` vs `mock_api`

Prefer `httpbin` when you only need:

- echoing query/body/header data
- simple status-code behavior
- generic request-shape validation

Use `mock_api/app.py` when you need:

- system-specific response bodies
- paths that mirror BlueKing-style upstream routes
- deterministic error payloads
- scenario-keyed behavior differences

Do not create a second mock service unless the current single-file mock
becomes fundamentally insufficient.

## YAML Case Authoring Rules

Each step should be minimal and explicit:

- `command` is the exact CLI arg vector after `bk-cli`
- use `${...}` placeholders only for environment values already exposed by the harness
- assert on `exit_code`
- add only the checks needed to prove the behavior
- `bk-cli auth status` is a query command:
  with credentials it returns `exit_code: 0`, `ok: true`, and
  `data.has_credentials: true`; without credentials it still returns
  `exit_code: 0`, `ok: true`, and `data.has_credentials: false`.
- `bk-cli auth check` is the fail-fast command for scripts and CI:
  without credentials it returns a non-zero exit code plus
  `ok: false` / `error.code: no_credentials`.

Prefer expectations on stable fields such as:

- `ok`
- `status`
- `data.<field>`
- `error.code`
- `error.message`
- `request.url` for dry-run coverage

Avoid fragile assertions on:

- full response bodies when only one field matters
- formatting-sensitive message text unless the wording itself is the contract
- timestamps, random IDs, or large echoed payloads unless required

## Mock API Rules

Keep all custom upstream behavior in:

- [app.py](./mock_api/app.py)

When adding a new mock-backed case:

- key the behavior off the scenario ID or request header
- keep the payload deterministic
- keep handlers small and obvious
- prefer extending existing routes over adding many near-duplicate ones

## Verification Workflow

After adding or changing a case, run:

```bash
go test ./tests/integration/runner -count=1
make test-integration SCENARIO=<SCENARIO_ID>
make test-integration CASE=tests/integration/cases/system/<system>/<file>.yaml
```

Use the exact case path variant for at least one changed system case.

## Common Mistakes To Avoid

- putting system cases directly under `cases/system/` instead of `cases/system/<system>/`
- adding reusable input data to `cases/` instead of `fixtures/`
- checking generated files into `artifacts/latest/`
- adding a new helper layer when the runner already supports the case
- using live BlueKing endpoints or real credentials
- making assertions on unstable data when a smaller stable check would do

## Update Policy

Update [README.md](./README.md) in the same change if:

- the authoring workflow changes
- the layout changes
- the recommended verification commands change

This file should stay agent-focused. Keep contributor-friendly narrative
and onboarding steps in `tests/integration/README.md`.
