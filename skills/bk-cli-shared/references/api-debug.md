# api-debug — APIGateway 排障参考

用于按 `status`、`code_name`、错误消息快速定位 APIGateway 常见问题。

## Agent 使用方式

1. 先判断响应是不是网关返回。
2. 再按 `HTTP status -> code_name/message` 查找最接近的条目。
3. 优先提取并保留这些上下文：`x-bkapi-request-id`、请求时间、method、path、stage、关键 header。
4. 如果文档说明“不是网关返回”，就把问题转给后端服务或网关负责人，不要继续把结论归因给 CLI。

## 如何判断是不是网关返回

调用链路：

```text
调用方 -> APIGateway -> 后端服务
```

网关错误响应协议如下：

```json
{
  "code_name": "",
  "data": null,
  "code": 16xxxxx,
  "message": "",
  "result": false
}
```

判断规则：

- 如果响应体符合上面的结构，优先按“网关返回”处理。
- 如果响应体不符合上面的结构，通常说明响应来自后端服务，不是网关统一错误协议。
- 对“不是网关返回”的响应有疑问时，应联系对应后端服务或网关负责人继续排查。

## 快速索引

| Status | 典型关键词 | 快速结论 |
|------|------|------|
| `308` | redirect | 不是网关返回 |
| `400` | `INVALID_ARGS` | 认证头、应用身份、用户身份、token 问题 |
| `403` | `APP_NO_PERMISSION` / `IP_NOT_ALLOWED` | 权限未申请或 IP 不在白名单 |
| `404` | `API_NOT_FOUND` | method/path 不存在，或后端 301 导致错误跳转 |
| `413` | `BODY_SIZE_LIMIT_EXCEED` | body 太大 |
| `414` | `URI_SIZE_LIMIT_EXCEED` | URI 太长 |
| `415` | unsupported media type | 不是网关返回 |
| `429` | `RATE_LIMIT_RESTRICTION` / `CONCURRENCY_LIMIT_RESTRICTION` | 命中频控或并发限制 |
| `499` | client closed request | 客户端超时或主动中断 |
| `502` | `BAD_GATEWAY` / `ERROR_REQUESTING_RESOURCE` | 后端连接异常、DNS、重启、接入层问题 |
| `504` | `REQUEST_BACKEND_TIMEOUT` | 后端超时或协议配置错误 |
| `508` | `RECURSIVE_REQUEST_DETECTED` | backend 指向了另一个网关 |

## status: 308

网关不会返回 `status code = 308`。

如果在网关的【环境管理】 - 【代理配置】中的负载均衡 Host 中配置后端服务地址为 http://xxx.com，而 xxx.com 只支持 https 访问，其在接入层可能做了 redirect，也可能直接拒绝访问

此时，调用会出现 308 之类的状态码 (如果调用方的 client 支持 redirect，会继续请求重定向后的接口，状态码是重定向接口返回的)

处理：确认后端服务真实的 scheme

## status: 400

### `app code cannot be empty`

```json
{
  "code": 1640001,
  "data": null,
  "code_name": "INVALID_ARGS",
  "message": "Parameters error [reason=\"app code cannot be empty\"]",
  "result": false
}
```

- 原因：没有提供 `X-Bkapi-Authorization`，或者其中缺少 `bk_app_code`。
- 处理：补充 `X-Bkapi-Authorization`，并确认其中包含 `bk_app_code`。
- 补充：
  - `app code cannot be longer than 32 characters`：`bk_app_code` 本身不合法。
  - `app secret cannot be longer than 128 characters`：`bk_app_secret` 本身不合法。

### `app not found`

```json
{
  "message": "Parameters error [reason=\"app not found\"]",
  "code_name": "INVALID_ARGS",
  "code": 1640001,
  "data": null,
  "result": false
}
```

- 原因：`bk_app_code` 没有找到。
- 处理：优先检查是否调用到了错误环境，或是否用了错误应用。

### `please provide bk_app_secret or bk_signature to verify app`

```json
{
  "code": 1640001,
  "data": null,
  "code_name": "INVALID_ARGS",
  "message": "Parameters error [reason=\"please provide bk_app_secret or bk_signature to verify app\"]",
  "result": false
}
```

- 原因：`X-Bkapi-Authorization` 中缺少 `bk_app_secret`。
- 处理：补充 `bk_app_secret`，或检查认证方式是否符合接口要求。

