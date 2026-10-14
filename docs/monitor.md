# monitor：只读可观测查询

`bk-cli monitor` 通过 Unify Query API Gateway 查询日志、Trace、结构化指标与 PromQL。
它使用 bk-cli 的 context、加密凭据、请求执行和 JSON envelope。

```bash
bk-cli monitor logs --config ./monitor.json --env dev --query ERROR --since 30m
bk-cli monitor trace --config ./monitor.json --env dev --id TRACE_ID
bk-cli monitor metrics --config ./monitor.json --env dev --field kube_pod_info --aggregate sum
bk-cli monitor query --config ./monitor.json --env dev --promql 'sum(kube_pod_info{namespace={{namespace}}})'
bk-cli monitor status --config ./monitor.json
```

## 部署与授权

先使用已有的 `context`、`auth` 命令配置目标部署并登录。URL 模板必须包含 `{gateway_name}`，不包含 stage。
应用必须获得目标网关资源的调用权限。monitor 的默认资源使用应用认证；实际权限仍由网关验证。

```bash
bk-cli context init --bk_api_url_tmpl='https://{gateway_name}.apigw.example.com'
bk-cli auth login --bk_app_code='APP_CODE' --bk_app_secret='APP_SECRET' --bk_token='USER_TOKEN'
bk-cli auth check
```

使用实际部署地址替换示例。内网部署可以使用 `bk_ticket`，也可以使用独立 `access_token`。
当前 `auth login` 的应用凭据模式要求同时提供用户 token/ticket，不能只保存 app code + secret。
monitor 使用当前 context 的统一认证存储。
`auth check` 检查本机凭据；`monitor status` 通过实际查询检查资源权限与响应。

## 项目资源配置

context 描述 BlueKing 部署；`--env` 描述项目的查询资源映射；`--stage` 描述 API Gateway stage。
三者相互独立。例如同一个部署的项目 dev/prod 数据表通常都通过网关的 prod stage 查询。

```json
{
  "default_env": "dev",
  "apigw": {
    "gateway_name": "bk-unify-query",
    "query_raw_path": "/query/ts/raw",
    "query_ts_path": "/query/ts",
    "query_promql_path": "/query/promql"
  },
  "environments": {
    "dev": {
      "space_uid": "YOUR_TRACE_SPACE",
      "trace_table_id": "YOUR_TRACE_TABLE",
      "log_space_uid": "YOUR_LOG_SPACE",
      "log_table_id": "YOUR_LOG_TABLE",
      "log_tables": {"stdout": "YOUR_LOG_TABLE"},
      "metrics_space_uid": "YOUR_METRIC_SPACE",
      "metrics_namespace": "YOUR_NAMESPACE"
    }
  }
}
```

替换 `YOUR_*` 占位符。未配置专属空间时，日志和指标使用 `space_uid`。
配置文件优先级：`--config` > `BK_CLI_MONITOR_CONFIG` > `~/.bk-cli/monitor.json`。
设置 `BK_CLI_CONFIG_DIR` 后，默认文件改为该目录下的 `monitor.json`。直接编辑文件中的 `environments` 即可维护环境映射。
默认文件不存在时，可直接通过 `--space-uid` 和 `--table` 指定资源；显式配置文件必须存在。
环境优先级：`--env` > `BK_CLI_MONITOR_ENV` > `default_env`；空间优先级：`--space-uid` > `BK_CLI_MONITOR_SPACE_UID` > 环境映射。
`monitor envs` 展示资源映射，不展示凭据。

默认网关名为 `bk-unify-query`，默认 stage 为 `prod`，资源路径如上。
环境内的 `apigw` 整体覆盖顶层 `apigw`；`--stage` 可覆盖配置 stage。
如配置 `apigw.base_url`，它必须匹配当前 context 渲染出的网关地址，不能绕过 context。
自定义网关需要填写 `gateway_name`。`proxy_path` 可使用 `{ "path": "UQ内部路径", "data": {...} }` 协议。
monitor 始终走 bk-cli 的网关调用路径。

