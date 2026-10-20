---
name: bk-cli-apigateway
description: 当需要通过 `bk-cli apigateway` 发现 BlueKing API Gateway 中所有公开网关、浏览某个网关公开的 API/资源列表、获取 OpenAPI/Schema、判断某个接口该怎么调用、申请网关或批量 API 权限，或在 `apigateway` 子命令与 `bk-cli api` 之间做选择时使用。尤其是当用户还不知道或不确定该调用哪个网关、哪个公开接口、接口名可能是什么、只知道业务目标/关键词/资源名片段，或遇到网关资源权限不足时，也应优先使用这个 skill，先搜索再确认，而不是直接猜测接口。
---

# bk-cli apigateway — API Gateway 管理

用于发现网关、浏览网关 API、获取 OpenAPI Schema，以及在用户明确确认后申请网关或批量 API 权限。

CRITICAL — 开始前 MUST 先用 Read 工具读取 `../bk-cli-shared/SKILL.md`。共享 skill 负责认证、context、tenant、stage、header/body、dry-run、verbose 等通用使用规则；本 skill 只补充 `apigateway` 命令自己的探索、调用与权限申请路径。

## 何时优先使用本 Skill

- 用户只知道“我想做什么”，但不知道应该调用哪个公开网关或公开接口。
- 用户还不知道当前有哪些网关，或只知道模糊名称，需要先筛选、搜索和浏览。
- 用户知道网关名，但还不知道资源名、API 名或入参，希望先查看资源列表与 Schema。
- 用户在决定“这里该用现成 `apigateway` 子命令，还是退回 `bk-cli api`”。
- 用户要把 API Gateway 发现结果接到脚本或 `jq` 中继续处理。
- 用户遇到网关资源权限不足，或明确要求申请某个网关、多个 API 的调用权限。

## 推荐工作流

1. 先 `list_gateways` 搜索并确认可能相关的公开网关。
2. 再用 `list_gateway_apis` 浏览该网关公开资源，缩小到候选 API。
3. 如果结果不唯一，向用户展示候选网关/API，并明确说明你为什么认为它们相关，请用户确认最终目标。
4. 用 `retrieve_gateway_api_details` 查看完整 Schema、认证要求、方法、路径、请求/响应结构。
5. 如果目标已经有合适的 `apigateway` 子命令，就继续使用该命令；权限申请按下文 `apply_gateway_permission` 的确认流程执行。
6. 如果需要真正调用某个公开 API，或目标接口尚未映射成 `apigateway` 子命令，就根据详情里的协议信息切换到 `bk-cli api` 做原始调用。

## Agent 工作方式

### 发现优先，不要猜接口

- 当用户不知道网关名、API 名、路径或入参时，不要直接假设接口。
- 先使用 `list_gateways` 和 `list_gateway_apis` 做搜索，把“我猜你要这个接口”变成“这里有几组可能匹配的公开接口”。
- 如果有多个候选，先给用户一个短名单，再继续下一步，不要跳过确认环节。

### 候选确认方式

给用户候选时，尽量包含这些信息：

- `gateway_name`
- `api_name`
- 简短匹配理由，例如“名字包含 system”“描述里提到了权限”“路径看起来对应创建应用”

推荐话术是“我找到了 2 个可能相关的公开接口，请确认你要的是哪一个”，而不是“我直接替你调用最像的那个”。

### 从发现切换到调用

- `bk-cli apigateway` 负责“找网关、找接口、看详情、看 Schema”，也提供需用户明确确认的 `apply_gateway_permission` 权限申请命令。
- `bk-cli api` 负责“按真实 method/path/query/body/header 发请求”。
- 只有在看过 `retrieve_gateway_api_details` 的结果之后，才去组装 `bk-cli api` 调用更稳妥。
- 首次调用、排障、或用户自己也不确定参数时，优先先生成 `bk-cli api ... --dry-run`，确认 URL、headers、body 后再真正发请求。

### 组装 `bk-cli api` 的原则

从 API 详情 / Schema 中提取这些信息：

- 网关名：对应 `bk-cli api <gateway_name>`
- HTTP 方法：如 `GET`、`POST`
- 路径：填到 `api_path`
- query 参数：放入 `--query '<json>'`
- path 参数：如果路径模板里有占位符，用 `--path '<json>'`，否则直接写入具体路径
- body：放入 `--body '<json>'`
- 额外 header：仅在接口详情确实要求时再用 `--header`

如果 detail/Schema 不能唯一说明如何调用，就先把不确定点告诉用户，再确认，不要把缺失信息默默补成猜测值。

### 读取 `auth_config` 做调用前检查

`retrieve_gateway_api_details` 的返回里如果包含：

```json
{
  "auth_config": {
    "user_verified_required": false,
    "app_verified_required": true,
    "resource_perm_required": true
  }
}
```

