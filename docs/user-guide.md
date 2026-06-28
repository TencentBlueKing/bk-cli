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
bk-cli --help
```

### 安装 SKILL (建议)

建议给使用 `bk-cli` 的 agent 安装 skills

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

```
# 注意，直接复制执行，不需要替换 `gateway_name`，这是个模板用于实际访问某个网关时渲染

bk-cli context init \
  --bk_api_url_tmpl="https://bkapi.example.com/api/{gateway_name}"
```

如果你希望这个环境默认带租户或自定义超时：

```
# 注意，不需要替换 `gateway_name`，这是个模板用于实际访问某个网关时渲染

bk-cli context init \
  --bk_api_url_tmpl="https://bkapi.example.com/api/{gateway_name}" \
  --tenant_id="tenant-a" \
  --timeout 90s
```

### 第二步：登录

最常见的是“应用 + 用户令牌”方式：

- bk_app_code/bk_app_secret: 在 蓝鲸开发者中心 应用设置 - 基本信息
- bk_ticket(内部上云)/bk_token(其他环境): 使用 Chrome 浏览器，登录任意一个蓝鲸站点，F12-Application-Cookies，搜索 `bk_ticket` / `bk_token`  [auth login 如何获取 bk_ticket/bk_token?](faq.md#auth-login-如何获取-bk_ticketbk_token)

```
# 注意，如果 your_secret/your_bk_ticket/your_bk_token 有特殊字符例如 $或 双引号 等，请把双引号换成单引号，避免被 shell 转义导致失败

(内部上云版网关)
bk-cli auth login \
  --bk_app_code="your_app" \
  --bk_app_secret="your_secret" \
  --bk_ticket="your_bk_ticket"

（其他版本网关，例如bk-dev/bkop/sg/bk2game 等）

bk-cli auth login \
  --bk_app_code="your_app" \
  --bk_app_secret="your_secret" \
  --bk_token="your_bk_token"
```

如果你拿到的是 access token，也可以直接登录：

- access_token: [auth login 需要的 access_token 如何获得？](faq.md#auth-login-需要的-access_token-如何获得)

```
bk-cli auth login --access_token="your_access_token"
```

登录后可以检查状态：

```
bk-cli auth status
bk-cli auth check
```

`auth status` 用来看当前 context 是否已有凭据，`auth check` 更适合脚本里做 fail-fast。

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
  --bk_api_url_tmpl="https://bkapi.example.com/api/{gateway_name}"

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
- `--body`：传 JSON 请求体
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

### 使用 system 子命令

system 子命令更适合“平台操作”，不需要你自己拼完整接口路径。

例如：

```
bk-cli apigateway list_gateway_apis --gateway_name bk-iam

bk-cli job fast_execute_script \
  --bk_biz_id 2 \
  --script_content "echo hello" \
  --target_server '{"host_id_list":[1]}'

bk-cli job +run-script \
  --biz 2 \
  --hosts 10.0.0.1,27:10.0.0.2 \
  --script_content "echo hello"

bk-cli bcs cluster_manager update_cluster --clusterID BCS-K8S-12345 \
  --body '{"clusterID":"BCS-K8S-12345"}'
```

`job +run-script` 是快捷指令，会先用 CMDB 将 IP 解析为主机 ID，再调用 Job 快速执行脚本；如果你已经有 `target_server` JSON，仍可直接使用 `job fast_execute_script`。

建议习惯性地先看帮助：

```
bk-cli bcs cluster_manager update_cluster --help
bk-cli bcs cluster_manager update_cluster -h --body-schema
bk-cli job --help
bk-cli job fast_execute_script --help
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

```
# 先检查凭据，失败就退出
bk-cli auth check >/dev/null

# 请求结果交给 jq 继续处理
bk-cli api bk-apigateway GET /api/v2/open/gateways/ | jq .
```

### 2. 优先查看是否有对应的  system 子命令，没有的话再切换使用 `api`

使用 `-h` 方法，查看是否有对应的 system 子命令。优先使用 system 子命令。

如果没有对应的 system 或 system 子命令，再切换使用 `api`。

这是最省时间、也最不容易卡住的用法。

### 3. 多环境并行使用

```
bk-cli context create prod \
  --bk_api_url_tmpl="https://bkapi.example.com/api/{gateway_name}"

bk-cli context create test \
  --bk_api_url_tmpl="https://bkapi.example.com/api/{gateway_name}"

bk-cli auth login --context prod --access_token="prod-token"
bk-cli auth login --context test --access_token="test-token"

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
