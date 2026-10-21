# Changelog

All notable changes to this project will be documented in this file.

## [0.2.9] - Unreleased

以下草案基于 `v0.2.8 → 801887f`，包含已合并的 AIDev #21 及其后续修复 #47。发布日期在正式发布时确定。

### Highlights

- **AIDev 资源管理**：新增 `bk-cli aidev`，提供 63 个 API action 和 `upload_file` 文件上传命令，覆盖空间、Skill、MCP、Prompt、智能体、知识库、知识文档、凭证和资源集合等操作。文件上传使用流式 multipart，支持本地文件校验和 dry-run 预览。([#21](https://github.com/TencentBlueKing/bk-cli/pull/21), @robot-wklken)
- **内置 `--jq` 结果投影**：支持对 API 和系统命令的完整返回结果进行筛选、提取和转换，无需安装外部 jq。分页命令在聚合完成后统一投影，输出为 JSON Lines。([#40](https://github.com/TencentBlueKing/bk-cli/pull/40), @robot-wklken)
- **个人 token 登录**：新增 `auth login --personal_token`，通过 Bearer 认证调用接口，支持凭据加密存储和认证信息脱敏。([#30](https://github.com/TencentBlueKing/bk-cli/pull/30), @robot-wklken)
- **环境变量登录**：支持通过 `BK_CLI_*` 环境变量提供四种登录凭据，减少在命令行参数中暴露凭据；显式 flags 优先，并校验环境模式冲突和空白输入。([#28](https://github.com/TencentBlueKing/bk-cli/pull/28), @robot-wklken)
- **批量申请网关权限**：新增 `apigateway apply_gateway_permission`，支持按网关批量申请资源权限，提交前要求明确确认，并保留申请单号及服务端返回的审批信息。([#38](https://github.com/TencentBlueKing/bk-cli/pull/38), @robot-wklken)
- **从文件或 stdin 读取请求体**：支持 `--body @file.json` 和 `--body -`，便于复用复杂 JSON、组合 shell pipeline 和执行自动化任务。([#19](https://github.com/TencentBlueKing/bk-cli/pull/19), @robot-wklken)

### Improvements

- 支持 AIDev 云环境网关名称兼容：在配置的 TE 网关模板中将 `bk-aidev` 映射为 `bkaidev`；提供配套使用指引和集成用例。([#21](https://github.com/TencentBlueKing/bk-cli/pull/21), @robot-wklken)
- 补充 usage skill 中的内置 `--jq` 示例、输出规则和失败处理指引，明确投影失败后不得自动重发请求。([#41](https://github.com/TencentBlueKing/bk-cli/pull/41), @wklken)
- 新增系统命令接入与变更的 Issue 模板，方便提交接口信息和命令需求。([#24](https://github.com/TencentBlueKing/bk-cli/pull/24), @wklken)

### Fixes

- **修正 AIDev 请求示例与 DELETE 输入**：归档和删除命令要求显式请求体，便于传入真实 `space_id`；修正 MCP、知识文档、凭证、资源集合和上传等示例，并同步命令帮助、使用指引及集成用例。([#47](https://github.com/TencentBlueKing/bk-cli/pull/47), @robot-wklken)
- **修复 JSON 数字精度丢失**：默认响应输出保留大整数、小数和指数的原始数字字面量，避免大 ID 被舍入；同步修复 CMDB 分页中的数字解析，确保相邻大整数主机 ID 不被错误去重。([#36](https://github.com/TencentBlueKing/bk-cli/pull/36), @robot-wklken)
- **修复帮助示例中的 shell 展开问题**：调整 URL、凭据、请求头和脚本示例的引用方式，避免复制执行时 `$`、命令替换等内容被意外展开。([#26](https://github.com/TencentBlueKing/bk-cli/pull/26), @robot-wklken)

### Upgrade Guide

- AIDev `upload_file` 使用 `--file` 和 `--space_id`，不使用 `--body`。归档或删除资源时，通过 `--body '{"space_id":"实际空间 ID"}'` 指定空间，不支持 `all`；完整输入以单个命令帮助和 `bk-cli-aidev` 使用指引为准。
- 使用个人 token 时，不要混用其他登录凭据或叠加 `X-Bkapi-Authorization`。权限申请接口仍要求应用身份，不支持个人 token。
- 环境变量凭据仅由 `auth login` 读取。建议限定注入作用域，登录完成后清理环境变量。
- `--jq` 投影失败时会保留原始响应并报告错误，**不代表请求未执行**。先检查任务、资源或申请单 ID，再决定后续操作。
- 内置 `--jq` 保持整数精度，但小数和指数遵循 gojq 浮点语义；需要保留原始数字字面量时省略 `--jq`。

## [0.2.8] - 2026-09-01

### Highlights

- **环境诊断**：新增 `bk-cli doctor`，检查上下文、脱敏凭据、URL 渲染结果和网关连通性，支持通过 `--offline`、`--gateway`、`--stage` 和 `--timeout` 进行离线或指定目标的检查。([#9](https://github.com/TencentBlueKing/bk-cli/pull/9), @wklken)
- **内置 Agent Skills**：新增 `bk-cli skills list` 和 `bk-cli skills read`，读取与当前二进制版本匹配的内置使用指引，并支持输出原始 Markdown。([#4](https://github.com/TencentBlueKing/bk-cli/pull/4), @wklken)

### Improvements

- **扩展 PaaS 应用管理**：新增 18 个 action，覆盖应用和模块查询、云原生应用创建、代码仓库分支、部署历史、日志、进程、环境变量及模块服务管理。([#15](https://github.com/TencentBlueKing/bk-cli/pull/15), [#18](https://github.com/TencentBlueKing/bk-cli/pull/18), @robot-wklken)
- PaaS 适用命令的模块参数默认使用 `default`，减少常见操作需要显式传入的参数。([#18](https://github.com/TencentBlueKing/bk-cli/pull/18), @robot-wklken)
- 添加项目 Logo，并调整中英文 README 展示。([#1](https://github.com/TencentBlueKing/bk-cli/pull/1), [#2](https://github.com/TencentBlueKing/bk-cli/pull/2), [#3](https://github.com/TencentBlueKing/bk-cli/pull/3), @wklken)

### Fixes

- **完善无效命令处理**：系统或子系统 action 名称拼写错误，或命令包含多余的位置参数时，返回机器可读错误并以非零状态码退出。([#13](https://github.com/TencentBlueKing/bk-cli/pull/13), @wklken)

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