应把它理解成接口调用前置条件，而不只是展示字段：

- `app_verified_required: true`：调用方需要带应用身份。
- `user_verified_required: true`：调用方需要带用户身份。
- `resource_perm_required: true`：除了有应用身份外，应用通常还需要先申请该 API 的资源权限。

如果当前 context 使用个人 token，CLI 使用 `Authorization: Bearer <personal-token>`，无需补充应用或用户凭据。应结合目标部署对个人 token 的支持情况和服务端返回，判断 token 是否有效及是否具有接口权限；不能再叠加 `X-Bkapi-Authorization`。例外：下文 `apply_gateway_permission` 需要应用身份，不支持个人 token。共享登录、互斥和脱敏规则见 `../bk-cli-shared/SKILL.md`。

对使用 app+user 或 `access_token` 的 agent 来说，推荐这样解读：

- 仅 `app_verified_required: true`：优先确认当前登录票据里有应用身份，或使用可满足要求的 `access_token`。
- 同时 `app_verified_required: true` 且 `user_verified_required: true`：优先确认当前登录票据同时具备应用身份和用户身份，或使用可满足要求的 `access_token`。
- `resource_perm_required: true`：即使认证字段齐全，也不能排除“应用尚未申请该 API 权限”。

### 失败时的诊断提示

如果后续 `bk-cli api` 调用失败，应该结合 `retrieve_gateway_api_details` 的 `auth_config` 一起解释错误，而不是只把原始报错丢给用户。

优先按下面的思路提示：

- 当前使用个人 token 时，先检查部署是否支持 Bearer 鉴权、token 是否有效及是否有该接口权限；本地 `auth check` 成功只说明凭据已保存。不要仅因接口要求应用或用户认证就要求追加旧凭据或 `X-Bkapi-Authorization`。
- 使用 app+user 或 `access_token` 时，当接口要求应用认证，但当前登录方式没有应用身份，提示用户检查 `bk_app_code` / `bk_app_secret`，或改用满足条件的 `access_token`。
- 使用 app+user 或 `access_token` 时，当接口要求用户认证，但当前登录方式没有用户身份，提示用户检查 `bk_token` / `bk_ticket`，或改用满足条件的 `access_token`。
- 当接口要求资源权限，且报错表现像“无权限访问 / permission denied / forbidden”时，按当前登录模式检查个人 token 的接口权限，或应用是否已申请该 API 权限。
- 使用 app+user 或 `access_token` 时，当接口既要求应用身份又要求资源权限，不要只给单一结论；应同时告诉用户检查“登录身份是否完整”以及“应用是否已申请 API 权限”。
- 确认缺少网关资源权限后，可按下文 `apply_gateway_permission` 发起申请；业务系统自身的权限不足需按对应系统流程处理，不要一遇到权限错误就自动提交申请。

推荐输出风格：

- 先说明该接口从 `auth_config` 看需要什么。
- 再说明当前报错更像是“缺身份”还是“缺 API 权限”。
- 如果无法仅凭错误唯一判断，就把两种最可能原因按优先级列出来，不要伪装成确定结论。

## 快速开始

```bash
# 1. 认证
bk-cli auth login --bk_app_code="your_app" --bk_app_secret="your_secret" --bk_token="your_token"

# 或在支持个人 token 的部署中独立登录
bk-cli auth login --personal_token='<personal-token>'

# 2. 列出网关
bk-cli apigateway list_gateways

# 3. 列出某个网关的 API
bk-cli apigateway list_gateway_apis --gateway_name bk-iam

# 4. 获取 API 详情 / Schema
bk-cli apigateway retrieve_gateway_api_details --gateway_name bk-iam --api_name create_system
```

## 面向“不确定接口”的探索流程

当用户只给出业务目标，例如“我想找蓝鲸里公开的权限系统接口”“我不知道该调哪个网关，只知道想查应用信息”，优先按下面的顺序做：

1. 用关键词、名称或模糊匹配搜索公开网关。
2. 进入最相关的一个或几个网关，列出公开 API。
3. 再用关键词缩小到候选接口。
4. 把候选接口返回给用户确认。
5. 对确认后的接口读取详情/Schema。
6. 根据详情构造 `bk-cli api` 调用。

示例：

```bash
# 先找可能相关的公开网关
bk-cli apigateway list_gateways --keyword "iam"
bk-cli apigateway list_gateways --name iam --fuzzy

# 再看某个网关下的公开接口
bk-cli apigateway list_gateway_apis --gateway_name bk-iam --keyword "system"

# 确认接口后查看详情
bk-cli apigateway retrieve_gateway_api_details --gateway_name bk-iam --api_name create_system
```

## Commands

### list_gateways

列出所有 API Gateway，可按条件筛选。

