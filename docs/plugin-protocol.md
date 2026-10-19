# bk-cli 插件协议 v1（第三方 CLI 接入说明）

通过 `bk-cli [--context NAME] <插件名> ...` 启动时，bk-cli 会在子进程环境中设置
`BK_CLI_PLUGIN_PROTOCOL`、`BK_CLI_PLUGIN_CONTEXT`、`BK_CLI_PLUGIN_AUTH` 三个变量。
第三方单独运行时不受影响。接入官方目录前，请按本文最后的审核清单自查。

bk-cli 用 125 表示宿主在启动插件前失败；子进程退出码与信号行为见下文。第三方不得使用
125。

当前目录中的 bkms-cli v1.0.4 登记为 `auth: none`：bk-cli 会传递 context
环境信息，但不会读取或传递凭据。该版本尚未实现协议 v1，仍按 bkms-cli
自己的方式认证。凭据共享需要等待实现协议 v1 的新版本发布，并由维护者审核后新增
`auth: shared` 目录条目。

## 1. 环境变量

```text
BK_CLI_PLUGIN_PROTOCOL=1
BK_CLI_PLUGIN_CONTEXT=<JSON 或 null>
BK_CLI_PLUGIN_AUTH=<JSON 或 null>
```

三个变量总是同时设置。值由宿主直接写入子进程环境数组，不要求用户 export，不通过
argv 或文件传递。构造子进程环境时，宿主先删除继承环境中所有以
`BK_CLI_PLUGIN_` 开头的变量，再写入本次调用的新值；Windows 上前缀匹配不区分大小写。
因此调用方预设的旧协议变量不会传给插件。

`BK_CLI_PLUGIN_CONTEXT` 不含秘密，每次执行都注入：

```json
{"name":"clouds","bk_api_url_tmpl":"https://bkapi.clouds.example.com/api/{gateway_name}/","tenant_id":"t1"}
```

`tenant_id` 未配置时省略该字段。未初始化任何 context 时为 `null`。

`BK_CLI_PLUGIN_AUTH` 按凭据类型区分结构：

```json
{"type":"app_user","bk_app_code":"...","bk_app_secret":"...","bk_ticket":"..."}
```

```json
{"type":"app_user","bk_app_code":"...","bk_app_secret":"...","bk_token":"..."}
```

```json
{"type":"access_token","access_token":"..."}
```

- `app_user` 中 `bk_token` 与 `bk_ticket` 恰好出现一个。
- 协议 JSON 独立定义，不直接序列化 bk-cli 内部 `Credential` 结构；内部结构调整不改变协议。
- bk-cli 新增凭据类型（如个人 token）时，只增加新的 `type`，不升级协议版本号。

## 2. 宿主如何取值

`--context` 用于同时选择环境与凭据来源。插件不读取宿主配置目录，也不依赖宿主
context 或凭据加密文件的结构。

"所选 context"依次取：显式 `--context`、宿主 active context、名为 `default` 的 context。
三者都不适用但宿主存在其他 context 时，宿主报 `plugin_context_error`，不会按名称挑选
其中一个 context 传给插件。

| 宿主状态 | `CONTEXT` | `AUTH` |
| --- | --- | --- |
| 目录版本为 `auth: none` | 所选 context 的环境信息 | `null`，不读取凭据 |
| `auth: shared`，已登录 | 所选 context 的环境信息 | 按类型投影整份凭据 |
| `auth: shared`，context 已初始化但未登录 | 所选 context 的环境信息 | `null` |
| 未初始化任何 context 且未指定 `--context` | `null` | `null` |
| 显式指定的 context 不存在 | 宿主报 `plugin_context_error` | — |
| 无 active context、无 `default`，但存在其他 context | 宿主报 `plugin_context_error` | — |
| 凭据文件损坏或无法解密 | 宿主报 `plugin_credential_error` | — |
| 存储中 `bk_token` 与 `bk_ticket` 同时存在 | 宿主报 `plugin_credential_error`（凭据歧义） | — |

`auth login` 只会保存 `bk_token` 与 `bk_ticket` 之一；同时存在只可能来自手工修改或损坏，
因此报错而不是按优先级选取。一次调用读取一次凭据，后续切换 context 或重新登录不影响
已启动的子进程。

共享的是原始凭据，宿主不负责换票、刷新或保证后端接受。

## 3. 进程、信号与退出码

