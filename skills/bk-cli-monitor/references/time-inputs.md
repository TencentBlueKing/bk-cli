# 查询时间：相对窗口、日期时间与时间戳

只有工具契约声明的顶层 `start_time` / `end_time` 才使用这里的转换。时间来自执行 CLI 的机器时钟，不由 Agent 编造 epoch；Profile 不固定查询日期。

## 两种输入模式

- 最近一段时间：`--since 15m`、`--since 1h30m`，结束点是本次执行时刻，最大 24h。
- 明确时间边界：`--start_time VALUE --end_time VALUE`。支持日期时间、RFC3339、日历关键词、整型时间戳。

两种模式互斥，包括 `--body` 中的时间字段；不会用 since 悄悄覆盖已有边界。显式命名 start/end 仍覆盖 body 中的同名字段。

## 可直接输入的格式

| 写法 | 含义 |
|---|---|
| `2026-10-13 14:00:00` / `2026-10-13 14:00` | 所选时区的日期时间 |
| `2026-10-13T14:00:00` | 无偏移的 ISO 本地时间，仍使用所选时区 |
| `2026-10-13` | 所选时区当天 00:00:00 |
| `2026-10-13T14:00:00+08:00` / `2026-10-13T06:00:00Z` | 明确的绝对时刻；字符串中的偏移优先于 timezone |
| `today` / `今天` | 所选时区今天 00:00 |
| `yesterday` / `昨天` | 所选时区昨天 00:00，不是现在减 24h |
| `yesterday 10:00` / `昨天 10:00:00` | 指定日历日的时刻 |
| `now` / `现在` | 本次 CLI 取到的实际时钟，按工具精度落到整秒/整毫秒 |
| `1791871200` | 原生单位的 Unix 整数；本例用于秒接口 |
| `1791871200s` / `1791871200000ms` | 显式单位，同一个绝对时刻；CLI 转成目标工具的单位 |

默认时区固定为 **Asia/Shanghai（北京时间）**，不依赖电脑的本地时区。用 `--timezone UTC` 或其他 IANA 名称覆盖。不支持含糊的 `Local`。

`--since` 是经过的时长，Unix 时间戳本身也是绝对时刻，时区不改变它们。时区只影响无偏移的日期和日历关键词。

## 实际命令

以下在已配置的 `game-production` Profile 下执行；日期示例仅演示格式，查询前注意数据保留期。

```bash
# 最近 90 分钟
bk-cli monitor log search_logs --profile game-production --component worker --since 1h30m --limit 3

# 昨天 10 点到 11 点，默认北京时间
bk-cli monitor log search_logs --profile game-production --component worker \
  --start_time 'yesterday 10:00' --end_time 'yesterday 11:00' --limit 3

# 固定日期时间
bk-cli monitor log search_logs --profile game-production --component worker \
  --start_time '2026-10-13 14:00:00' --end_time '2026-10-13 15:00:00' --limit 3

# 明确使用 UTC；也可直接传带 Z 或 +08:00 的 RFC3339 字符串
bk-cli monitor metric execute_range_query --profile game-production --component backend \
  --binding latency --start_time '2026-10-13 06:00' --end_time '2026-10-13 07:00' --timezone UTC

# 时间戳，显式写单位最不容易用错
bk-cli monitor log search_logs --profile game-production --component worker \
  --start_time 1791871200s --end_time 1791874800s --limit 3

# 今天零点至今；不是滚动 24 小时
bk-cli monitor apm search_spans --profile game-production --component backend \
  --start_time today --end_time now --limit 3
```

`--body` 内的顶层时间也会转换；timezone 只作为 CLI flag，不放进 API body：

```bash
bk-cli monitor log search_logs --profile game-production --component worker \
  --body '{"start_time":"昨天 10:00","end_time":"昨天 11:00","limit":3}'
```

先加 `--dry-run` 查看 `request.body.tool_args` 中最终发送的 Unix 值和 JSON 类型。使用 now/since 的预览和之后的执行会各自取时钟；需要完全复现同一窗口时，固定两个绝对边界。

## 秒、毫秒与 JSON 类型

- **秒，字符串**：日志检索/字段分析、PromQL、告警、事件、Span 检索、APM calculate_by_range 等顶层时间参数。
- **毫秒，字符串**：`metric execute_sql_query`、`log search_log_clustering_pattern`。
- **秒，整数**：`apm get_profile_label`、`get_profile_type`、`query_graph_profile`。CLI 对日期、since 和 body 都按 Schema 生成整数，不再发送字符串。
- `alert get_alert_event_tag_detail` 只有顶层 start_time，支持上述输入，但没有 since/end_time。

裸整数按工具原生单位解释，**不按数字位数猜秒/毫秒**。例如 SQL 的 `1791871200` 会被解释成毫秒；想输入秒请加 `s` 后缀或使用日期格式。使用毫秒接口时，日期可精确到毫秒；秒接口不接受非整秒值，不静默舍入。纳秒时间戳和小数形式的 Unix 时间戳不在本地解析范围内。

## 例外和错误边界

- `relation find_relations` 的时间在 `query_list[].timestamp`；`find_relations_range` 在 `query_list[].start_time/end_time`。它们仍按 Schema 传整数秒，不支持本地 since、timezone 或嵌套自然时间转换。
- `search_index_set_context --from` 是读取原始日志定位记录，不是“开始时间”。`dtEventTimeStamp` 等定位字段原样保留，不在本次时间转换范围内。
- 自然关键词仅支持上述明确写法。不解析“昨天下午”“上周五”“一小时前”；单独的 `10:00` 也不猜日期。最近一小时使用 since，具体时刻写完整日期或日历关键词。
- since 不支持 `1d` / `1w`，一天用 `24h`。时间精度也必须符合工具单位，例如秒接口不能用 `--since 0.1s`。
- start 必须早于 end；没有填齐必需边界时不会猜另一个边界。日志单次窗口最大 24h，其他工具的跨度、保留期和查询量仍由各服务限制，不保证任意历史窗口都有数据。
- 夏令时切换时，不存在或重复的当地时刻会报错，要求显式 RFC3339 偏移。自然日可能是 23/25 小时，因此 `yesterday → today` 不总等于 since 24h；超过日志窗口上限时需自行拆分。
- 时间范围末端是否包含某个数据点以具体工具为准；CLI 不统一改成半开区间，也不自动加减 1 秒。跨窗口汇总时按实际接口规则去重。
- 非法时区、时间格式、精度丢失、反向区间和 since 冲突均在发请求前失败；不会因查不到数据而自动扩大时间或切换索引。