### `bk_app_code or bk_app_secret is incorrect`

```json
{
  "code": 1640001,
  "data": null,
  "code_name": "INVALID_ARGS",
  "message": "Parameters error [reason=\"bk_app_code or bk_app_secret is incorrect\"]",
  "result": false
}
```

- 原因：`bk_app_code + bk_app_secret` 校验失败。
- 处理：确认请求头中的应用身份与 BlueKing PaaS 平台或运维签发的一致。

### `user authentication failed, please provide a valid user identity, such as bk_username, bk_token, access_token`

```json
{
  "code": 1640001,
  "data": null,
  "code_name": "INVALID_ARGS",
  "message": "Parameters error [reason=\"user authentication failed, please provide a valid user identity, such as bk_username, bk_token, access_token\"]",
  "result": false
}
```

- 可能原因：
  - 没有提供 `X-Bkapi-Authorization`。
  - 没有提供 `bk_token` 或 `access_token`。
  - `bk_token` 非法或已过期。
  - `access_token` 非法或已过期。
- 处理：确认 `bk_token` 或 `access_token` 存在且可用。

### `user authentication failed, the user indicated by bk_username is not verified`

```json
{
  "code": 1640001,
  "data": null,
  "code_name": "INVALID_ARGS",
  "message": "Parameters error [reason=\"user authentication failed, the user indicated by bk_username is not verified\"]",
  "result": false
}
```

- 原因：只传了 `bk_username`，但没有提供能证明真实用户身份的 `bk_token` 或 `access_token`。
- 处理：补充合法的 `bk_token` 或 `access_token`。
- 备注：
  - 不建议依赖“免用户认证应用白名单（不推荐）”这类插件配置。
  - 若接口需要用户身份，应按接口要求开启并传递用户认证，而不是规避用户认证。

### `access_token is invalid`

```json
{
  "code": 1640001,
  "data": null,
  "code_name": "INVALID_ARGS",
  "message": "Parameters error [reason=\"access_token is invalid, url: http://authapi.example.com/oauth/token, code: 403\"]",
  "result": false
}
```

- 可能原因：
  - `access_token` 填错了。
  - `access_token` 已过期。
  - `access_token` 不是通过正确环境生成。
- 处理：
  - 重新确认 token 来源与目标环境。
  - 若已过期，续期或重新生成。

### `the access_token is the application type and cannot indicate the user`

```json
{
  "code_name": "INVALID_ARGS",
  "code": 1640001,
  "data": null,
  "message": "Parameters error [reason=\"the access_token is the application type and cannot indicate the user\"]",
  "result": false
}
```

- 原因：当前 `access_token` 是应用态 token，只能代表 `bk_app_code + bk_app_secret`，不能代表用户。
- 处理：改用用户态 `access_token` 调用需要用户身份的接口。

## status: 401

- 当前文档没有共享排障条目。
- 建议优先检查认证头是否缺失、token 是否过期，以及接口是否要求更高等级的身份。

## status: 403

### `App has no permission to the resource`

```json
{
  "code": 1640301,
  "data": null,
  "code_name": "APP_NO_PERMISSION",
  "message": "App has no permission to the resource",
  "result": false
}
```

- 原因：网关 API 开启了资源权限校验，当前 `bk_app_code` 没有权限，或者权限已过期。
- 处理：到开发者中心申请或续期对应云 API 权限。

### `Request rejected by ip restriction`

```json
{
  "code_name": "IP_NOT_ALLOWED",
  "message": "Request rejected by ip restriction",
  "result": false,
  "data": null,
  "code": 1640302
}
```

- 原因：命中了网关或资源配置的 IP 访问保护。
- 处理：将调用方出口 IP 加入白名单。

## status: 404

### `API not found`

```json
{
  "code_name": "API_NOT_FOUND",
  "message": "API not found [method=\"POST\" path=\"/api/xxxxx\"]",
  "result": false,
  "data": null,
  "code": 1640401
}
```

- 常见原因：
  - method/path 拼错了。
  - 对应资源没有发布。
- 处理：
  - 先确认 method 与 path 完全正确。
  - 再找网关负责人确认资源已发布且存在。

### `API not found` 但本质是后端 `301 -> 404`

