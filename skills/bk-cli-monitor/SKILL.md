---
name: bk-cli-monitor
description: 当需要通过 `bk-cli monitor` 以用户权限查询蓝鲸观测数据时使用：业务空间、指标、日志、告警、事件、APM/Trace/Profiling、仪表盘、关联关系、告警配置、日志采集或日志提取状态。用户给出 bk_biz_id、告警 ID、索引集 ID、APM 应用、trace_id、指标表等标识，或要求判断某个观测工具能否执行时，优先使用本 skill。
---

# bk-cli monitor — 使用与排障

开始前读取 [共享使用规则](../bk-cli-shared/SKILL.md)。本 Skill 面向执行观测任务的 Agent；人工准备配置见 [Profile 初始化](../../docs/monitor.md)。资源标识必须来自用户已确认的 Profile 或当前账号可见目录，不猜测业务、表、索引集、应用或 Trace ID。


`bk-cli monitor` 面向研发、SRE 和自动化脚本，提供日志、指标、告警、调用链等观测数据的统一查询入口。用户可以在终端完成排查，也可以把同样的命令交给脚本或 Agent 使用，无需为每种数据分别编写接口调用。

它适合三类日常工作：

- **查问题**：检索错误日志、查看指标变化，从告警或单次调用继续查相关数据。
- **复用查询**：通过团队维护的 Profile 保存常用数据来源和查询参数，减少反复查找业务、索引集与应用名称。
- **接入自动化**：取得结构化 JSON 结果和明确的失败状态，供巡检脚本、排障工具或 Agent 处理。

> 使用前须具备包含 `monitor` 的构建、已登录的数据来源和对应资源权限。配置和预检均不替代执行时的服务端鉴权。

## 1. 快速开始：优先使用团队 Profile

Profile 是一份有名字的观测配置。SRE 提前确认业务、日志来源、指标表和 APM 应用，使用者选择名称即可，不必每次手工填写这些标识。

假设团队提供了 `team-profiles.yaml`，其中有名为 `checkout-prod` 的 Profile：

```bash
# 指定本次终端使用的观测配置文件
export BK_CLI_MONITOR_PROFILE_FILE=./team-profiles.yaml

# 看有哪些配置，再确认目标业务和数据来源
bk-cli monitor profile list
bk-cli monitor profile show --profile checkout-prod

# 查询最近 15 分钟的错误日志，最多返回 20 条
bk-cli monitor log search_logs --profile checkout-prod \
  --query_string 'ERROR OR WARN' --since 15m --limit 20
```

文件名和 Profile 名称都是示例，以团队实际交付为准。也可以在单次命令中使用 `--profile_file ./team-profiles.yaml`，不设置环境变量。

首次使用或配置更新后，可运行以下检查：

```bash
bk-cli monitor profile check --profile checkout-prod
```

它检查工具契约、权限和资源元数据，不读取日志正文，也不证明每条查询表达式都有效。普通查询不需要每次先检查；真正执行时，服务端仍会重新鉴权。

### 一个业务有多个组件或数据来源怎么办？

V2 Profile 可以分别配置后端、Worker 等组件，并把日志、指标和 Trace 指向各自的数据来源。同类指标还可以命名为 `traffic`、`latency` 等查询绑定。

```bash
# 示例 V2 文件由团队另行提供；先查看 backend 组件的配置
bk-cli monitor profile show --profile_file ./multi-source-profiles.yaml \
  --profile game-production --component backend

# 执行该组件预设的 traffic 指标查询
bk-cli monitor metric execute_range_query \
  --profile_file ./multi-source-profiles.yaml \
  --profile game-production --component backend --binding traffic --since 1h
```

- 有多个组件时，明确指定 `--component`；同一模块有多个绑定时，明确指定 `--binding`。
- 缺少配置或选择有歧义时会停止，不自动切换业务或数据来源。
- Profile **不是凭据，也不是授权**。共享配置文件不代表共享访问权限，更不能共享凭据目录。

配置格式、模板及覆盖规则见 [Profile 使用与交付](references/profiles.md)。

## 2. 没有 Profile：先找资源，再查询

命令统一采用以下结构，日志位于 `monitor log`，不是独立的 `bk-cli log`：

```text
bk-cli monitor <模块> <工具> [参数]
```

下文用引号包住的 `<业务ID>`、`<索引集ID>` 等都是占位符，执行前替换为当前账号可见的真实值。不要直接照抄示例标识，也不要让 Agent 猜测资源 ID。