```bash
bk-cli apigateway list_gateways [--name <name>] [--fuzzy] [--keyword <keyword>]
```

| Flag | 类型 | 必填 | 说明 |
|------|------|------|------|
| `--name` | string | 否 | 网关名称过滤 |
| `--fuzzy` | bool | 否 | 启用网关名称模糊匹配 |
| `--keyword` | string | 否 | 描述字段中的搜索关键词 |

```bash
bk-cli apigateway list_gateways --name bk-iam
bk-cli apigateway list_gateways --name iam --fuzzy
bk-cli apigateway list_gateways --keyword "identity"
```

### list_gateway_apis

列出某个网关下的 API（资源）。

```bash
bk-cli apigateway list_gateway_apis --gateway_name <name> [--keyword <keyword>]
```

| Flag | 类型 | 必填 | 说明 |
|------|------|------|------|
| `--gateway_name` | string | **是** | 网关名称，必须匹配 `^[a-z][a-z0-9-]{2,29}$` |
| `--keyword` | string | 否 | 搜索关键词 |

```bash
bk-cli apigateway list_gateway_apis --gateway_name bk-iam
bk-cli apigateway list_gateway_apis --gateway_name bk-iam --keyword "create"
bk-cli apigateway list_gateway_apis --gateway_name bk-iam --header 'X-Request-Id:req-001'
```

### retrieve_gateway_api_details

获取某个资源的完整 API Schema，包括 OpenAPI 规范、请求/响应格式和认证要求。

```bash
bk-cli apigateway retrieve_gateway_api_details --gateway_name <name> --api_name <name>
```

| Flag | 类型 | 必填 | 说明 |
|------|------|------|------|
| `--gateway_name` | string | **是** | 网关名称，必须匹配 `^[a-z][a-z0-9-]{2,29}$` |
| `--api_name` | string | **是** | API 资源名 |

```bash
bk-cli apigateway retrieve_gateway_api_details --gateway_name bk-iam --api_name create_system
```

### apply_gateway_permission

调用 `v2_open_apply_gateway_permission` 申请网关或批量 API 权限。需要应用身份；`target_app_code` 必须与请求使用的应用一致。

不支持 `personal_token`。个人 token context 或显式 `Authorization: Bearer` header 在预览和提交时均返回 `unsupported_credential_type`，不会发送请求。先通过 `--context` 切换到应用 context，或使用 `bk_app_code` / `bk_app_secret` 加 `bk_token` / `bk_ticket`、满足要求的 `access_token` 登录，并移除 Bearer header。不要尝试在个人 token 上叠加 `X-Bkapi-Authorization`，也不要自动重试。

**Agent 每次实际提交前必须询问用户，确认目标网关、申请应用、资源范围、有效期和原因；暂不允许跳过询问。** 提交会创建申请单并触发通知和审批，成本较高。只有用户明确同意本次具体请求后才能传 `--confirmed`，该参数只是记录已获同意，不能代替询问。用户拒绝或尚未同意时停止提交；申请内容变化后需重新预览并确认。

先汇总同一网关所需的接口，用一个 `resource_names` 数组批量申请，避免逐接口创建申请单。只有用户明确同意整网关范围时才用 `grant_dimension: gateway`。不要自动重试，重试可能创建重复申请单。

```bash
bk-cli apigateway apply_gateway_permission --gateway_name <name> --body @permission.json [--dry-run | --confirmed]
```

| Flag | 类型 | 必填 | 说明 |
|------|------|------|------|
| `--gateway_name` | string | **是** | 要申请权限的目标网关，必须匹配 `^[a-z][a-z0-9-]{2,29}$` |
| `--body` | string | **是** | JSON 请求体，支持内联 JSON、`@file` 或 `-` 从 stdin 读取 |
| `--confirmed` | bool | **实际提交时是** | 仅在用户明确同意本次具体申请后传入 |
| `--dry-run` | bool | 否 | 只预览，不创建申请单，不需要确认 |

准备 `permission.json`，将应用、申请人、网关和接口名替换为实际确认的值：

```json
{
  "target_app_code": "my-app",
  "grant_dimension": "resource",
  "resource_names": ["user_query", "user_create"],
  "applicant": "developer",
  "expire_days": 180,
  "reason": "Read and create users for the integration"
}
```

`target_app_code`、`grant_dimension`、`applicant` 必填；按资源申请时 `resource_names` 必填，必须是非空字符串数组。`grant_dimension` 只能为 `resource` 或 `gateway`；整网关申请无需 `resource_names`。`expire_days` 可选，整数 `0` 或省略表示永久，`180` 表示 6 个月，`360` 表示 12 个月；`reason` 可选。

