---
name: bk-cli-shared
description: 当任务涉及 bk-cli 的通用使用规则时使用，尤其是认证、context 切换、stage/timeout、header/body、tenant、dry-run、verbose，或当用户在 `bk-cli api`、`apigateway`、`cmdb` 与后续 system 之间拿不准该怎么发请求、为什么请求失败、哪些输入是共享行为时，都应先使用这个 skill。
---

# bk-cli shared — 通用使用规则与排障入口

## 何时优先使用本 Skill

- 用户问的是认证、context、tenant、header、body、timeout、stage、dry-run、verbose 这类“所有系统都通用”的规则。
- 用户在排查请求失败，但问题还没明确落到某个具体系统命令上。
- 用户不确定应该先看系统专属 skill，还是先确认共享输入和运行时设置。
- 用户想先把“请求是怎么发出去的”看清楚，再决定要不要深入到某个系统命令。

## 推荐读取顺序

1. 先读本 skill，确认共享使用规则和排障顺序。
2. 再读系统专属 skill，补上具体命令、字段和示例。
3. 如果系统专属命令还没覆盖目标接口，再读 `bk-cli-api` 处理原始调用。

## 基础准备

```bash
# 初始化 context
bk-cli context init \
  --bk_api_url_tmpl="https://bkapi.your-domain.com/api/{gateway_name}/"

# 登录：app + user
bk-cli auth login --bk_app_code="your_app" --bk_app_secret="your_secret" --bk_token="your_token"
bk-cli auth login --bk_app_code="your_app" --bk_app_secret="your_secret" --bk_ticket="your_ticket"

# 登录：access_token
bk-cli auth login --access_token="your_token"

# 登录：个人 token（独立使用，不需要应用凭据）
bk-cli auth login --personal_token='<personal-token>'

# 环境自检
bk-cli doctor
bk-cli doctor --offline
```

## 通用 Flags

| Flag | 适用范围 | 说明 |
|------|----------|------|
| `--context <name>` | 全局 | 单次命令覆盖当前激活的 BlueKing context |
| `--stage <stage>` | API Gateway 请求 | 网关 stage，默认 `prod`，常见值为 `testing` |
| `--dry-run` | 请求类命令 | 只预览请求，不发起网络请求 |
| `--verbose` | 请求类命令 | 将请求/响应细节打印到 stderr |
| `--insecure` | 请求类命令 | 跳过 HTTPS 证书校验，仅用于临时调试 |
| `--header 'Key:Value'` | 请求类命令，可重复 | 传入额外请求头；header name 和 value 会在本地校验 |
| `--body '<json>'` | 支持 body 的请求类命令 | 传入 JSON body，CLI 管理 `Content-Type: application/json` |
| `--timeout <duration>` | `bk-cli api` | 单次覆盖 context timeout，例如 `180s` |

## 多 Context 管理

context 表示一个独立的 BlueKing 部署目标，不是 API Gateway stage；`prod`、`testing` 这类环境阶段应通过 `--stage` 控制。

```bash
# 创建不同部署的 context，并设置该 context 的默认 timeout / tenant
bk-cli context create clouds \
  --bk_api_url_tmpl="https://bkapi.clouds.example.com/api/{gateway_name}/" \
  --tenant_id="tenant-a" \
  --timeout 120s

bk-cli context create devops \
  --bk_api_url_tmpl="https://bkapi.devops.example.com/api/{gateway_name}/"

# 查看、切换、单次覆盖 context
bk-cli context list
bk-cli context use clouds
bk-cli api bk-iam GET /api/v2/systems/ --context devops
```

- 每个 context 都维护自己的一套凭据和默认项。
- `bk-cli auth login` 会把凭据写入当前 active context，或写入 `--context <name>` 指定的 context。
- 命令执行时 context 来源优先级是：显式 `--context`，当前 active context，本地已有 context 的 fallback。
- 显式指定不存在的 `--context` 会直接报错，不会静默回退。
- context 名称必须匹配 `^[a-z][a-z0-9-]*$`，例如 `default`、`clouds`、`prod-1`。
- tenant 是 context 的默认值；单次覆盖租户时使用 `--header 'X-Bk-Tenant-Id:<value>'`。

## 通用请求规则