```bash
# 按名称查找业务空间
bk-cli monitor space search_spaces --space_name '<业务名称关键词>'

# 查看该业务的日志索引集
bk-cli monitor log list_index_sets --bk_biz_id '<业务ID>'

# 确认所选索引集有哪些可查询字段
bk-cli monitor log get_index_set_fields \
  --bk_biz_id '<业务ID>' --index_set_id '<索引集ID>'

# 开始检索
bk-cli monitor log search_logs \
  --bk_biz_id '<业务ID>' --index_set_id '<索引集ID>' \
  --query_string 'ERROR OR WARN' --since 15m --limit 20
```

出现多个候选时，应结合业务名称、数据来源和采集范围选择，不把列表第一项当作默认答案。已知正确标识时可以直接查询，不必每次重复发现资源。

## 3. 典型排查场景

### 3.1 日志：从关键词定位到上下文

日志支持按索引集检索，也支持按已配置的场景检索。查询词、时间范围、返回条数和字段过滤都可以显式指定。

对于容器日志，要区分“选择哪些日志来源”和“筛选来源中的哪些记录”：

| 参数 | 用途 | 示例 |
| --- | --- | --- |
| `--scene` | 选择日志场景 | `--scene k8s`；场景须由目标服务支持 |
| `--where` | 按场景标签选择数据来源 | `--where 'cluster_id=<集群ID>'` |
| `--filter` | 对日志行做字段短语过滤 | `--filter '__ext.io_kubernetes_pod_namespace=checkout-prod'` |
| `--query_string` | 输入关键词或复杂查询表达式 | `--query_string 'ERROR OR WARN'` |

表中的集群、命名空间及字段名仅说明写法。实际场景和字段应从 `list_log_scenes`、`get_scene_log_fields` 或 `get_index_set_fields` 获取。多个 `--filter` 按 AND 组合，再与查询词组合；它们不是权限隔离规则。

需要查看一条命中日志前后的内容时，先保存该记录，再查询上下文：

```bash
bk-cli monitor log search_logs \
  --bk_biz_id '<业务ID>' --index_set_id '<索引集ID>' \
  --query_string ERROR --since 15m --limit 1 > hit.json

bk-cli monitor log search_index_set_context \
  --bk_biz_id '<业务ID>' --from hit.json --size 5
```

`hit.json` 必须来自成功且有命中的检索结果，并保留原始定位字段；不能用模拟记录、猜测的时间戳或其他业务的记录替代。`--size 5` 是每个方向的条数限制，不是总返回条数。

**日志结果可能包含业务内容。** 保存文件时使用授权的位置，不把它提交到代码仓库或公开文档。

### 3.2 指标：查看一段时间内的变化

指标支持 PromQL 范围查询，也提供指标组、字段目录与 SQL 查询入口。可以用于查看错误率、耗时、吞吐等变化，具体取决于业务已接入的指标。

```bash
# 查看可用的指标组；这不是所有监控数据的总目录
bk-cli monitor metric list_time_series_groups --bk_biz_id '<业务ID>'

# 查看选定指标表的字段
bk-cli monitor metric list_time_series_metrics \
  --bk_biz_id '<业务ID>' --table_id '<指标表ID>'

# 按已确认的表达式查询最近一小时，每分钟一个查询步长
bk-cli monitor metric execute_range_query \
  --bk_biz_id '<业务ID>' --promql '<PromQL表达式>' --since 1h --step 1m
```

CLI 不会根据表名自动生成或改写 PromQL。需要固定查询口径时，可由 SRE 在 V2 Profile 中维护具名查询；指标含义和计算方式仍需由业务确认。

### 3.3 告警：从告警详情继续查关联日志

告警列表和 TopN 查询限定在最终选择的业务中。列表返回的告警必须属于同一业务；归属不一致或无法确认时，CLI 会拒绝输出结果。后续详情使用该业务列表返回的告警 ID，不根据返回内容自动切换业务。

```bash
# 最近一小时仍处于异常状态的告警
bk-cli monitor alert list_alerts \
  --bk_biz_id '<业务ID>' --status ABNORMAL --since 1h

# 查看一条告警的详情
bk-cli monitor alert get_alert_info \
  --bk_biz_id '<业务ID>' --id '<告警ID>'

# 获取该告警关联的日志查询目标
bk-cli monitor alert get_alert_log_relations \
  --bk_biz_id '<业务ID>' --alert_id '<告警ID>'
```

关联查询返回的是日志目标和索引信息，不是日志正文。继续使用日志命令，并结合告警发生时间缩小查询范围。

### 3.4 调用链：从一次调用查看完整 Trace

APM（应用性能监控）提供应用、服务、Span、Trace 和性能剖析查询。Span 是一次调用记录，Trace 是由多个 Span 组成的调用链。

