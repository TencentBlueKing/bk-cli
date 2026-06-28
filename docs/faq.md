# bk-cli FAQ

> 高频问题汇总。使用指南见 [user-guide.md](user-guide.md)，扩展开发见 [develop-extension-guide.md](develop-extension-guide.md)。

## 基本问题

### bk-cli 是通用的吗？能用在其他环境吗？

可以。所有蓝鲸环境都可以使用，按规则配置 `context` 并使用对应环境的票据 `login` 即可。

另外，需要确保 `bk-cli` 安装所在机器能访问对应环境的网关地址。

如果不确定当前机器是否配置正确，可以先运行：

```
bk-cli doctor
```

它会检查本地 context、当前选中的 context、登录凭据摘要、`bk_api_url_tmpl` 渲染结果和网关连通性。只想看本地状态、不做网络探测时，使用 `bk-cli doctor --offline`。

## 安装

## SKILLs

### 安装时报错 `Permission denied` 或 `Could not read from remote repository`

`npx skills add` 安装时会执行 `git clone`，请确保网络可访问 GitHub：

```
npx skills add https://github.com/TencentBlueKing/bk-cli.git
```

如果无法直接访问 GitHub，可以先在本地下载或打包 `skills/` 目录，再离线安装：

```
git clone --depth 1 https://github.com/TencentBlueKing/bk-cli.git
npx skills add bk-cli/skills
```