- 小概率情况：网关已经将请求代理给后端，但后端先返回 `301`，且 `location` 是错误 URL；如果 HTTP client 自动 follow redirect，就会拼出错误地址，最终得到 `404`。
- 常见场景：后端框架把不带 `/` 的 URL 自动重定向为带 `/` 的 URL。
- 排查方式：用 `curl -vv` 复现，确认是否出现两次请求，以及第一次 `301` 的 `location` 是否错误。
- 修复建议：后端 API 不应对网关代理过来的接口返回 `301`。

示例：

```text
* [HTTP/2] [1] OPENED stream for https://xxx.apigw.example.com/prod/api/foo/bar
> GET /prod/api/foo/bar HTTP/2
> Host: xxx.apigw.example.com
* Request completely sent off
< HTTP/2 301
< location: /api/foo/bar/
< x-bkapi-request-id: c1b44506-9e78-4993-a0e9-06ea72821144
< x-request-id: c1b445069e784993a0e906ea72821144

* Issue another request to this URL: 'https://xxx.apigw.example.com/api/foo/bar/'
* [HTTP/2] [3] OPENED stream for https://xxx.apigw.example.com/api/foo/bar/
> GET /api/foo/bar/?bk_biz_id=123&domain_list=123&vip_list=123&rs_list=123 HTTP/2
> Host: xxx.apigw.example.com
< HTTP/2 404
{"message":"API not found [method=\"GET\" path=\"/api/xxx/api/foo/bar/\"]","code_name":"API_NOT_FOUND","code":1640401,"data":null,"result":false}
```

## status: 413

### `Request body size too large`

```json
{
  "code_name": "BODY_SIZE_LIMIT_EXCEED",
  "message": "Request body size too large.",
  "result": false,
  "data": null,
  "code": 1641301
}
```

- 原因：请求体超过网关限制，当前上限约为 `40M`。
- 处理：避免经由网关传超大 body，必要时直连后端服务。

## status: 414

### `Request uri size too large`

```json
{
  "code_name": "URI_SIZE_LIMIT_EXCEED",
  "message": "Request uri size too large.",
  "result": false,
  "data": null,
  "code": 1641401
}
```

- 原因：URI 太长。
- 处理：不要把超长参数拼进 URI，改走 body 或缩短 query。

## status: 415

网关不会返回 `status code = 415`。

- 415 Unsupported Media Type
- 415 不支持请求中的媒体类型
- 返回 status code 415 表示后端不支持对应的 content-type, 需要 client 发起请求时指定正确的 content-type，例如如果服务端要求使用 json，那么调用时需要在请求中增加 header `content-type: application/json`

## status: 429

频控相关 header：

```text
X-Bkapi-RateLimit-Limit
X-Bkapi-RateLimit-Remaining
X-Bkapi-RateLimit-Reset
X-Bkapi-RateLimit-Plugin
```

### `API rate limit exceeded by stage strategy`

```json
{
  "code_name": "RATE_LIMIT_RESTRICTION",
  "message": "API rate limit exceeded by stage strategy",
  "result": false,
  "data": null,
  "code": 1642902
}
```

- 原因：命中了环境级频率控制策略。
- 处理：降低调用频率，或联系网关负责人调整策略。

### `API rate limit exceeded by resource strategy`

```json
{
  "code_name": "RATE_LIMIT_RESTRICTION",
  "message": "API rate limit exceeded by resource strategy",
  "result": false,
  "data": null,
  "code": 1642903
}
```

- 原因：命中了 API 资源级频率控制策略。
- 处理：降低调用频率，或联系网关负责人调整策略。

### `API rate limit exceeded by stage global limit (deprecated)`

```json
{
  "code_name": "RATE_LIMIT_RESTRICTION",
  "message": "API rate limit exceeded by stage global limit",
  "result": false,
  "data": null,
  "code": 1642901
}
```

- 原因：命中了已废弃的环境全局频率控制策略。
- 处理：降低调用频率，或联系网关负责人确认旧策略是否仍在生效。

### `Request concurrency exceeds`

```json
{
  "code_name": "CONCURRENCY_LIMIT_RESTRICTION",
  "message": "Request concurrency exceeds",
  "result": false,
  "data": null,
  "code": 1642904
}
```

- 原因：请求并发超过网关限制。
- 处理：降低并发，不要使用网关接口做压测。

## status: 499 Client Closed Request

### 无 response body

> `499 client has closed connection` 表示客户端发起请求后，没有等到服务端响应，就主动断开了连接。

