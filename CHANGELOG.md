# Changelog

All notable changes to this project will be documented in this file.

## [0.2.9] - Unreleased

发布前置条件：等待 AIDev [#21](https://github.com/TencentBlueKing/bk-cli/pull/21) 合入，补充其发布说明并重新验证后再发布。以下草案基于 `v0.2.8 → ee4d3eb`，包含已合并的 #41。

### Highlights

- **内置 `--jq` 结果投影**：支持对 API 和系统命令的完整返回结果进行筛选、提取和转换，无需安装外部 jq。分页命令在聚合完成后统一投影，输出为 JSON Lines。([#40](https://github.com/TencentBlueKing/bk-cli/pull/40), @robot-wklken)
- **个人 token 登录**：新增 `auth login --personal_token`，通过 Bearer 认证调用接口，支持凭据加密存储和认证信息脱敏。([#30](https://github.com/TencentBlueKing/bk-cli/pull/30), @robot-wklken)
- **环境变量登录**：支持通过 `BK_CLI_*` 环境变量提供四种登录凭据，减少在命令行参数中暴露凭据；显式 flags 优先，并校验环境模式冲突和空白输入。([#28](https://github.com/TencentBlueKing/bk-cli/pull/28), @robot-wklken)
- **批量申请网关权限**：新增 `apigateway apply_gateway_permission`，支持按网关批量申请资源权限，提交前要求明确确认，并保留申请单号及服务端返回的审批信息。([#38](https://github.com/TencentBlueKing/bk-cli/pull/38), @robot-wklken)
- **从文件或 stdin 读取请求体**：支持 `--body @file.json` 和 `--body -`，便于复用复杂 JSON、组合 shell pipeline 和执行自动化任务。([#19](https://github.com/TencentBlueKing/bk-cli/pull/19), @robot-wklken)

### Improvements

- 补充 usage skill 中的内置 `--jq` 示例、输出规则和失败处理指引，明确投影失败后不得自动重发请求。([#41](https://github.com/TencentBlueKing/bk-cli/pull/41), @wklken)
- 新增系统命令接入与变更的 Issue 模板，方便提交接口信息和命令需求。([#24](https://github.com/TencentBlueKing/bk-cli/pull/24), @wklken)

### Fixes

- **修复 JSON 数字精度丢失**：默认响应输出保留大整数、小数和指数的原始数字字面量，避免大 ID 被舍入；同步修复 CMDB 分页中的数字解析，确保相邻大整数主机 ID 不被错误去重。([#36](https://github.com/TencentBlueKing/bk-cli/pull/36), @robot-wklken)
- **修复帮助示例中的 shell 展开问题**：调整 URL、凭据、请求头和脚本示例的引用方式，避免复制执行时 `$`、命令替换等内容被意外展开。([#26](https://github.com/TencentBlueKing/bk-cli/pull/26), @robot-wklken)

### Upgrade Guide

- 使用个人 token 时，不要混用其他登录凭据或叠加 `X-Bkapi-Authorization`。权限申请接口仍要求应用身份，不支持个人 token。
- 环境变量凭据仅由 `auth login` 读取。建议限定注入作用域，登录完成后清理环境变量。
- `--jq` 投影失败时会保留原始响应并报告错误，**不代表请求未执行**。先检查任务、资源或申请单 ID，再决定后续操作。
- 内置 `--jq` 保持整数精度，但小数和指数遵循 gojq 浮点语义；需要保留原始数字字面量时省略 `--jq`。

## [0.2.7] - 2026-06-22

### Bug Fixes

- move to github and refactor the ci for github actions
- fix ci fails

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