```bash
# 先预览，向用户说明具体申请内容及通知、审批成本
bk-cli apigateway apply_gateway_permission --gateway_name bk-iam \
  --body @permission.json --dry-run

# 仅在用户明确同意上述具体申请后提交
bk-cli apigateway apply_gateway_permission --gateway_name bk-iam \
  --body @permission.json --confirmed
```

不传 `--confirmed` 或传 `--confirmed=false` 时，实际提交返回 `confirmation_required`，不会发起请求。预览和提交沿用共享请求层的 JSON 语法校验和内联、文件、stdin 读取规则；非法 JSON 返回 `request_error`，不会发送请求。原始 body 保持不变；请求体结构、字段类型、必填字段、申请范围、资源列表、有效期以及是否允许申请均由服务端校验。

返回值保持 JSON envelope 和完整上游响应。提交成功后，向用户展示 `data.data.record_id` 申请单号（`message` 也会展示返回的单号），大单号应保持原值。**提交成功不表示权限已获批。** 响应中若有非空 `itsm_ticket_id`、`itsm_ticket_url`，展示对应工单和审批链接；若返回审批人或其他审批信息，也一并告知用户。没有返回链接或审批人时不要猜测，不要把申请单号当作审批完成状态。

### query_log_by_request_id

根据 request_id 查询 API Gateway 日志。

```bash
bk-cli apigateway query_log_by_request_id --request_id <id>
```

| Flag | 类型 | 必填 | 说明 |
|------|------|------|------|
| `--request_id` | string | **是** | 要查询的请求 ID |

```bash
bk-cli apigateway query_log_by_request_id --request_id=9f6563e4-cc2d-402d-b8d0-11ad26d7f6a9
```

## 认证与通用输入

认证、`--stage`、`--body`、`--header`、tenant、timeout、dry-run、verbose 等通用规则见 `../bk-cli-shared/SKILL.md`。

## 环境与请求输入

```bash
# 使用 testing stage，而不是 prod
bk-cli apigateway list_gateways --stage testing

# 使用不同的 context（BlueKing 部署）
bk-cli apigateway list_gateways --context clouds

# 查看实际请求/响应细节
bk-cli apigateway list_gateways --verbose

# 传入请求级 header
bk-cli apigateway list_gateway_apis --gateway_name bk-iam \
  --header 'X-Request-Id:req-001'
```

说明：
- `--gateway_name` 会在本地校验 `^[a-z][a-z0-9-]{2,29}$`，不合法时直接返回 `invalid_gateway_name`。
- 其他通用请求输入与排障顺序以 `../bk-cli-shared/SKILL.md` 为准。

## 原始 API 调用

如果某个 apigateway 接口还没有映射成子命令，直接使用 `bk-cli api`。完整说明见 `bk-cli-api` skill。

```bash
bk-cli api bk-apigateway GET /api/v2/open/gateways/ --query '{"name":"bk-iam"}'
bk-cli api bk-apigateway GET /api/v2/open/gateways/ --dry-run
```

一个常见串联方式是：

```bash
# 1. 发现公开网关
bk-cli apigateway list_gateways --name iam --fuzzy

# 2. 浏览该网关下的公开接口
bk-cli apigateway list_gateway_apis --gateway_name bk-iam --keyword "system"

# 3. 查看目标接口详情 / Schema
bk-cli apigateway retrieve_gateway_api_details --gateway_name bk-iam --api_name create_system

# 4. 根据详情改用 bk-cli api 发真实请求
bk-cli api bk-iam POST /api/v2/systems/ --body '{"id":"demo","name":"Demo"}' --dry-run
```

上面第 4 步里的 `POST`、路径和 body 只是“根据第 3 步详情去落命令”的示例形态。真正执行时，应以 `retrieve_gateway_api_details` 返回的协议信息为准，不要跳过确认直接套用。

## Pipeline 示例

可直接使用内置 `--jq` 提取完整 envelope 中的字段，无需安装外部 jq：

```bash
bk-cli apigateway list_gateways --jq '.data'
bk-cli apigateway list_gateways --jq '{ok, status, data}'
```

JSON Lines 输出、失败保留原结果及重试规则见[共享 skill 的内置 `--jq` 说明](../bk-cli-shared/SKILL.md#内置---jq-结果投影)。投影失败不表示请求未执行，尤其不能据此自动重发权限申请。

需要外部 jq 的选项（例如 `-r` 输出无引号字符串）时，仍可使用以下 pipeline：

```bash
# 提取 data
bk-cli apigateway list_gateways | jq '.data'

# 统计某个网关的 API 数量
bk-cli apigateway list_gateway_apis --gateway_name bk-iam | jq '.data | length'

# 列出 API 名称
bk-cli apigateway list_gateway_apis --gateway_name bk-iam | jq -r '.data[].name'
```

`apigateway` 子命令的 stdout 同样是结构化 JSON envelope，适合继续接 `jq`、shell pipeline 或其他自动化步骤。