- 常见原因：
  - client 自己的 timeout 太短。
  - 后端服务响应过慢，client 先超时退出。
- 处理：
  - 先确认 client timeout 是否合理。
  - 再联系网关负责人确认后端性能是否达标。

其他排查线索：

- 如果是 SaaS，并且失败请求基本都卡在 `30s` 左右，优先检查是否使用 `gunicorn` 默认超时：
  - 文档：`https://docs.gunicorn.org/en/stable/settings.html#timeout`
- 如果不是 SaaS，检查请求是否运行在某个 worker / handler 生命周期内；若 worker 提前中止，也会导致请求中止。

## status: 500

- 当前文档没有共享排障条目。
- 建议结合 `x-bkapi-request-id`、网关日志和后端日志一起排查。

## status: 502

### `Bad Gateway [upstream_error="cannot read header from upstream"]`

```json
{
  "data": null,
  "code_name": "BAD_GATEWAY",
  "code": 1650200,
  "message": "Bad Gateway [upstream_error=\"cannot read header from upstream\"]",
  "result": false
}
```

- 对应 nginx 错误：`upstream prematurely closed connection while reading response header from upstream`
- 可能原因：
  - 后端发布、reload、重启。
  - 网络抖动。
  - 后端启用了 keep-alive，但 keep-alive timeout 小于 `60s`。
- 处理：带上时间、request 信息、`request-id` 找网关负责人进一步排查。

### `104: Connection reset by peer`

```json
{
  "data": null,
  "code_name": "BAD_GATEWAY",
  "code": 1650200,
  "message": "Bad Gateway",
  "result": false
}
```

> `recv() failed (104: Connection reset by peer) while reading response header from upstream`

- 原因：网关等待后端响应时，连接被后端 reset。
- 常见场景：后端发布或重启导致连接中断。
- 处理：带上请求时间、request 信息、`request-id` 找网关负责人继续查。

### DNS 解析失败

```json
{
  "data": null,
  "code_name": "BAD_GATEWAY",
  "code": 1650200,
  "message": "Bad Gateway",
  "result": false
}
```

- 现象：后端服务域名无法解析时，也可能表现为 `502`。
- 常见原因：
  - 后端服务地址配置错误。
  - 后端服务仍使用不再支持的 `.oa` 域名。
- 处理：检查后端域名在 IDC 环境是否可解析、可连通。

### `Request backend service failed`

```json
{
  "data": null,
  "code_name": "ERROR_REQUESTING_RESOURCE",
  "code": 1650201,
  "message": "Request backend service failed [detail=\"Bad Gateway\" err=\"EOF\" status=\"502\"]",
  "result": false
}
```

- 原因：网关请求后端服务失败，具体细节在 `message` 中。
- 处理：直接访问后端服务地址，在 IDC 机器上用 `curl` 复现，确认是否是后端服务或接入层问题。

## status: 503

- 当前文档没有共享排障条目。
- 建议优先确认网关环境是否正在发布、重启，或后端是否处于不可用状态。

## status: 504

### `Request backend service timeout [upstream_error="cannot read header from upstream"]`

```json
{
  "code_name": "REQUEST_BACKEND_TIMEOUT",
  "data": null,
  "code": 1650401,
  "message": "Request backend service timeout [upstream_error=\"cannot read header from upstream\"]",
  "result": false
}
```

- 对应 nginx 错误：`upstream timed out (110: Connection timed out) while reading response header from upstream`
- 常见原因：
  - 后端接口响应超时。
  - 网关资源配置的后端 timeout 太小。
  - 后端只支持 `https`，但网关后端地址配成了 `http://{host}:{port}`。
- 处理：
  - 优先优化后端性能，必要时再谨慎调大网关 timeout。
  - 如果是协议配置问题，把后端地址改成 `https://{host}:{port}`。

## status: 508

### `Recursive request detected, please contact the api manager to check the resource configuration`

```json
{
  "code_name": "RECURSIVE_REQUEST_DETECTED",
  "data": null,
  "code": 1650801,
  "message": "Recursive request detected, please contact the api manager to check the resource configuration",
  "result": false
}
```

- 原因：网关禁止把另一个网关地址作为 backend，避免递归调用。
- 处理：
  - 不要把另一个网关地址配置为 backend。
  - 如果请求从网关进入后端，后端又要调用另一个网关接口，请重新构造下游请求，不要复用上游请求头。
