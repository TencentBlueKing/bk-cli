# 用 Profile 交付业务观测能力

Profile 是 SRE 维护的具名资源配置：研发和 Agent 选择一个名字，CLI 确定性地补齐业务、日志来源、指标表和 APM 应用。不再每次让 Agent 查询索引列表、猜关联关系。它不是凭据、IAM 授权或自动记忆。

## 研发：拿到文件后怎样使用

先按共享 Skill 配好 SRE 指定名称的 bk-cli context，并用自己的账号登录。Profile 只引用 context 名称，不创建 context、不保存 Token，也不修改当前激活环境。

```bash
export BK_CLI_MONITOR_PROFILE_FILE=./team-profiles.yaml
bk-cli monitor profile list
bk-cli monitor profile show --profile checkout-prod
bk-cli monitor status --profile checkout-prod
bk-cli monitor log search_logs --profile checkout-prod --since 15m --query_string ERROR --limit 5
bk-cli monitor metric list_time_series_metrics --profile checkout-prod
```

业务首次接入或配置更新后运行 `status`/`profile check`；普通查询无需每次预检。请求仍由服务端实时鉴权，预检结果不是缓存凭据。

只检查权限、不读取资源字段目录：

```bash
bk-cli monitor permission check --profile checkout-prod --tool search_logs
bk-cli monitor permission apply --profile checkout-prod --tool search_logs
```

`apply` 只返回 IAM 链接，不提交申请。模块级 `permission list/apply` 仍是业务级判断，不枚举 Profile 中每一个资源；检查具体资源用 `--tool`。

## V2：业务组件和多个数据来源

异构业务使用 `version: 2`：顶层 `sources` 引用已有 context + 业务 ID；Profile 下按 `components` 组织业务组件，每个组件用具名 `bindings` 绑定一个 module 和 source。

```bash
bk-cli monitor profile show --profile game-production --component backend
bk-cli monitor log search_logs --profile game-production --component backend --since 15m
bk-cli monitor metric execute_range_query --profile game-production --component backend --binding traffic --since 1h
```

多组件必须指定 `--component`，同 module 有多个绑定必须指定 `--binding`；缺失的能力不回退到其他组件或默认环境。Source 冲突依旧停止，不修改 active context。

- binding 的 `args` 只放跨工具含义稳定的资源字段，如索引集、应用、指标表、仪表盘 UID。
- 其他固定参数放在 `tools.<精确工具名>`；不得引用写入/导出工具、未知参数或预填时间/关联批次。优先级：显式 flag > body > 精确工具预设 > 公共资源参数。
- `options.scene/where/filters` 仅供日志绑定使用；不要把日志的过滤语法套给 APM、事件或告警。
- `status/check` 分 Source 和 binding 报告；`permission_only` 没有执行数据查询，`metadata` 也不证明每条表达式有效。一次最多预检 20 个工具，可用组件/绑定缩小范围。
- V2 诊断的 `--dry-run` 只输出本地计划。`list/show` 不要求已登录；离线状态不证明 Token 有效。

示例见 [profiles-v2.yaml](profiles-v2.yaml)，其中标识全是示例，必须由 SRE 按实时目录替换。V1 继续兼容，适合简单单来源。

## SRE：V1 文件格式与分发

读取随 CLI 打包的示例（标识均为示例，必须替换）：

```bash
bk-cli skills read bk-cli-monitor references/profiles.yaml --raw > team-profiles.yaml
```

V1 文件为 `version: 1` 和 `profiles` 名称映射。完整示例见同目录 `profiles.yaml`。每个 Profile 包含：

| 字段 | 用途 |
|---|---|
| `description` | 可选，描述业务对象、场景和使用边界 |
| `context` | 必填，本机已配置的 bk-cli 部署名称；不是网关 stage |
| `bk_biz_id` | 必填，非零整数；负数空间 ID 也支持，建议写成字符串 |
| `log.index_set_id` | 固定索引集，与 `log.scene` 二选一 |
| `log.conditions` | 可选，索引集 `search_logs` 的结构化过滤对象；与查询词按上游规则组合 |
| `log.scene` | 场景标识，从 `list_log_scenes` 获取 |
| `log.where` | 选表路由标签，如 `cluster_id`；不是日志行过滤，不把 namespace 放在这里 |
| `log.filters` | `search_logs` 的日志行字段短语过滤，按 AND 组合；namespace 等字段从日志字段目录获得 |
| `metric.table_id` | 只补给接受 `table_id` 的指标字段/SQL 工具 |
| `apm.app_name` | APM 应用，补给接受该字段的 APM 工具 |
| `apm.service_name` | 可选，只补给有 `service_name` 或 `service_names` 参数的工具 |

同业务不同 namespace 用不同的 Profile 名称。需要复用配置时可使用 YAML 原生锚点和 `<<`；合并是浅层的，覆盖整个 `log` 块时需写全该块，不引入自定义继承、模板、远程 include 或脚本。