- stdout 输出结构化 JSON envelope；CLI 级错误和 verbose 信息输出到 stderr。
- `--dry-run` 适合先确认 URL、query、body、headers 是否正确。
- `--verbose` 对请求类命令生效，请求/响应细节会输出到 stderr。
- `--insecure` 类似 `curl --insecure`，只影响真实 HTTPS 请求；临时调试自签名证书或测试环境时再使用。
- `--header` 必须包含 `:`，且 header name 不能为空；非法 header 不会进入网络请求阶段。
- `--header` 可以显式覆盖同名请求头，名称匹配不区分大小写；旧凭据模式可覆盖 `X-Bkapi-Authorization`，个人 token 模式可覆盖 `Authorization`，也可覆盖 `X-Bk-Tenant-Id`。
- 个人 token 模式下，原始 `api` 和要求认证的系统命令不能再叠加 `--header 'X-Bkapi-Authorization:<json>'`，否则会在本地报错。
- 传入 `--body` 时仍不能手动覆盖 `Content-Type`，该 header 由 CLI 管理。
- dry-run / verbose 会对 `Authorization` 和 `X-Bkapi-Authorization` 做脱敏显示，即使该 header 是调用方显式覆盖的；响应 envelope 的这两个 header 也会脱敏。
- 路径占位符值会被当作单个 URL path segment 转义，不能依赖它注入 `/`、`?` 等路径结构。

## 超时规则

- 每个 context 都有默认 `timeout`，未设置时为 `60s`。
- `bk-cli api` 默认使用当前 context 的 timeout；显式传 `--timeout <duration>` 时，本次请求优先使用该值。
- 某些系统命令可能会在自己的帮助信息或系统专属 skill 中说明额外的超时行为；遇到这类命令时，以该命令帮助和系统 skill 为准。

## 认证与登录

多数情况下，你不需要关心 CLI 内部如何组装认证头，只需要确保目标 context 已经登录到匹配的身份。

当前 bk-cli 公开支持四种登录形态：

- `bk-cli auth login --bk_app_code=X --bk_app_secret=Y --bk_token=Z`
- `bk-cli auth login --bk_app_code=X --bk_app_secret=Y --bk_ticket=Z`
- `bk-cli auth login --access_token=Z`
- `bk-cli auth login --personal_token='<personal-token>'`

个人 token 来自个人中心。`--personal_token` 与 `--bk_app_code`、`--bk_app_secret`、`--bk_token`、`--bk_ticket`、`--access_token` 互斥，其他凭据 flag 即使显式传空值也不能混用。token 不应带 `Bearer ` 前缀，不能是空值或包含空白、控制字符；非法输入不会替换已有凭据，也不会在错误中回显原值。同一 context 再次成功登录会替换原凭据，凭据仍加密保存。

使用个人 token 登录后，原始 `api` 和要求认证的系统命令使用 `Authorization: Bearer <personal-token>`，无需额外提供应用或用户凭据；明确无需认证的系统命令不会自动携带 token。部署是否支持个人 token、token 有效期和接口权限由服务端判定。

排查认证问题时，优先记住下面几点：

- `bk-cli auth status` 适合做“当前 context 有没有凭据”的检查。
- `bk-cli auth status` 在“没有凭据”时仍返回 `ok: true`，要看 `data.has_credentials` 是否为 `true`。
- `bk-cli auth check` 更适合脚本和 CI；目标 context 没有凭据时，会返回 `ok: false`、`error.code: no_credentials`，并以 exit code 1 结束。
- 个人 token 登录后，`auth status/check` 显示 `credential_type: personal_token`，不显示 token；`doctor --offline` 会检查凭据格式并显示脱敏摘要。这些本地检查不验证远端有效期或权限。
- 如果你需要临时覆盖 CLI 自动生成的认证头，旧凭据模式使用 `--header 'X-Bkapi-Authorization:<json>'`，个人 token 模式使用 `--header 'Authorization:Bearer <personal-token>'`；dry-run / verbose 仍会做脱敏显示。

## 与系统专属 Skill 的关系

- `bk-cli-api` 补充原始 API 调用的 URL 构造、`--query`、`--path` 和输出解析细节。
- 系统专属 skill 只补充该系统自己的命令语义、业务字段和示例。
- 本 skill 只负责共享使用规则、公共输入约定和排障入口，不解释命令是如何在仓库里实现出来的。
- 如果你是在扩展命令、维护 skill，或修改共享请求契约，请切换到 `AGENTS.md` 和 `docs/design.md`。

## API 调试参考（api-debug）

- 当用户已经拿到 API Gateway 的 HTTP status、`code_name`、错误消息，或需要快速判断错误是网关返回还是后端返回时，继续读取 `references/api-debug.md`。
- 该参考文档按 `status -> message/code_name -> 原因 -> 处理` 组织，适合 agent 先做快速定位，再补充上下文说明。
- 优先用它处理 400、403、404、429、499、502、504、508 这类常见网关错误；如果响应体不符合网关错误协议，也要先看它里面的“如何判断是不是网关返回”。

## 排障顺序建议

1. 先运行 `bk-cli doctor --offline` 确认本地 context、当前选中项、凭据摘要和 URL 模板渲染结果。
2. 如果需要确认网关连通性，再运行 `bk-cli doctor`。
3. 再确认 stage、tenant、timeout、header/body、TLS 证书输入是否符合共享规则。
4. 然后用 `--dry-run` 看最终请求构造是否符合预期。
5. 仍有问题时再看系统专属 skill 或 `--verbose` 输出，定位到具体命令或接口层面。