## 查询规则

- `logs` 支持 `--log-source`、`--query`、重复的 `--where FIELD=VALUE`、`--from`、`--order-by`。
- `trace` 将 `--id` 转为精确过滤；默认字段为 `trace_id`，可用 `--trace-field` 覆盖。
- 默认查询最近一小时、最多 100 条。`--start`/`--end` 支持 Unix 秒或带时区 RFC3339；`--start` 与 `--since` 互斥。
- `metrics` 支持 `--field`、`--aggregate`、`--group-by`、`--step`、`--instant`。
- metrics 在全部 query_list 中追加或校验 namespace；冲突 namespace、OR 条件和非法条件结构会在请求前失败。
- `query` 支持原生 PromQL。`{{namespace}}` 替换为带引号的配置 namespace；表达式中的占位符不要再加引号。没有占位符的表达式保持原样。
- `--namespace` 覆盖构造查询的 namespace；`--timeout` 覆盖该次请求超时。
- 一页 span 不代表完整 Trace；`--from` 是手动翻页入口，没有自动聚合。

别名：`queryRaw`/`query-raw` → `logs`，`queryTs`/`query-ts` → `metrics`，`promql` → `query`；`query --query` 是 `--promql` 的别名。

## 完整请求、空间隔离与输出

`--body '<json>'` 优先于构造查询 flags。`--input FILE` 或 `--input -` 接受完整 UQ JSON，且与构造查询 flags 互斥。
两种完整输入不能同时使用。JSON 中的未知字段和请求大整数保持原样；metrics 的 namespace 校验仍执行。
输入中的 `space_uid` 必须匹配配置，并移至 `X-Bk-Scope-Space-Uid` 请求头。
禁止 `X-Bk-Scope-Skip-Space`，禁止通过 header 覆盖为其他 space。

```bash
cat request.json | bk-cli monitor logs --config ./monitor.json --env dev --input -
bk-cli monitor logs --config ./monitor.json --env dev --dry-run
```

stdout 使用 bk-cli envelope，UQ 查询结果位于 `data`；APIGW 的 `result/data` 包装会被解开。
`--dry-run` 采用共享的 `dry_run/request` 结构并脱敏鉴权头。
HTTP 错误、网关拒绝、UQ 内层错误返回非零退出码；`is_partial:true` 或非空 `status.code` 保留 stdout 数据，同时返回系统错误（退出码 2）。
输入错误使用退出码 1。共享响应解析器对超出安全整数范围或浮点转换会改变数值的 JSON number 保留原始数值字面量，嵌套对象和数组中的超大整数也不会被舍入，输出仍为 JSON 数值。

## 资源检查

`monitor status`（别名 `verify`）默认遍历所有配置环境，不受 `BK_CLI_MONITOR_ENV` 或 `default_env` 限制；`--env` 仅检查指定环境。
每个环境先验证 `vector(1)` 的实际返回值，再检查已配置日志源、Trace 表，以及 namespace 下的结构化指标和 PromQL。
默认日志表与某个命名源重复时不重复检查；未配置的表不生成探针。

结果只含计数、查询 trace_id 和检查状态，不输出日志/span payload。
失败后继续剩余检查，stdout 保留完整摘要，最终返回非零。
`empty` 仅表示请求成功但时间窗内没有数据，不代表采集健康。
`--local` 不联网，返回 `local_only:true`、`verified:false` 及凭据文件是否存在；它不验证文件内容，凭据有效性请使用 `auth check`。
status 的 `--dry-run` 只预览第一跳请求，不执行在线验证。


本系统提供查询与资源检查，不包含告警、事件、仪表盘管理或通用元数据发现。

响应脱敏仅针对认证凭证。`X-Bk-Tenant-Id`、`X-Request-Id` 等普通请求头的值不会作为密钥替换查询结果。
