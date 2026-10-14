---
name: bk-cli-monitor
description: 通过 bk-cli monitor 只读查询蓝鲸日志、Trace、结构化指标与 PromQL，检查项目各环境的查询资源状态。
---

# 蓝鲸可观测查询

先阅读 [共享使用规则](../bk-cli-shared/SKILL.md)。本系统的配置与查询细节见 [monitor 使用指南](../../docs/monitor.md)。

## 前置条件

- 已配置目标部署的 bk-cli context，且用户已通过 `bk-cli auth login` 登录。
- 应用具备 Unify Query 网关资源的调用权限。
- 项目提供环境资源 JSON，描述日志/Trace 表、查询空间与 metrics namespace。

不要向用户索取 secret、token 或 ticket，也不要读取凭据文件。认证由用户在本机完成。
bk-cli 当前的应用登录模式需要用户 token/ticket，也可使用独立 access token。
默认项目资源配置为 `~/.bk-cli/monitor.json`；可通过 `--config` 或 `BK_CLI_MONITOR_CONFIG` 指定。

## 查询流程

1. 使用 `bk-cli monitor envs --config /path/to/monitor.json` 确认环境映射。
2. 显式选择项目 `--env`。它不等于 BlueKing 部署的 `--context` 或网关 `--stage`。
3. 首先选择较小时间窗和条数；需要时先加 `--dry-run`。
4. 检查退出码、stdout 的 `data` 及 stderr，partial 结果不能判为完全成功。

```bash
bk-cli monitor logs --config ./monitor.json --env dev --query ERROR --since 30m --limit 20
bk-cli monitor logs --config ./monitor.json --env dev --log-source stdout --where service=api
bk-cli monitor trace --config ./monitor.json --env dev --id TRACE_ID --since 1h
bk-cli monitor metrics --config ./monitor.json --env dev --field kube_pod_info --aggregate sum
bk-cli monitor query --config ./monitor.json --env dev --promql 'sum(kube_pod_info{namespace={{namespace}}})'
bk-cli monitor status --config ./monitor.json
```

Trace ID 来自实际日志或请求链路；一页 span 不代表完整 Trace。
metrics 会校验 namespace，包括完整 JSON 请求。原生 PromQL 只替换显式 `{{namespace}}` 占位符，不自动改写其他 selector。
`--input -` 读取 stdin 的完整 UQ JSON；`--body` 覆盖构造查询 flags；两者不能同时使用。

## 结果判读

- 查询使用 bk-cli envelope，业务结果位于 `data`。
- HTTP、网关内层错误、UQ partial 或 status.code 都必须作为失败处理。
- `monitor status` 默认检查全部配置环境；`--env` 限定检查范围。
- status 的 `empty` 表示请求成功但窗口内没有数据；它不能证明采集正常。
- `status --local` 只展示本地状态，不验证凭据内容或网关权限。
- 输出、认证和退出码遵循 bk-cli 的统一契约。

若任务变为开发或扩展本系统，转而阅读仓库 `AGENTS.md`、`docs/design.md` 和 `.agents/skills/create-bk-cli-system/SKILL.md`。