skills 所在目录：[bk-cli/skills](https://github.com/TencentBlueKing/bk-cli/tree/master/skills)。本质上是一组 markdown 文件，可以下载后打包分发。

### 机器所在的区域无法访问 GitHub 怎么办？

目前使用的 [npx skills](https://www.npmjs.com/package/skills) 工具，依托 Git 仓库进行分发。

如果对应区域无法访问 GitHub，只能通过文件分发的方式：下载 `skills/` 目录后打包分发，再在本机执行 `npx skills add <本地路径>`。

## context

### context 可以怎么用？

这里的 context 设计参考了 k8s 的 context。

可以使用：

- `bk-cli context list` 查看所有 context
- `bk-cli context status` 查看当前生效的 context
- `bk-cli context use {context_name}` 切换 context
- `bk-cli xxxxxxxx --context {context_name}` 使用对应 context 发起调用

context 本身是逻辑上的设计，可以用于隔离：

- 作为环境隔离，例如内部上云一个 context，bkop 一个 context，在同一台机器上用 `bk-cli` 操作多个环境
- 作为账号隔离，可以登录不同的账号（应用身份/用户身份），调用时按需切换

### `"context \"default\" already exists"`

现象：

```bash
$ bk-cli context init --bk_api_url_tmpl="xxxx"
{
  "ok": false,
  "error": {
    "code": "create_context_failed",
    "message": "context \"default\" already exists"
  }
}
```

原因：这台机器上之前已经初始化并创建了名为 `default` 的 context。

可以查看 `~/.bk-cli/contexts` 目录，`cat ~/.bk-cli/contexts/default/config.yaml`：

- 如果只是想改 `bk_api_url_tmpl` 或超时时间，直接改文件即可
- 如果是想变更登录票据，直接执行 `bk-cli auth login ...`，无需重新初始化
- 或者确认可以删除后，删掉 `contexts/default` 目录，然后重新执行初始化

## auth

### 可以重复 auth login 吗？

可以。重复 `auth login` 会更新本地的 `~/.bk-cli/contexts/<context>/credentials.enc`。

### login 时会校验票据的合法性吗？

不会。目前只会加密存储；票据是否合法，在发起调用时由服务端校验。

### auth login 如何获取 bk_app_code 和 bk_app_secret?

访问 蓝鲸开发者中心-应用开发，搜索应用并进入应用管理页。

在应用管理页，展开左侧菜单**应用配置**，点击**基本信息**。tab 页面**密钥信息**中的 `bk_app_code` 和 `bk_app_secret`，即为访问云 API 所需的蓝鲸应用账号。

### auth login 如何获取 bk_ticket/bk_token?

用户登录蓝鲸后，浏览器 Cookies 中会存储用户登录凭证，此凭证即可代表用户身份。

使用 Chrome 浏览器，登录任意一个蓝鲸站点，F12 → Application → Cookies，搜索 `bk_ticket`（内部上云版）或 `bk_token`（其他环境）。

### auth login 需要的 access_token 如何获得？

https://bk.tencent.com/docs/markdown/ZH/APIGateway/1.17/UserGuide/Explanation/access-token.md

### 我可以使用非自己 app 签发的 access_token 吗？例如 aidev 或蓝鲸监控 MCP 签发的？

access_token = 应用 + 用户。

aidev 或蓝鲸监控的应用，只申请了自己系统的部分接口权限，只用于用户使用 aidev 或监控 MCP。

如果你用了，那么调用其他系统时，是以 aidev/监控的 `app_code` 身份调用，接口无权限且无法申请。

### 有没有可能就以个人 access_token 通用？有个人 access_token 其实就知道是谁了，就能知道这个人对某个系统有哪些权限

目前每个蓝鲸系统给到产品页面的接口，和接入网关的接口是不一样的，两套接口。产品页面只校验用户登录态；接入网关接口大部分校验了应用态，并且开启了应用权限认证，意味着需要提供应用身份，并且这个应用需要申请权限。

`bk-cli` 底层是调用各个系统接入网关的接口。

所以，如果你用 agent 使用 playwright 拉起浏览器，提供登录态，实际上走的是产品页面，有权限。

但是，使用 `bk-cli` 发起调用，实际上走的是各个系统接入网关的接口。此时**是否需要应用身份/是否需要申请权限，取决于各个系统的网关接口配置**。

如果：

1. 系统配置接口需要提升应用身份，但是不需要申请权限，那么将去掉环节【申请接口权限-审批】
2. 系统直接提供用户态接口，例如蓝盾 devops 网关中的用户态接口，那么将去掉环节【提供应用身份】，此时只需要一个登录态或任意 access_token 就能调用

`网关配合 bkauth/权限中心在设计新的方案，上线后用户将无需提供目前复杂的认证票据。`

### 当前的权限是怎么控制的？

- `bk_app_code` + `bk_app_secret` = 应用
- `bk_ticket` / `bk_token` = 用户
- `access_token` = 应用 + 用户

- 应用即开发者中心的应用，需要在开发者中心申请对应系统接口权限【接口调用权限】
- 用户即当前登录的用户身份，调用对应系统接口时，对应系统会拿用户去权限中心鉴权，有没有权限取决于在权限中心有没有权限【业务权限】

所以权限即为：对应应用申请到的 API 权限 + 用户在权限中心申请到的权限。

【目前已经在调研实现新的方案，未来 bk-cli 登录后将会有独立的身份及权限】

### 应用如何申请调用对应网关的权限？

访问 蓝鲸开发者中心-应用开发，搜索应用并进入应用管理页。

点击左侧菜单**云 API 权限**，进入云 API 权限管理页，切换到**网关 API** 页。

在网关列表中，筛选出待申请权限的网关，点击网关名，然后在右侧页面选中需访问的网关 API，点击**批量申请**。在申请记录中，可查看申请单详情。待权限审批通过后，即可访问网关 API。

**申请后**，在 网关 API 文档 查找对应网关的负责人，**联系负责人审批**。

### 支持设备码方式进行 login 吗？

目前规划中，未来会实现。

## api

### bk-cli api 能调用自己的非公开网关或接口吗？

可以。非公开只是不能通过 `bk-cli apigateway` 查询到，但是 `bk-cli api` 本质上是拼接一个 HTTP 请求发送，并不限制公开/非公开。

所以只要是已发布到网关的接口，都可以调用。

## apigateway

### bk-cli apigateway 为什么查不到我的网关或接口

`bk-cli apigateway` 只能查询到：公开的网关、公开的接口。

私有网关或不公开的接口，无法查询到。

### 通过 bk-cli apigateway 查询回来接口之后的调用为什么接口拼接参数不对？

`bk-cli apigateway` 查询回来的接口协议，是对应系统网关注册到蓝鲸 apigateway 的接口定义。

如果这份定义信息使用标准 OpenAPI 声明，有完整的请求参数声明，并且有完整的文档/example，那么 agent 拼接 `bk-cli api` 的参数非常准确，不会出错。

但是如果对应的接口没有提供 OpenAPI 声明，甚至连文档都没有，那么 agent 无法构造出合法的请求体，导致接口请求报错。

**完全取决于被调用系统的注册信息是否完善。**

如果发现不完善的，可以给对应网关管理员提需求，补齐接口声明。

### 作为网关管理员，如何提升 bk-cli 调用接口的准确性和成功率？

1. 修改网关的【描述】信息，包含网关的定位/能力/领域范围等，确保用户在使用 cli 探索的时候能找到这个网关
2. 确认网关的接口 OpenAPI 声明是否完整
   - 如果不完整，需要从页面或自动化导入 yaml 中，补齐接口的请求参数声明
3. 确认接口的文档是否完整
   - 需要包含接口的完整说明，特别是一些参数定义、枚举、特殊情况等

## 请求报错

### Parameters error [reason=\"\"]

```json
{
  "ok": false,
  "status": 400,
  "headers": {
    "Content-Type": "application/json; charset=utf-8",
    "X-Bkapi-Error-Code": "1640001",
    "X-Bkapi-Error-Message": "Parameters error [reason=\"\"]",
    "X-Bkapi-Request-Id": "5211965e-1133-479d-a360-2821c91cabba",
    "X-Request-Id": "a7994e08d2f63d32198231678663a790"
  },
  "data": {
    "code": 1640001,
    "code_name": "INVALID_ARGS",
    "data": null,
    "message": "Parameters error [reason=\"\"]",
    "result": false
  }
}
```

可能原因：

- `bk-cli auth login` 的参数，特别是 `--bk_token=""`，使用的双引号，当 `bk_token` 值中有 `$` 符号时，bash 会当成变量渲染，导致 `bk_token` 值错误。此时需要将**双引号**改成**单引号**规避渲染。
