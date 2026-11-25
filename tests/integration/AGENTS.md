# AGENTS.md — Integration Harness Guide

This file is for coding agents working inside `tests/integration/`.
It explains how to add or update integration coverage without drifting
from the current harness contract.

## Purpose

The integration harness validates the real built `bk-cli` binary through
declarative YAML cases. Docker Compose provides the default isolated services;
the Verification Workflow below also permits local execution when Docker
is unavailable.

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
- `cases/mcp/` (non-system MCP commands; use the `mcp` config seed and `/mcp` mock endpoint)

### System commands

System scenarios must live under:

```text
tests/integration/cases/system/<system>/
```

Examples:

- `tests/integration/cases/system/devops/`
- `tests/integration/cases/system/monitor/` — profile cases use synthetic `fixtures/monitor-profiles.yaml`; never real credentials or live IDs; direct-route cases must assert the individual HTTP path, and preflight cases must not execute business queries
- `tests/integration/cases/system/apigateway/`
- `tests/integration/cases/system/aidev/`

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
- `MONITOR-001-monitor-log-search-logs-scene.yaml`
- `SYSYAML-001-apigateway-list-gateways.yaml`

## How To Add Coverage For A New System

1. Confirm the public command exists under `cmd/system/<system>/` and is registered.
2. Create `tests/integration/cases/system/<system>/` if it does not exist.
3. Add one baseline happy-path YAML scenario for every public action.
4. Use `${MOCK_API_BASE_URL}` when the action expects domain-shaped data.
5. Prefer `${HTTPBIN_BASE_URL}` only for protocol-style request/response checks.
6. Run the new scenarios using the Verification Workflow below.

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
6. Run the case in isolation by scenario ID and by case path using the Verification Workflow below.

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

After changing the runner or adding or changing a case, run:

```bash
go test ./tests/integration/runner -count=1
```

Select the execution environment before running the affected scenarios:

- With usable Docker Compose and a reachable Docker daemon, run the commands below.
- Without usable Docker, run the same checked-in YAML cases against the locally
  built CLI through `tests/integration/cmd/inttest`, using the existing fixtures
  and local instances of the services required by those cases. Follow the
  [local execution example](README.md#无可用-docker-时的本地验证) for setup.

```bash
make test-integration SCENARIO=<SCENARIO_ID>
make test-integration CASE=tests/integration/cases/system/<system>/<file>.yaml
```

In either environment, run every affected scenario by ID and at least one
changed system case by its exact case path. Preserve the case steps and
expectations; change only environment values needed for local paths and service
addresses. Use the same build settings as `make test-integration`, including
`INTEGRATION_BK_TE_DOMAIN`, so legacy gateway checks retain their coverage.

For CLI, YAML case, fixture, mock response or runner logic changes, passing local
execution satisfies the repository's required local integration checks when
Docker is unavailable. This is a standing repository policy: continue already
authorized commit, push and PR delivery without requesting a per-task Docker
waiver. Other applicable checks still must pass. If a required service or
scenario cannot run, report that remaining verification blocker.

Changes to Compose configuration, Dockerfiles, container entrypoints, container
networking, mounts or service startup still require Compose verification in a
capable environment. Local execution alone does not verify those contracts.
The integration CI workflow continues to run the Compose suite.

Record the execution mode, commands, exit codes and report paths in the handoff
or PR. In a structured verification result, mark the applicable local checks as
required and report their actual results. Record an unexecuted Compose check as
not required for this local policy, with its environment reason; never report
it as passed. Preserve any failed Compose attempt as failure evidence. Disclose
that container orchestration remains unverified locally, and report CI status
separately from local results.

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
