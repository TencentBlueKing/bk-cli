# monitor Profile 初始化

本文供人工准备观测配置；查询、权限诊断和排障由 [monitor Skill](../skills/bk-cli-monitor/SKILL.md) 提供。使用前须安装包含 `monitor` 的构建，并按 [共享使用规则](../skills/bk-cli-shared/SKILL.md) 配置 context、使用自己的身份登录。

## 1. 准备资源

由 SRE 或业务维护者确认要使用的 context、业务、日志索引集、指标表和 APM 应用。资源 ID 来自当前账号可见的目录；不要把示例标识当成真实资源。

Profile 只保存资源默认值，不保存凭据、不提供授权，也不会修改当前激活的 context。共享 Profile 不代表共享权限，不共享凭据目录。

## 2. 单来源配置

复制 [V1 模板](../skills/bk-cli-monitor/references/profiles.yaml)，按模板替换 context 和已确认的资源。配置结构示例：

```yaml
version: 1
profiles:
  checkout-prod:
    context: observability
    bk_biz_id: "100100"
    log:
      index_set_id: 101001
    metric:
      table_id: example.request_metrics
    apm:
      app_name: example-app
```

这里的 context、业务和资源均为虚构示例。配置中的 context 必须已存在且已登录。

```bash
export BK_CLI_MONITOR_PROFILE_FILE=./team-profiles.yaml
bk-cli monitor profile list
bk-cli monitor profile show --profile checkout-prod
bk-cli monitor profile check --profile checkout-prod
bk-cli monitor status --profile checkout-prod
```

也可在单次命令中指定 `--profile_file ./team-profiles.yaml`。离线查看配置不证明认证或权限有效；在线检查也不读取日志正文或执行预设查询。

## 3. 多组件与多来源

当日志、指标、Trace 位于不同 context 或业务，使用 [V2 模板](../skills/bk-cli-monitor/references/profiles-v2.yaml)。V2 分为：

| 字段 | 用途 |
| --- | --- |
| `sources` | 引用已有 context 与业务，不填写 URL 或凭据 |
| `components` | 描述后端、Worker 等被观测组件 |
| `bindings` | 为模块配置资源及具名查询，引用一个 source |
| `args` | 该绑定的公共资源默认值 |
| `tools` | 精确工具的预设，覆盖同名公共默认值 |

```bash
bk-cli monitor profile show --profile_file ./multi-source-profiles.yaml \
  --profile game-production --component backend

bk-cli monitor profile check --profile_file ./multi-source-profiles.yaml \
  --profile game-production
```

多个组件或绑定有歧义时，明确指定 `--component` / `--binding`。显式 context 或业务与 Profile 冲突时停止，不静默切换来源。完整格式、覆盖规则和交付步骤见 [Profile 使用与交付](../skills/bk-cli-monitor/references/profiles.md)。

## 4. 判断检查结果

- `metadata`：检查了该工具最终解析资源的元数据；不证明所有查询表达式有效。
- `permission_only`：只完成权限预检，未完成资源元数据验证。
- `fail`：按结果中的错误及下一步处理；不要扩大权限或换来源掩盖错误。
- `warn` / `skipped`：分别表示警告或未检查，不等于通过。

预检不会执行 SQL、PromQL 或 Span 检索。工具级预设覆盖表或索引时，检查的必须是覆盖后的同一资源。

## 5. 查询与维护

首次交付或配置变更后检查资源；正常查询不需要每次先预检，服务端会在执行时重新鉴权。通用 `--stage`、`--header` 同样作用于中间检查请求；使用 `--jq` 时可读取最终结果及 Profile 来源信息。

日期、RFC3339、相对时间和单位转换见 [查询时间](../skills/bk-cli-monitor/references/time-inputs.md)。Profile 不固定运行时查询窗口，不支持脚本、插值或远程 include。保存的查询结果可能含业务内容，不提交到源码仓库或公开文档。

模块能力、完整查询示例和错误处理统一见 [monitor Skill](../skills/bk-cli-monitor/SKILL.md)。