```bash
# 查看当前业务的 APM 应用
bk-cli monitor apm list_apm_applications --bk_biz_id '<业务ID>'

# 检索某个应用最近 15 分钟的调用记录
bk-cli monitor apm search_spans \
  --bk_biz_id '<业务ID>' --app_name '<应用名称>' --since 15m --limit 20

# 使用查询返回的 trace_id 查看调用链
bk-cli monitor apm get_trace_detail \
  --bk_biz_id '<业务ID>' --app_name '<应用名称>' --trace_id '<Trace ID>'
```

应用内检索不会自动限定到某个服务。需要服务级过滤时，应使用该工具支持的过滤条件，或使用团队已验证的 V2 工具预设；不能假定所有 APM 工具都接受同一个服务参数。

## 4. 时间范围：优先使用相对时间或可读日期

常用写法：

| 输入 | 含义 |
| --- | --- |
| `--since 15m` | 从本次执行时刻向前查询 15 分钟 |
| `--since 1h30m` | 最近 90 分钟 |
| `--start_time 'yesterday 10:00' --end_time 'yesterday 11:00'` | 昨天 10 点到 11 点 |
| `--start_time today --end_time now` | 今天零点到本次执行时刻 |
| 带 `Z` 或 `+08:00` 的 RFC3339 时间 | 明确时区偏移的绝对时刻 |

例如，按昨天的固定时段查询日志：

```bash
bk-cli monitor log search_logs \
  --bk_biz_id '<业务ID>' --index_set_id '<索引集ID>' \
  --start_time 'yesterday 10:00' --end_time 'yesterday 11:00' --limit 20
```

- 无时区偏移的日期默认按 **Asia/Shanghai（北京时间）** 解释，可用 `--timezone UTC` 等名称覆盖。
- `--since` 与显式起止时间二选一，包括 `--body` 中的时间；不会静默覆盖。
- `--since` 最大 24 小时，日志单次窗口也不超过 24 小时；其他查询还受服务端保留期和查询限制约束。
- 裸整数按具体工具的秒或毫秒单位解释，不按位数猜测。日期格式通常更容易核对。
- 资源关联批次中的嵌套时间、日志上下文定位字段不适用上述自动转换规则。

完整格式和边界见 [查询时间](references/time-inputs.md)。

## 5. 能力范围与命令发现

当前提供 **11 个业务模块、65 个只读工具**。下表是命令覆盖范围，不表示每个工具在所有部署中都可用；实际可用性还取决于服务版本、数据接入和当前用户权限。

| 模块 | 工具数 | 可以查询什么 |
| --- | ---: | --- |
| `log` | 9 | 日志场景、索引集、字段、日志、上下文及聚类结果 |
| `metric` | 4 | 指标组、字段、PromQL 和 SQL 查询 |
| `alert` | 12 | 告警、详情、事件和关联观测数据 |
| `apm` | 11 | APM 应用、服务、Span、Trace 和性能剖析 |
| `event` | 3 | 自定义事件和日志关键字事件 |
| `relation` | 2 | 资源关系的即时和时间段查询 |
| `space` | 2 | 业务空间和集群目录 |
| `dashboard` | 2 | 仪表盘目录及详情 |
| `alarm` | 8 | 告警策略、屏蔽、通知组及处理套餐配置，只读查看 |
| `collect` | 8 | 日志采集、索引集分组及清洗预览，只读查看 |
| `extract` | 4 | 日志提取范围及任务状态，不发起提取或下载 |

另外提供 `tools`、`permission`、`profile` 和 `status` 辅助命令，不计入上述 65 个工具。

不知道用哪个工具，或不确定参数时：

```bash
# 按业务和类别查看服务端工具目录
bk-cli monitor tools search --bk_biz_id '<业务ID>' --category log

# 从服务端读取某个工具的当前参数契约
bk-cli monitor tools schema --tool search_logs

# 查看本地命令参数、示例和所需权限
bk-cli monitor log search_logs --help
```

目录结果中的 `command` 表示已提供的 CLI 入口。服务端目录可能包含尚未封装的工具，不能据此推断 CLI 支持任意工具执行。

## 6. 权限与问题定位

### 有权限才可查询；缺权限时给出下一步

```bash
# 查看当前用户在业务下的日志权限状态
bk-cli monitor permission list --bk_biz_id '<业务ID>' --module log --missing_only

# 检查具体工具与索引集的权限，不读取日志正文
bk-cli monitor permission check --bk_biz_id '<业务ID>' \
  --tool get_index_set_fields --body '{"index_set_id":<索引集ID>}'

# 获取申请入口，不会自动提交申请
bk-cli monitor permission apply --bk_biz_id '<业务ID>' --tool search_logs \
  --body '{"target_type":"index_set","index_set_id":<索引集ID>}'
```

