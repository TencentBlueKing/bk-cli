# Changelog

All notable changes to this project will be documented in this file.

## [0.2.6] - 2026-06-11

### Features

- Release/v0.2.5
- **api**: Support insecure HTTPS requests

## [0.2.5] - 2026-05-22

### Bug Fixes

- **context**: Align saved config output across list and status
- **auth**: Include credential type in auth check output
- **system**: Require bodies for YAML actions
- **docs**: Update the skills and fix the integration tests fail
- **system**: Map paas legacy gateway name
- **system**: Require body for paas deployment
- **makefile**: Integration-test failed

### Documentation

- **developer-guide**: Update
- **docs/user-guide**: Update
- **apigateway**: Use realistic UUID example for query_log_by_request_id
- **docs/user-guide.md**: Update
- **skill**: Sync devops subsystem guide

### Features

- V0.2.4
- Add query_log_by_request_id action
- **apigateway**: Add query_log_by_request_id action and update skill doc
- **system**: Support one-level subsystem registration
- **devops**: Split devops commands into subsystems
- **bcs**: Add cluster manager system actions
- **cmd/system/bcs**: Add bcs cluster_manager actions
- **system**: Add paas deployment actions

## [0.2.4] - 2026-04-21

### Bug Fixes

- **docs/user-guide.md**: Add user guide
- **cmdb**: Align empty response handling with current CLI contract
- **cr/comments**: Fix cr comments, add Agents for request

### CI

- Release v0.2.3

### Documentation

- **docs/develop-extension-guide.md**: Add doc
- **docs/design.md**: Update design.md
- **docs/user-guide.md**: Update
- **skills**: Align bk-cli guidance with usage perspective

### Features

- **docs/develop-extension-guide.md**: Add developer guide doc
- **bk-job/compat**: Support bk-job and jobv3-cloud in different envs
- **context/output**: Disable create auto change; fix output escape

## [0.2.3] - 2026-04-16

### Bug Fixes

- **cli**: Validate yaml defaults and auth login inputs
- **api**: Preserve large integers in request previews
- **gse**: Use CMDB bk_agent_id examples for agent list

### Documentation

- Changelog
- Update phase 3 example to use bk-apigateway/v2_open_list_gateways
- **cli**: Clarify permission-related help guidance

### Features

- Bk-cli update
- **makefile**: Add `make install`
- **specs**: Remove specs
- Add test-bk-cli skill for read-only bk-cli command testing
- Restrict system subcommand testing to native bk-cli commands only
- Reorganize test-bk-cli skill into 4 phases
- Enable bk-cli api testing in phase 3 via apigateway path discovery
- Add full context/auth lifecycle test in phase 2
- Hardcode test credentials in phase 2, no user prompt needed
- **cli**: Split version metadata from context status
- **auth**: Split credential status from login checks

### Miscellaneous

- Unnecessary files

### Testing

- **integration**: Add containerized CLI integration suite

## [0.2.2] - 2026-04-14

### Bug Fixes

- Conflict, sync with master

### Features

- Npm install

### Testing

- **internal**: Raise internal package coverage and add coverage gate

## [0.2.1] - 2026-04-13

### Bug Fixes

- Harden system runtime and registration

### Documentation

- **readme.md**: Update readme
- Add design doc consolidation spec
- **design.md**: Add design docs
- **readme_en.md**: Update
- Add request input refactor plans
- Add bk-cli system creation skills
- Docs/design.md
- Plan
- **context**: Clarify timeout duration examples
- **skills**: Move go-coded action guidance
- **agents**: Document system extension skills

### Features

- **bk-cli**: Sdd, first version
- **cmd/context**: Support legacy {api_name} in BK_API_URL_TMPL
- Add configurable request timeouts
- **system**: Add YAML authConfig handling
- **system**: Support help-only YAML header params
- **system**: Add job, sops, gse, devops, nodeman, bkmonitor systems (merge request !9)
- Npm

### Miscellaneous

- Ignore local worktrees

### Refactor

- System/actions
- **requestexec**: Fix timeout error labels
- Unify auth policy and system request spec
- Remove global format flag
- **skills**: Consolidate shared api debug guidance
- **system**: Align action docs and implementation terminology

### Styling

- **lints**: Fix lints

### Testing

- **system**: Align apigateway test filename

### Api

- Refine response header output