bk-cli 启动受管理的第三方进程后，会继承其 stdin、stdout、stderr 和工作目录，并等待
第三方进程退出。退出码契约如下：

| 情况 | 退出码 |
| --- | --- |
| bk-cli 在启动第三方进程前失败 | 125；stderr 为带 `plugin_*` 错误码的 JSON |
| 第三方正常退出 | 原样返回第三方退出码 |
| 第三方在 Unix 上被信号 `n` 终止 | `128 + n` |
| 第三方在 Windows 上退出 | 原样返回子进程状态中的退出码 |

Unix 上，宿主监听 `SIGINT`、`SIGQUIT`、`SIGTERM` 和 `SIGHUP`。终端已经把 `SIGINT`
和 `SIGQUIT` 发给整个前台进程组，因此宿主不会再次转发这两个信号；收到 `SIGTERM` 或
`SIGHUP` 时，宿主把信号转发给第三方并继续等待其退出。Windows 上，控制台会把
Ctrl-C/Ctrl-Break 直接送到第三方，宿主不会再次转发，并最终返回子进程状态中的退出码。

## 4. 第三方必须遵循的规则

```text
BK_CLI_PLUGIN_PROTOCOL 不存在：
    使用第三方原有的认证与配置方式

BK_CLI_PLUGIN_PROTOCOL 存在：
    校验协议版本、CONTEXT 与 AUTH
    凭据只保留在内存
    携带共享凭据的请求使用 CONTEXT 中的地址模板与租户
```

具体约定：

1. 协议版本未知、任一变量缺失、JSON 畸形、必填字段为空或 `type` 未知时，明确报错退出，
   不静默进入独立模式。
2. 进入协议模式后，共享凭据是本次调用唯一的认证来源，不回退到第三方本地保存的另一身份。
3. 携带共享凭据的请求必须用 `CONTEXT.bk_api_url_tmpl` 渲染网关地址，并使用
   `CONTEXT.tenant_id`，不得被第三方自身配置覆盖，避免把某一环境的凭据发送到另一环境。
4. `AUTH` 为 `null` 时，帮助和本地命令继续可用；需要认证的操作立即报错并提示
   `bk-cli auth login`，不另起交互登录。
5. 第三方根据凭据类型构造自己的认证请求；协议不规定 HTTP header、Cookie 或后端接口。
6. 不保存、打印或记录凭据。读取后从自身环境中移除 `BK_CLI_PLUGIN_*`，启动后续子进程时
   不传递。
7. 凭据过期时返回认证失败；协议不提供自动刷新或重新取票。
8. 受管理版本只能通过 `bk-cli plugin update <名称>` 更新。插件自己的自更新命令在
   `BK_CLI_PLUGIN_PROTOCOL` 存在时必须拒绝执行。

单独执行第三方 CLI 时，原有认证方式不受影响。

插件在协议模式下自行替换可执行文件会破坏 bk-cli 的摘要校验，后续调用报
`plugin_digest_mismatch`。恢复时使用
`bk-cli plugin install <名称> --version <已安装版本>`，不要继续运行插件自己的更新器。

## 5. 接入审核清单

官方目录中的“获准版本”表示**经过审核、允许持有凭据的完整执行方式**，而不只是一个
文件名或匹配的摘要。`auth: shared` 的版本必须满足：

1. 遵守上一节全部规则。
2. 不持久化、不记录共享凭据，调试或 verbose 输出中也不出现。
3. 不提供加载用户代码、执行 shell 或 hook、加载外部插件的入口；如果存在，这些路径拿不到凭据。
4. 分发产物中不内置任何应用 secret。
5. 自身不使用退出码 125。
6. 自更新命令在 `BK_CLI_PLUGIN_PROTOCOL` 存在时必须拒绝执行；受管理版本只允许通过
   `bk-cli plugin update` 更新。
7. 以 `CGO_ENABLED=0` 静态构建，使 `LD_PRELOAD`、`DYLD_INSERT_LIBRARIES`
   等动态库注入无效；宿主因此不必清理这类环境变量。

## 6. 申请收录

维护者先按上述审核清单审核第三方版本，再用以下工具生成目录条目：

```bash
go run ./tools/plugin-catalog-entry ...
```

核对生成结果后，将条目提交到 `internal/plugin/catalog.yaml`。只有实现本协议并通过审核的
版本才能使用 `--auth shared`；其他获准但不共享凭据的版本必须使用 `--auth none`。