这里 JSON 中的索引集 ID 应替换为整数，不带引号。上述单工具 `permission check/apply` 需要服务端判定，不支持 `--dry-run`。

CLI 会根据服务端结果说明缺少哪类权限，并返回可用的申请链接。用户自行打开链接申请；预检通过和取得链接都不等于已获得授权。具体工具支持的 MCP 权限与原生权限路径以其帮助和服务端契约为准。

### 根据错误类型处理，而不是盲目重试

| 提示 | 应当怎么做 |
| --- | --- |
| `needs_resource` | 补齐索引集、应用等具体资源；这不等于无权 |
| `permission_missing` / `permission_denied` | 查看权限诊断和服务端申请指引 |
| `resource_not_visible` / `resource_not_found` | 从当前账号可见目录重新核对资源，必要时联系维护者 |
| `resource_blocked` | 联系维护者核查资源元数据，不应反复申请更多权限 |
| `profile_context_mismatch` / `profile_business_mismatch` | Profile 与显式范围冲突，确认配置后再查 |
| `needs_component` / `needs_binding` | 用 `profile show` 查看配置，再明确选择 |
| `response_scope_mismatch` | 列表业务归属与请求不一致或无法确认，核对 Profile、业务选择和服务端接口 |
| `invalid_response` | 接口响应缺少约定结构，联系维护者核对接口契约 |
| `api_not_found` | 核对 stage、网关前台路径、后端映射和服务路由/版本 |
| `auth_failed` | 在对应数据来源更新登录状态；不通过换账号或换来源掩盖错误 |

可用 `status` 检查当前状态，或先做离线检查：

```bash
bk-cli monitor status
bk-cli monitor status --offline
```

离线检查不会验证登录是否有效；`skipped` 表示未检查，`warn` 也不等于该项通过。带 `--profile` 的在线状态检查还会检查该配置的资源元数据。

## 7. 结果使用与产品边界

- **输出可供程序处理。** 命令返回结构化 JSON，包含成功状态和数据；错误带有错误码，适用时提供下一步命令。脚本应同时检查退出码及结果，不能只看是否输出了 JSON。
- **可先预览请求。** 在查询命令后加 `--dry-run`，检查实际业务、资源和时间范围，不发送查询；这不证明权限或查询表达式有效。带 `now` / `since` 的预览与执行会分别取时钟，精确复现时应固定两个时间边界。
- **空结果不是“没有故障”。** 它只表示本次范围与条件没有命中；CLI 不会自动扩大时间、换索引或切换数据来源。
- **只提供只读入口。** 当前不创建或修改告警策略、采集配置、屏蔽规则，不启动任务或提供文件导出下载。只读命令也不表示用户凭据本身只有读取权限。
- **不替代分析判断。** CLI 提供数据和可复用查询，不保证自动得出根因；指标口径、过滤条件和处置动作仍需结合业务确认。
- **不强制迁移。** 它可以与观测页面及已有 MCP 入口并行使用；共同遵守服务端权限，但不代表不同入口的登录状态和配置可直接互换。

建议从一个明确业务、一个短时间窗和一种数据开始，确认来源与结果后，再加入其他观测数据或自动化流程。

## 通用请求参数与输出

业务命令逐一调用对应的 HTTP API；GET 参数位于 query，POST 参数位于 JSON body。`--body` 接受完整业务参数对象、`@file` 或 stdin；同名具名参数覆盖对应字段。结构化参数按当前命令帮助传 JSON。

- `--stage` 选择网关 stage，默认 `prod`；context 选择部署，两者独立。
- `--header` 可重复，支持本次请求的租户和其他 header 覆盖；相同选项也用于权限与 Profile 预检的中间请求。认证规则和脱敏约定见共享 Skill。
- `--jq` 投影最终完整 envelope，包含显式 Profile 的来源信息，不在中间请求上执行。表达式语法错误在请求前失败；投影执行失败会保留完整原结果并返回非零，不重发查询。

```bash
bk-cli monitor log search_logs --profile checkout-prod --since 15m \
  --stage testing --header 'X-Example:probe' --jq '.data'

bk-cli monitor log search_logs --profile checkout-prod --since 15m \
  --dry-run --jq '{source:.profile.name,args:.request.body}'
```

Profile `check` 验证当前工具最终解析出的资源。工具预设覆盖公共 `table_id` 时，元数据检查必须指向覆盖后的表；`permission_only` 表示没有完成该资源的元数据验证，不能写成资源已验证或 SQL 已执行。
