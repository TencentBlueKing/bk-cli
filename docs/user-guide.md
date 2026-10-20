# bk-cli 用户指南

> 这是一份写给 `bk-cli` 使用者的指南。

## 1. bk-cli 是做什么的

`bk-cli` 是一个和 BlueKing 平台 API 交互的命令行工具。

你可以把它理解成两种用法：

1. 直接调用任意网关接口：`bk-cli api ...`
2. 使用已经封装好的高层命令：`bk-cli apigateway ...`、`bk-cli cmdb ...`、`bk-cli job ...`

如果你是第一次接触，建议先记住这几个点：

- `context` 用来保存“你连哪个环境或账号 (隔离)
- `auth` 用来保存“你用什么凭据访问”
- `doctor` 用来排查当前 context、凭据、URL 渲染和网关连通性
- `api` 是最通用的兜底方式
- system 子命令更省心，适合常见操作

## 2. 安装

### 推荐：从 npm 安装

从 [npm](https://www.npmjs.com/package/@blueking/bk-cli) 安装：

```
npm i -g @blueking/bk-cli
```

### 其他方式

- 从 [GitHub Releases](https://github.com/TencentBlueKing/bk-cli/releases) 下载适用于你平台的二进制包（由 GoReleaser 发布）
- 从源码构建安装 (需要 golang 环境)

```
git clone https://github.com/TencentBlueKing/bk-cli.git
cd bk-cli
make install
```

安装完成后，先确认命令可用：

```
bk-cli version
bk-cli doctor --offline
bk-cli --help
```

### 安装 SKILL (建议)

`bk-cli` 二进制已经内置随当前版本发布的 skills，agent 可以直接读取：

```
bk-cli skills list
bk-cli skills read bk-cli-shared
bk-cli skills read bk-cli-shared/references/api-debug.md
bk-cli skills read bk-cli-shared --raw
```

`skills list` 和 `skills read` 默认输出 JSON，适合 agent 先发现并结构化读取 skill；需要直接读取原始 `SKILL.md` 或 reference Markdown 文件时，对 `skills read` 显式加 `--raw`。

如果 agent 运行环境需要把这些 skills 安装到独立的技能系统中，也可以从仓库安装。

必装 skills:

- bk-cli-shared 公共共享知识
- bk-cli-api 通过 `bk-cli api` 对任意 BlueKing API Gateway 发起原始 HTTP 调用
- bk-cli-apigateway 通过 `bk-cli apigateway` 发现 BlueKing API Gateway 中所有公开网关、浏览某个网关公开的 API/资源列表、获取 OpenAPI/Schema
- bk-cli-bcs 通过 `bk-cli bcs cluster_manager` 操作 BCS 集群、节点、节点组和节点模板
- 其他子系统 skill 按需安装

[bk-cli skills 仓库](https://github.com/TencentBlueKing/bk-cli/tree/master/skills)

#### 1. npx skills 从 GitHub 安装

```
# 安装时会执行 git clone，请确保网络可访问 GitHub
# HTTPS：
npx skills add https://github.com/TencentBlueKing/bk-cli.git
```

#### 2. 从 GitHub 仓库离线安装

```
git clone --depth 1 https://github.com/TencentBlueKing/bk-cli.git
npx skills add bk-cli/skills
```

## 3. 上手前先理解三个概念

### context：连接哪个环境

一个 context 代表一个 BlueKing 部署环境，比如 `default`、`dev`、`clouds`。

它通常保存这些信息：

- 网关 URL 模板
- 默认租户（可选）
- 默认超时

也可以作为账号隔离，例如一个账号 (应用身份/用户身份) 一个 context.

### auth：用什么凭据访问

认证信息是按 context 存的。也就是说：

- 你切到不同 context，可以使用不同凭据
- 先有 context，再做 `auth login`

### 两类命令入口

- `bk-cli api`：最灵活，适合直接调用接口
- `bk-cli <system> <action>`：更易用，适合常见平台操作

当前常见 system 包括：

- `apigateway`
- `bcs`
- `paas`
- `cmdb`
- `job`
- `sops`
- `gse`
- `devops`
- `nodeman`

不知道某个 system 能做什么时，先运行：

```
bk-cli --help
bk-cli apigateway --help
bk-cli bcs cluster_manager --help
bk-cli cmdb --help
```

## 4. 五分钟快速开始

### 第一步：初始化默认 context

Bash 示例使用单引号包裹 URL 模板、凭据和其他字面值，复制后替换引号内的示例值。
`{gateway_name}` 本身不是 Bash 变量，由 CLI 在请求时渲染；`$name`、`${name}` 和命令替换会在双引号内展开。
如果值包含单引号，需要先结束单引号、用反斜杠转义该字符，再重新开始单引号，例如 `'it'\''s'` 会传入 `it's`。

```
# 注意，直接复制执行，不需要替换 `gateway_name`，这是个模板用于实际访问某个网关时渲染

bk-cli context init \
  --bk_api_url_tmpl='https://bkapi.example.com/api/{gateway_name}'
```

如果你希望这个环境默认带租户或自定义超时：

```
# 注意，不需要替换 `gateway_name`，这是个模板用于实际访问某个网关时渲染

bk-cli context init \
  --bk_api_url_tmpl='https://bkapi.example.com/api/{gateway_name}' \
  --tenant_id='tenant-a' \
  --timeout 90s
```

### 第二步：登录

最常见的是“应用 + 用户令牌”方式：

- bk_app_code/bk_app_secret: 在 蓝鲸开发者中心 应用设置 - 基本信息
- bk_ticket(内部上云)/bk_token(其他环境): 使用 Chrome 浏览器，登录任意一个蓝鲸站点，F12-Application-Cookies，搜索 `bk_ticket` / `bk_token`  [auth login 如何获取 bk_ticket/bk_token?](faq.md#auth-login-如何获取-bk_ticketbk_token)

```
# 凭据使用单引号包裹，避免 $、命令替换、双引号和反斜杠被 shell 解释

# 内部上云版网关
bk-cli auth login \
  --bk_app_code='your_app' \
  --bk_app_secret='your_secret' \
  --bk_ticket='your_bk_ticket'

# 其他版本网关，例如 bk-dev/bkop/sg/bk2game 等

bk-cli auth login \
  --bk_app_code='your_app' \
  --bk_app_secret='your_secret' \
  --bk_token='your_bk_token'
```

如果你拿到的是 access token，也可以直接登录：

- access_token: [auth login 需要的 access_token 如何获得？](faq.md#auth-login-需要的-access_token-如何获得)

```
bk-cli auth login --access_token='your_access_token'
```

如果你的部署支持个人 token，可在个人中心生成 token、配置可调用的 API/MCP 权限，再单独登录：

```bash
bk-cli auth login --personal_token='<personal-token>'
# 或保存到指定 context
bk-cli auth login --context prod --personal_token='<personal-token>'
bk-cli api httpbin GET /anything
```

`--personal_token` 与其他登录凭据 flags 互斥，不能传入 `Bearer ` 前缀、空值、空白或控制字符。同一 context 再次登录会替换原凭据，凭据仍加密保存。原始 `api` 和要求认证的系统 action 使用 Bearer 鉴权，不需要同时提供应用凭据；明确无需认证的 action 不自动携带 token。token 有效期、权限和部署支持情况由服务端判断，CLI 不管理 MCP 权限列表。

`auth status/check` 显示 `credential_type=personal_token`，不显示 token；登录和 `check` 只检查本地保存状态，不验证远端权限。`logout` 删除该 context 的凭据。`--dry-run` 与 `--verbose` 的认证头均脱敏；远端响应正文仍按原样返回，服务端若回显敏感信息需自行处理。

单次覆盖 Bearer 认证值可用 `--header 'Authorization:Bearer <personal-token>'`，头名忽略大小写；个人模式不能同时显式提供 `X-Bkapi-Authorization`。租户来自 context 或显式 `X-Bk-Tenant-Id`。

#### 通过环境变量登录

为了避免票据出现在 `ps` 等进程命令行信息中，可以让 secrets 管理工具或 CI 注入环境变量，由 CLI 直接读取，不要再将变量展开到凭据 flags。

| 凭据字段 | 环境变量 |
| --- | --- |
| `bk_app_code` | `BK_CLI_BK_APP_CODE` |
| `bk_app_secret` | `BK_CLI_BK_APP_SECRET` |
| `bk_token` | `BK_CLI_BK_TOKEN` |
| `bk_ticket` | `BK_CLI_BK_TICKET` |
| `access_token` | `BK_CLI_ACCESS_TOKEN` |
| `personal_token` | `BK_CLI_PERSONAL_TOKEN` |

只注入完整的一种模式：应用 code + secret + token、应用 code + secret + ticket，独立 access token，或独立个人 token。注入后执行：

```bash
bk-cli auth login
bk-cli auth login --context dev
```

登录完成后，应在注入变量的 shell 或 CI 步骤中执行 `unset BK_CLI_BK_APP_CODE BK_CLI_BK_APP_SECRET BK_CLI_BK_TOKEN BK_CLI_BK_TICKET BK_CLI_ACCESS_TOKEN BK_CLI_PERSONAL_TOKEN`，或只在 `auth login` 这一步的作用域内注入这些变量，避免后续命令及其子进程（包括第三方插件）继承明文凭据。后续 API 请求可使用已加密保存到 context 的凭据。

显式凭据 flags 先确定模式，并优先于同名环境字段。环境只补齐选中模式内未显式传入的字段。例如 `--bk_token` 选择应用 + token，即使环境中还有 ticket 或 access token，也不会切换身份；只传应用 code 或 secret 时，从环境选择唯一的 token 或 ticket，不能改用 access token。旧模式的多个非空凭据 flags 仍沿用原顺序：access token 优先，其次 ticket，再其次 token。`--personal_token` 与其他凭据 flags 互斥，即使其他 flag 显式传空值也会报错；单独传 `--personal_token` 会忽略所有环境凭据。显式选择旧模式时忽略 `BK_CLI_PERSONAL_TOKEN`，不能用它补齐旧模式。

显式空值（如 `--bk_token=''`）报 `invalid_credentials`，不会回退到环境。无凭据 flags 时，个人 token 与任一其他凭据环境值共存、access token 与任一应用模式字段共存、或 token 与 ticket 同时非空，都会报 `invalid_credentials`；只传应用 code / secret 时，环境 token 与 ticket 同时非空也会报错。缺少模式必填字段同样失败，需补齐或清理环境后重试。

实际采用的环境值若含首尾空格、制表符或换行，会返回 `invalid_credentials` 并提示变量名；CLI 不自动裁剪原值。请修正 secrets 管理工具或 CI 注入的值后重试。被显式 flags 覆盖或属于未选模式的环境值不参与此检查。仅注入应用 code + secret 而缺少用户票据时，可以补充 `BK_CLI_BK_TOKEN` 或 `BK_CLI_BK_TICKET`，然后再次执行 `bk-cli auth login`。

个人 token 的环境值同样不能含 `Bearer ` 前缀、内部空白或非法 token 字符；使用与 flag 输入相同的格式校验。

没有凭据参数、环境也没有值时，仍返回 `not_implemented`（退出码 1）：设备码登录尚未实现，不等待 stdin、不使用或覆盖已有凭据。成功后仍加密保存到选中的 context；失败不覆盖旧凭据。

环境输入避免的是 argv 暴露；有权限的进程仍可能读取环境，shell 调试输出和 CI 日志也可能暴露值。单引号只防止 shell 展开，不隐藏已经传入 flags 的票据。本次登录仅支持 flags 和环境变量；`--body -` 的 stdin 输入用于 API 请求体，不能导入登录凭据。

登录后可以检查状态：

```
bk-cli auth status
bk-cli auth check
bk-cli doctor
```

`auth status` 用来看当前 context 是否已有凭据，`auth check` 更适合脚本里做 fail-fast。
`doctor` 适合排查当前机器上的 context 列表、选中的 context、凭据类型、脱敏票据、URL 模板渲染结果和网关连通性。
个人 token 登录后，`doctor` 会显示 `personal_token` 类型和脱敏值；凭据格式无效时，本地凭据检查失败。使用 `doctor --offline` 可跳过连通性探测；本地检查通过不代表 token 的远端有效期或权限已验证。

### 第三步：先发一个原始 API 请求

```
bk-cli api bk-apigateway GET /api/v2/open/gateways/?name=bk-iam
```

带查询参数：

```
bk-cli api bk-apigateway GET /api/v2/open/gateways/ \
  --query '{"name":"bk-iam","fuzzy":true}'
```

带请求体：

```
# 这个只是示例，不能实际执行成功
bk-cli api bk-demo POST /api/v2/resources/ \
  --body '{"name":"test"}'
```

### 第四步：再试一个 system 子命令

```
bk-cli apigateway list_gateways --name bk-iam --fuzzy
bk-cli cmdb search_business --bk_biz_id 2
bk-cli paas create_cloud_native_app -h --body-schema
```

如果你不知道参数怎么写，最稳妥的方式就是：

```
bk-cli apigateway list_gateways --help
bk-cli cmdb search_business --help
```

## 5. 常见操作

### 管理多个环境

```
# 新建一个 context
bk-cli context create dev \
  --bk_api_url_tmpl='https://bkapi.example.com/api/{gateway_name}'

# create 不会自动切换当前环境，按需显式切换
bk-cli context use dev

# 查看所有 context
bk-cli context list

# 查看当前环境配置
bk-cli context status
```

如果只想让某一次命令临时使用别的环境，不需要切换全局 active context：

```
bk-cli api bk-iam GET /api/v2/systems/ --context dev
```

### 管理认证信息

```
# 查看当前环境认证状态
bk-cli auth status

# 脚本里检查是否已登录
bk-cli auth check

# 排查 context、凭据、URL 渲染和网关连通性
bk-cli doctor
bk-cli doctor --offline

# 删除当前环境凭据
bk-cli auth logout
```

### 使用 `api` 命令

`api` 适合这些场景：

- 你已经知道接口路径和方法
- 暂时还没有对应的 system 子命令
- 你想快速验证一个接口

最常见的几个参数：

- `--query`：传查询参数，值是 JSON
- `--path`：替换路径模板里的占位符，值是 JSON
- `--body`：传 JSON 请求体，或用 `--body @file` 从本地文件读取 JSON 请求体，也可以用 `--body -` 从 stdin 读取
- `--stage`：指定网关 stage，默认是 `prod`
- `--timeout`：覆盖本次请求超时

例如：

```
# 路径占位符替换
bk-cli api bk-apigateway GET /api/v2/open/gateways/{gateway_name}/resources/ \
  --path '{"gateway_name":"bk-iam"}'

# 指定 stage
bk-cli api bk-demo GET /api/v2/foo/ --stage testing

# 覆盖本次超时
bk-cli api bk-demo GET /api/v2/foo/ --timeout 180s
```

### 先用 `apigateway` 查接口，再用 `api` 调用

这是一个很常见的流程，特别适合下面两种情况：

- `bk-cli` 里还没有你要的 system 子命令
- 你只知道业务大概在哪个平台，但还不确定具体网关名、接口名或协议细节

推荐流程是：

1. 先用 `bk-cli apigateway list_gateways` 查有哪些公开网关
2. 再用 `bk-cli apigateway list_gateway_apis` 查某个网关下有哪些接口
3. 需要时，用 `bk-cli apigateway retrieve_gateway_api_details` 看接口协议和细节
4. 最后回到 `bk-cli api` 发起实际调用

示例：

```
# 先找网关
bk-cli apigateway list_gateways --keyword iam

# 再看这个网关下有哪些接口
bk-cli apigateway list_gateway_apis --gateway_name bk-iam --keyword system

# 查看某个接口的详细协议
bk-cli apigateway retrieve_gateway_api_details \
  --gateway_name bk-iam \
  --api_name create_system

# 确认好网关、方法、路径和参数后，再用 api 发调用
bk-cli api bk-iam GET /api/v2/systems/
```

如果你暂时没有现成的 system 子命令，这通常是最稳妥、最高效的探索方式。

### 申请网关或接口权限

遇到网关 API 权限不足（例如错误码 `1640301`）时，可以使用 `bk-cli apigateway apply_gateway_permission` 调用 `v2_open_apply_gateway_permission` 发起申请。该接口需要应用身份；`target_app_code` 应与请求使用的应用一致。不支持 `personal_token`：个人 token context 或显式 `Authorization: Bearer` header 在预览和提交时均返回 `unsupported_credential_type`，不会发送请求。请使用应用凭据（`bk_app_code` / `bk_app_secret` 加 `bk_token` / `bk_ticket`）或满足要求的 `access_token`，也可通过 `--context` 切换到应用 context；移除 Bearer header 后重新预览。业务系统自身的权限不足需按对应系统流程处理。

**Agent 每次提交前必须询问用户，确认具体网关、申请应用、资源范围、有效期和原因；暂不允许跳过询问。** 申请会创建单据，触发通知和审批流程，成本较高。先汇总同一网关中需要的接口，将多个接口放入一个 `resource_names` 数组；只有用户明确同意整网关范围时才使用 `grant_dimension: gateway`。不要自动重试，避免重复申请单。

先准备 `permission.json`（将示例应用、申请人和资源名称替换为实际值）：

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

`target_app_code`、`grant_dimension`、`applicant` 必填；按资源申请时 `resource_names` 必填。`expire_days` 为 `180` 表示 6 个月、`360` 表示 12 个月，`0` 或省略表示永久；`reason` 可选。按整网关申请时将 `grant_dimension` 设为 `gateway`，无需 `resource_names`。

```bash
# 预览不会创建申请单，不需要确认
bk-cli apigateway apply_gateway_permission --gateway_name bk-iam \
  --body @permission.json --dry-run

# 将预览的具体申请内容告知用户并询问；仅在用户明确同意后执行
bk-cli apigateway apply_gateway_permission --gateway_name bk-iam \
  --body @permission.json --confirmed
```

预览和提交都沿用共享请求层的 JSON 语法校验和输入读取规则，支持内联 JSON、`@file` 和 stdin。非法 JSON 返回 `request_error`，不会发送请求。原始 body 保持不变；请求体结构、字段类型、必填字段、申请范围、资源列表、有效期以及是否允许申请均由服务端校验。修正申请内容后，应重新预览并向用户确认。

`--body` 也支持内联 JSON 或 `--body -` 从 stdin 读取。`--confirmed` 表示用户已明确确认本次申请内容，不能代替向用户询问；不传该参数或传 `--confirmed=false` 时，实际提交在本地返回 `confirmation_required`，不会发起请求。

返回值保持 JSON envelope 和完整上游响应；`message` 展示申请单号，`data.data.record_id` 是上游申请记录 ID。**提交成功不表示权限已获批。** Agent 应向用户展示申请单号，以及响应中实际返回的审批链接、审批人等信息，方便联系审批。`record_id` 为正整数，大单号在响应和 `message` 中保持原值。响应还可能包含 `itsm_ticket_id` 和 `itsm_ticket_url`；非空时应展示对应工单和链接。没有链接或审批人时不要猜测，也不要把申请单号当作审批完成状态。

### 使用 system 子命令

system 子命令更适合“平台操作”，不需要你自己拼完整接口路径。

例如：

```
bk-cli apigateway list_gateway_apis --gateway_name bk-iam

bk-cli job fast_execute_script \
  --bk_biz_id 2 \
  --script_content 'echo hello' \
  --target_server '{"host_id_list":[1]}'

bk-cli bcs cluster_manager update_cluster --clusterID BCS-K8S-12345 \
  --body '{"clusterID":"BCS-K8S-12345"}'

bk-cli paas create_cloud_native_app \
  --body '{"code":"bk-demo","name":"bk-demo","source_config":{"source_origin":1,"source_repo_url":"https://github.com/octocat/helloWorld.git","source_repo_auth_info":{},"source_dir":"","source_init_template":"docker"},"bkapp_spec":{"build_config":{"build_method":"dockerfile","dockerfile_path":"Dockerfile"}}}'

bk-cli paas get_minimal_app_list --app_status normal
bk-cli paas deploy_with_module --app_code bk-demo --env stag \
  --body '{"version_type":"branch","version_name":"master"}'
bk-cli paas list_config_vars --app_code bk-demo
bk-cli paas set_config_var_value --app_code bk-demo --config_var_key FOO \
  --body '{"environment_name":"stag","value":"bar","is_sensitive":false}'
bk-cli paas list_module_services --app_code bk-demo
bk-cli paas bind_service \
  --body '{"code":"bk-demo","service_id":"svc-uuid","module_name":"default"}'

```

建议习惯性地先看帮助：

```
bk-cli bcs cluster_manager update_cluster --help
bk-cli bcs cluster_manager update_cluster -h --body-schema
bk-cli job --help
bk-cli job fast_execute_script --help
bk-cli paas create_cloud_native_app -h --body-schema
```

## 6. 这些参数很常用

### `--context`

让当前命令临时使用指定 context，不影响全局切换结果。

```
bk-cli auth status --context dev
```

### `--dry-run`

先预览，不真正执行。非常适合排查参数是否拼对了。

```
bk-cli api bk-demo GET /api/v2/foo/ --dry-run
bk-cli update --dry-run
```

### `--verbose`

把更详细的请求和响应信息打到 `stderr`，适合排查问题。

```
bk-cli api bk-demo GET /api/v2/foo/ --verbose
```

### `--insecure`

跳过 HTTPS 证书校验，行为类似 `curl --insecure`。只建议在临时调试自签名证书或测试环境时使用；`--dry-run` 不会发起网络请求，因此不会触发 TLS 连接。

```
bk-cli api bk-demo GET /api/v2/foo/ --insecure
```

### `--header`

给单次请求补充或覆盖 header。常见于临时调试。

```
bk-cli api bk-demo GET /api/v2/foo/ \
  --header "X-Request-Id:req-001"
```

也可以单次覆盖认证或租户 header：

```
bk-cli api bk-demo GET /api/v2/foo/ \
  --header 'X-Bkapi-Authorization:{"access_token":"custom-token"}' \
  --header 'X-Bk-Tenant-Id:tenant-b'
```

## 7. 高级用法

### 1. 在脚本里使用

`bk-cli` 默认输出结构化 JSON，这对脚本很友好。

API 响应中的 JSON 数字会按原始字面量输出，例如 `9007199254740993` 不会在 CLI 内被舍入为 `9007199254740992`，也不会被转成字符串。`--dry-run` 不发请求，只预览输入的 query/body，并使用同样保留数字字面量的解析方式。下游工具的解析或算术操作仍受该工具自身的数字精度限制。

```
# 先检查凭据，失败就退出
bk-cli auth check >/dev/null

# 请求结果交给 jq 继续处理
bk-cli api bk-apigateway GET /api/v2/open/gateways/ | jq .
```

### 内置 jq 筛选

raw API 和所有 system action 可以直接用 `--jq` 查询完整最终 envelope：

```bash
# 提取网关名称，每个名称是一行带引号的 JSON 字符串
bk-cli apigateway list_gateways --jq '.data.items[] | .name'

# 在 CMDB 完成所有分页后汇总主机 ID
bk-cli cmdb list_biz_hosts_all --bk_biz_id 2 --jq '.data.info | map(.bk_host_id)'

# 对已脱敏预览查询 URL
bk-cli api bk-demo GET /api/v2/foo/ --dry-run --jq '.request.url'

# stdin 仍用于请求体，jq 不额外读取 stdin
cat body.json | bk-cli api bk-demo POST /api/v2/foo/ --body - --jq '.data'
```

每个结果是一行合法 JSON：对象和数组紧凑编码，字符串带引号并转义换行；多结果输出 JSON Lines，`empty` 无输出。`null` / `false` / `empty` 不采用外部 jq 的 `-e` 退出规则。根输入是 envelope，因此上游内容通常从 `.data` 开始查询。context、auth、doctor、version、update、skills 等命令不支持此 flag，使用时会明确报错。

空表达式、语法错误和未知函数在请求前失败，退出 1。运行错误或预算耗尽退出 2，并在 stdout 保留完整原结果、stderr 给出 `jq_execution_error` 或 `jq_limit_exceeded`。真实请求已经完成时会明确提示；先检查原结果中的任务/资源 ID，避免直接重跑写命令。已确认失败的结果不会被投影隐藏，原命令的 HTTP/业务判定及退出语义不变。stdout 写入失败返回非 0，部分写出后停止输出。

内置解释器为 gojq v0.12.19，无需外部 jq。正常 `halt` 结束展示，`halt_error` 作为展示错误处理。固定预算为 2 秒、10,000 个结果、16 MiB 编码输出（含换行）。不开放系统环境变量、文件模块或额外 stdin；不提供 `-r` / `-e`，部分正则特性与外部 jq 有差异。整数 ID 与整数运算保持精度，小数及指数按 gojq 浮点语义处理，不保证任意精度小数。投影只减少展示量，不减少上游请求或下载量；进程内计算不提供严格内存隔离。

### 2. 优先查看是否有对应的  system 子命令，没有的话再切换使用 `api`

使用 `-h` 方法，查看是否有对应的 system 子命令。优先使用 system 子命令。

如果没有对应的 system 或 system 子命令，再切换使用 `api`。

这是最省时间、也最不容易卡住的用法。

### 3. 多环境并行使用

```
bk-cli context create prod \
  --bk_api_url_tmpl='https://bkapi.example.com/api/{gateway_name}'

bk-cli context create test \
  --bk_api_url_tmpl='https://bkapi.example.com/api/{gateway_name}'

bk-cli auth login --context prod --access_token='prod-token'
bk-cli auth login --context test --access_token='test-token'

bk-cli api bk-iam GET /api/v2/systems/ --context prod
bk-cli api bk-iam GET /api/v2/systems/ --context test
```

### 4. 开启 shell 自动补全

如果你经常使用 `bk-cli`，可以生成自动补全脚本：

```
bk-cli completion bash --help
bk-cli completion zsh --help
bk-cli completion fish --help
```

## 8. 常见注意事项

### 先 `context init`，再 `auth login`

没有 context 时，`bk-cli` 不知道应该连哪个 BlueKing 环境。

### tenant 属于 context，不属于凭据

如果某个环境总是用同一个 tenant，建议在 `context init` 或 `context create` 时设置。
如果只是偶尔覆盖一次，用 `--header 'X-Bk-Tenant-Id:...'` 更方便。

### `--path` 适合模板路径，固定路径直接写出来更直观

这两种都可以：

```
bk-cli api bk-apigateway GET /api/v2/open/gateways/bk-iam/resources/

bk-cli api bk-apigateway GET /api/v2/open/gateways/{gateway_name}/resources/ \
  --path '{"gateway_name":"bk-iam"}'
```

如果路径已经是固定值，直接写完整路径通常更好读。

### 403 不一定都是同一种权限问题

可以先按这个思路判断：

- 如果网关返回类似“App has no permission”或错误码 `1640301`，通常是 API 权限
  - [应用如何申请调用对应网关的权限？](faq.md#应用如何申请调用对应网关的权限)
- 如果上游业务系统返回类似 `bk_error_code 9900403`，通常是业务权限

### 命令名和 context 名尽量使用小写字母、数字和中划线

这样最稳妥，也最不容易碰到本地校验错误。

## 9. 遇到问题时先看哪里

推荐按这个顺序排查：

1. 先运行 `bk-cli <命令> --help`
2. 再加 `--dry-run` 看最终会发什么请求
3. 需要更多细节时，再加 `--verbose`
4. 检查当前环境是否正确：`bk-cli context status`
5. 检查当前环境是否已登录：`bk-cli auth status`

## 10. 相关文档

- 项目简介与快速入口：[README.md](https://github.com/TencentBlueKing/bk-cli/blob/master/README.md)
- 常见问题：[docs/faq.md](faq.md)
- 开发者视角的扩展指南：[docs/develop-extension-guide.md](https://github.com/TencentBlueKing/bk-cli/blob/master/docs/develop-extension-guide.md)
