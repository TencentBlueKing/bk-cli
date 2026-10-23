---
name: bk-cli-mcp
description: 当用户希望通过 bk-cli 连接远程 MCP server、检查连接、发现工具、查看工具 Schema、调用指定工具或排查 MCP 认证和未知执行结果时使用。
---

# bk-cli MCP 使用指引

## 适用范围与准备

使用 `bk-cli mcp <server>` 连接配置中的 Streamable HTTP server。先确认用户授权调用的目标和工具；发现工具不意味着授权调用全部工具。有副作用的工具按用户的业务授权执行。

默认配置为 `~/.bk-cli/mcp.json`，或 `BK_CLI_CONFIG_DIR` 下的 `mcp.json`；`--config <path>` 只选一个指定文件，不合并。配置必须是严格 JSON，每个 server 都必须显式填写 `"type": "http"`。从其他客户端迁移时，应另存一份只含 HTTP 服务的配置并补齐 type；不能直接使用含 stdio 条目的文件：

```json
{
  "mcpServers": {
    "demo": {
      "type": "http",
      "url": "https://mcp.example.com/demo/mcp/",
      "headers": {"Authorization": "Bearer your-personal-token"}
    }
  }
}
```

认证只来自所选 server 的 headers，配置字符串就是最终发送值，不支持环境变量展开或模板渲染；不读取 context、auth login 缓存、`.env` 或其他客户端配置。显式 `--context` 会报错，`--header/--stage/--tenant` 不支持。不要通过反复登录 bk-cli 解决 MCP 身份问题。

除个人 Bearer 外，可将 `X-Bkapi-Authorization` 设为完整认证 JSON 字符串（配置中转义内层引号）：应用 code/secret + ticket、应用 code/secret + token、或独立 access_token。不支持仅含应用 code/secret 的组合。不要同时配置两种认证头。无认证 server 可省略 headers，自定义 API key 也写在 headers 中。参见[用户指南](../../docs/user-guide.md#11-远程-mcp-工具)的完整示例。

## 推荐流程

```bash
# 本地帮助，只展示用法，不列出服务或连接远端
bk-cli mcp
bk-cli mcp demo

# 列出当前配置的服务（只读本地文件，全部 header 值脱敏）
bk-cli mcp --list
bk-cli mcp --config ./mcp.json --list

# 检查并发现；只读取目录，不调用业务工具
bk-cli mcp demo --check
bk-cli mcp demo --tools
bk-cli mcp demo search --help

# 确认 Schema 后显式调用一个工具
bk-cli mcp demo search --keyword '网关错误' --limit 20 --enabled=false
bk-cli mcp demo search --body '{"keyword":"网关错误","limit":20}'
bk-cli mcp demo search --body @params.json
bk-cli mcp demo search --body - < params.json

# 最终结果筛选
bk-cli mcp demo --tools --jq '.data.tools[].name'
bk-cli mcp demo search --body @params.json --jq '.data.result'

# 离线预览，必须有完整 body，无参数工具用 {}
bk-cli mcp demo search --body @params.json --dry-run
```

server、tool 和字段名使用配置/发现返回的原名，不改写大小写、下划线或点号。未知工具应先看完整目录，不猜名称。工具帮助联网但不执行工具或读取 stdin。

- 字符串保持字符串；布尔值显式使用 `=true/false`，数组和对象使用 JSON。
- 只发送明确提供的参数，不依赖 Schema default 自动补值。
- body 是完整 arguments 对象，不是 HTTP/JSON-RPC 包装。body 覆盖全部具名参数，不合并；未知或重复 flag 仍报错。
- 复杂 Schema、保留名（如 `help/body/config`）或无法表示为 flag 的字段通过 body 原名传入。
- `--list` 不接受 server/tool 或其他执行模式；`--check`、`--tools` 与工具调用互斥。离线 dry-run 不接受具名工具参数或 `--help`，也不证明远端 Schema、工具存在性或权限。
- 默认总超时 60 秒，`--timeout 90s` 覆盖整次连接、所有分页和调用。`--verbose` 输出脱敏诊断；`--insecure` 只在明确需要时使用。

## 结果判断与恢复

本地错误写 stderr、退出 1；远程失败写 stdout 的一个 `ok:false` envelope、退出 2。成功结果位于 `data.result`，其中 text 不自动解释为业务 JSON。`ok:true` 不证明异步任务完成或任意业务字段表示成功；check 成功也不证明所有工具均有权限。

- `invalid_mcp_config`：按错误中的 server 和字段修正配置；任意条目结构不合规都会使整份文件校验失败。
- `invalid_mcp_auth`：所选 server 的认证格式不受支持，按错误列出的字段组合修正；`--list` 只检查结构，成功不代表认证有效。
- `mcp_rpc_error`：在 `data.rpc_error` 查看服务端 code/message，区分参数或工具错误与协议错误；不自动重发。
- `mcp_auth_required`：检查所选 server 配置中的 headers，不自动 OAuth 登录或更换身份。
- `mcp_permission_denied`：检查服务、工具与业务权限，不一律归因于票据过期。
- `mcp_tool_not_found` / `invalid_mcp_arguments`：本地校验失败，没有调用工具；重新查看工具帮助。
- `mcp_capability_unsupported`：保留输入要求/任务状态供人工或外层 Agent 判断；一期不支持 input_required 接续、sampling、elicitation、异步轮询，不能直接重发原调用。
- **`data.execution_state: "unknown"`**：请求可能已执行，完整响应未取得。先用业务查询核实，避免重复写操作。
- jq/输出失败：读取保留的原结果；展示失败不表示请求失败，不能盲目重试。失败 envelope 不被 jq 隐藏。

CLI 不自动重发 `tools/call`。固定上限为单个 JSON/SSE 事件 16 MiB、累计目录 32 MiB、100 页，超限明确失败。JSON 数字在参数、目录、结果和简单 jq 字段提取中保持精度；jq 计算语义见[共享指引](../bk-cli-shared/SKILL.md#内置---jq-结果投影)。原始业务结果不承诺通用敏感内容过滤。

仅支持远程 Streamable HTTP，不支持 stdio、旧 HTTP+SSE、OAuth 自动登录或自动导入其他客户端配置。实现和扩展请转到 [AGENTS.md](../../AGENTS.md) 与[设计文档](../../docs/design.md)，不要将使用示例作为协议实现说明。