文件查找顺序：显式 `--profile_file` → `BK_CLI_MONITOR_PROFILE_FILE` → `${BK_CLI_CONFIG_DIR:-~/.bk-cli}/monitor/profiles.yaml`。必须显式选择 `--profile NAME`；没有“上次使用”或默认 Profile。文件最大 64 KiB、1–100 个 Profile、单个 YAML 文档；未知字段、重复键和混合日志模式会报错。

SRE 按业务仓库或内部配置渠道维护文件，业务研发只用自己的凭据。可以共享 Profile 文件和业务 Skill，**不能共享 `credentials.enc` 或整个 CLI 凭据目录**。真实业务 ID、内网地址和应用名称不写入开源示例。

## 覆盖和安全边界

- 参数只补给声明了该字段的工具；不会把日志 `conditions` 错塞到告警过滤中。
- 普通参数优先级：显式 flag > `--body` > Profile。`conditions` 或 `table_id_conditions` 是整体替换，不尝试深合并布尔表达式。
- 场景 `--where KEY=VALUE` 按 key 覆盖选表路由，不能与显式 `table_id_conditions` 混用。日志行过滤用 `log.filters` 或可重复的 `--filter FIELD=VALUE`；显式 filter 按 key 覆盖 Profile 并保留其他字段，最终与 `query_string` 按 AND 组合。
- `filters` 的值会转义为字段短语；文本字段的匹配行为仍由服务端字段映射决定。复杂正则、范围或 OR 条件用 `--query_string`，CLI 不推断查询语言。
- `--context`、`bk_biz_id`、日志模式或 `scene` 与 Profile 冲突会停止，避免在另一环境/业务复用同一个数字 ID。需要改这些范围时换 Profile 或不带 `--profile`。
- 显式 `index_set_id`、`table_id` 等可覆盖默认值；Profile 是默认参数，不是不可绕过的访问控制。限制访问必须由服务端 IAM 实现。
- `metric.table_id` **不改写 PromQL**。不同指标有不同的标签语义，namespace 标签和查询表达式仍须显式给出。
- 没有 `service_name`/`service_names` 参数的 APM 工具不会自动限定服务。`log.conditions` 和 `log.filters` 只作用于 `search_logs`（conditions 仅索引集模式）；不自动转写聚类、分析、字段目录或上下文工具的过滤格式。
- Profile 不能证明资源仍存在，查询为空也不会自动换索引。新配置通过预检后，SRE 仍需执行一个小时间窗的真实查询验收。

使用 Profile 的输出会附加顶层 `profile` 字段，含文件路径、SHA-256、绑定 context、业务、实际补齐字段和 `resolved_parameters`（最终资源参数）；原有 `data` 形状不变，`--from` 仍能读取检索结果。使用 `--dry-run` 查看最终请求体，确认显式覆盖后的范围。

## 自检与排障

```bash
bk-cli monitor profile check --profile checkout-prod
bk-cli monitor status --profile checkout-prod
bk-cli monitor status --profile checkout-prod --offline
bk-cli monitor status --check_update
```

- `profile check`：先执行工具契约/权限预检，再读取索引字段、场景字段、指标字段或 APM 服务目录。不会读取日志正文、告警内容或指标时序；也不证明每条过滤表达式、namespace 值和所有工具都有效。
- `status`：显示版本、本地 context、凭据文件；在线时发一次鉴权请求、对比内置/服务端目录版本。带 Profile 时再做上述资源检查。
- `--offline` 或 `status --dry-run`：完全不联网，不解密凭据；`skipped` 表示没有验证，不能说 Token 有效。
- `--check_update`：可选查询官方 GitHub Release，不下载、不安装；发布源不可达、预览版本不可比或目录版本不一致都是 `warn`，不伪装成“最新”。不检查 npm 发布渠道。
- 检查失败会保留 stdout 诊断报告并非零退出；`warn` 不导致失败。不要只看顶层 `ok`，也要看每条检查是 `pass`、`warn` 还是 `skipped`。

`profile_not_found` 先查 `profile list`；`profile_context_mismatch` / `profile_business_mismatch` 不要靠自动切换绕过；资源不可见请让 SRE 核对配置；`permission_missing` 用 `permission apply`；`auth_failed` 在绑定 context 下重新登录。

## FDE 交付清单

SRE 面向业务定制的最小交付是：一个已核对资源的 Profile 文件、一段场景 Skill，以及真实查询验收记录。Skill 应写业务工作流，不再复制 ID 关联表。例如：

```text
使用 bk-cli-monitor Skill；Profile 文件使用团队提供的 team-profiles.yaml。
排查生产结算服务时选 --profile checkout-prod，从最近 15 分钟日志与告警开始。
缺权限按 CLI 指引申请；配置失效联系 SRE，不猜索引、不把空结果当无故障。
```

交付前：确认 context 指向的数据来源（可能不等于故障所在环境）；核对业务和资源目录；运行 `profile check` 与小窗口查询；换成研发本人的账号核验权限；确认文件没有凭据。CLI/Skill 可共用，Profile 随业务维护，不为每个业务复制一套 CLI。
