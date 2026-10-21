---
name: bk-cli-aidev
description: 当需要通过 `bk-cli aidev` 查询或管理蓝鲸 AIDev 空间、Skill、MCP、Prompt、智能体、知识库、知识文档、凭证、资源引用，或上传/导入相关制品时使用。
---

# bk-cli aidev — 蓝鲸 AIDev 资源能力

用于通过 `bk-cli aidev` 调用蓝鲸 AIDev 私有 OpenAPI。

CRITICAL — 开始前 MUST 先读取 `../bk-cli-shared/SKILL.md`。共享 skill 负责认证、context、tenant、stage、dry-run、verbose、header/body 和通用请求规则；本 skill 只补充 `aidev` 命令自己的语义与输入约定。

## 当前覆盖范围

`bk-cli aidev` 提供 AIDev 资源查询与管理能力，包含空间、Skill、MCP、Prompt、智能体、知识库、知识文档、凭证、资源引用、资源集合和上传/导入等命令。

完整命令列表以 `bk-cli aidev --help` 为准，单个命令参数以 `bk-cli aidev <action> --help` 为准。

## 输入约定和易错点

- 调用 AIDev 前，按共享 skill 配置当前 context 的登录凭据，并确保身份具有对应空间和 API 权限。支持 Bearer 鉴权的部署可使用个人 token；使用应用与用户凭据时，需要同时提供两种身份并申请对应 API 权限。
- CLI flag 名保持上游 OpenAPI 参数名不变，例如 `space_id`、`skill_id`、`mcp_type`、`agent_code`、`page_size`。
- `list_skills`、`list_agents` 等 GET 列表的 `space_id` 可省略或传 `all`，表示查询当前租户内所有有权限空间；`list_knowledges` 必须在 body 中传真实空间 ID，不支持 `all`。
- 带复杂 request body 的 action 统一通过共享 `--body '<json>'` 传入；执行时要求请求体的 action 会在本地校验 `--body` 非空。
- 查看请求体结构时使用 `bk-cli aidev <action> -h --body-schema`。
- `upload_file` 是文件上传命令，使用 `--file`、`--space_id` 和可选 `--module` 发送 `multipart/form-data`，不使用 `--body`。

## 常用工作流

1. 用 `list_spaces` 确认可访问空间，记录目标 `space_id`。
2. 上传 Skill 或知识 ZIP 时，先用 `upload_file` 或 `get_upload_url` 得到制品地址，再调用 Skill upsert 或知识导入命令。
3. 管理 Skill、MCP、Prompt、智能体前，先用对应 `list_*` 命令定位 ID；写操作优先在测试空间或测试 stage 验证。
4. 管理智能体时，通常按“创建/更新 Agent → 绑定 Skill / MCP / KnowledgeBase → `publish_agent` 发布”的顺序操作。
5. 导入或重建知识后，用 `get_knowledge_status` 或 `check_upload_status` 查询处理状态。
6. 管理凭证时，先用 `list_credential_types` 确认凭证类型，再创建、绑定、解绑或删除凭证。

## Commands

### 空间

#### `list_spaces`

```bash
bk-cli aidev list_spaces
bk-cli aidev list_spaces --keyword demo
```

- 列出当前调用身份可访问的空间及 space_id，支持关键词过滤。适用于尚未确定目标空间时；该接口不授予空间权限。
- 不需要传 `space_id`；可用 `--keyword` 按空间名称模糊搜索。

### Skill

#### `list_skills`

```bash
bk-cli aidev list_skills --space_id demo-space --page 1 --page_size 20
bk-cli aidev list_skills --space_id all --fuzzy demo
```

- 按空间和 fuzzy 关键词筛选 Skill 并取得 ID；结果可能分页，调用方需逐项精确比对 skill_code。适用于同步前查重或 Agent Skill 引用解析。
- 常用筛选：`fuzzy`、`generate_type`、`group_type`、`tag_id`、`tag_name`、`page`、`page_size`。

#### `upsert_skill`

```bash
bk-cli aidev upsert_skill \
  --body '{"space_id":"demo-space","skill_code":"demo-skill","skill_name":"Demo Skill","description":"demo description","url":"bkrepo://demo.zip"}'
```

- 使用已上传的 Skill 制品按 skill_code 创建或更新 Skill。已有编码会更新；同版本是否可覆盖受 allow_version_overwrite 控制。适用于完成 ZIP 上传和版本核对后。
- 请求体必填：`space_id`、`skill_code`、`skill_name`、`description`、`url`。
- `url` 通常来自 `upload_file` 或 `get_upload_url` 的上传结果。
- 默认允许覆盖已存在的同版本；需要禁止时在 body 中传 `"allow_version_overwrite":false`。

#### `get_skill`

```bash
bk-cli aidev get_skill --skill_id 1 --space_id demo-space
bk-cli aidev get_skill --skill_id 1 --space_id demo-space --version v1.0.0
```

- 读取指定 Skill 的配置；传 version 时读取该版本。适用于同步前比对、确认版本或解析 Agent 引用。
- 必填：`skill_id`、`space_id`；`version` 不传时返回最新版本。

#### `archive_skill`

```bash
bk-cli aidev archive_skill --skill_id 1 --body '{"space_id":"demo-space"}'
```

- 归档指定 Skill。先核对目标 ID、版本和 Agent 引用，再执行。
- 必填：`skill_id` 和包含真实 `space_id` 的 DELETE 请求体；通过 `--body` 显式传入空间 ID，不支持 `all`。

#### `get_skill_download_url`

```bash
bk-cli aidev get_skill_download_url --skill_id 1 --space_id demo-space
```

- 获取指定 Skill 文件的临时下载链接。适用于核对或下载已有制品；链接有时效，不用于上传新版本。
- 必填：`skill_id`、`space_id`。

#### `list_skill_versions`

```bash
bk-cli aidev list_skill_versions --skill_id 1 --space_id demo-space
```

- 列出指定 Skill 的历史版本。适用于选择待查看或下载的版本；不改变当前版本。
- 必填：`skill_id`、`space_id`。

### MCP

#### `list_mcps`

```bash
bk-cli aidev list_mcps --space_id demo-space --mcp_type apigw
bk-cli aidev list_mcps --space_id all --fuzzy demo --page 1 --page_size 20
```

- 按空间、mcp_code 和类型列出 MCP 候选，结果可能分页。适用于发现资源；建立 Agent 引用时应改用 by-code 精确确认。
- `mcp_type` 可用于过滤 `apigw` 或 `resource`；未传时同时返回资源型和 APIGW MCP。
- 常用筛选：`agent_code`、`mcp_code`、`mcp_name`、`created_by`、`updated_by`、`tag_names`。

#### `create_mcp`

```bash
bk-cli aidev create_mcp \
  --body '{"space_id":"demo-space","mcp_name":"Demo MCP","mcp_code":"demo-mcp","url":"https://mcp.example.com/mcp","protocol":"streamable_http","credential_type":"custom","description":"demo description","credential_id":1}'
```

- 在指定空间创建 MCP 配置。适用于确需新建 MCP 时；Agent 引用已有 MCP 应先使用 by-code 精确查询。
- `url` 必须是 HTTP(S) MCP 服务地址，不能使用上传制品地址。`credential_type` 为 `custom` 时必须同时传 `credential_id`；`null` 不需要凭证 ID，`blueapps` 还要求蓝鲸网关 URL。
- 请求体结构以 `bk-cli aidev create_mcp -h --body-schema` 为准。

#### `get_mcp`

```bash
bk-cli aidev get_mcp --mcp_id 1 --mcp_type apigw --space_id demo-space
bk-cli aidev get_mcp --mcp_id 1 --mcp_type resource --space_id demo-space
```

- 按类型和 ID 读取 MCP 详情。适用于已取得准确 ID 后核对资源型或 APIGW MCP 配置。
- 必填：`mcp_id`、`mcp_type`、`space_id`；`mcp_type` 常见值为 `apigw` 或 `resource`。
- 查询 APIGW 授权相关 MCP 时可补 `--agent_code`。

#### `get_mcp_register_json`

```bash
bk-cli aidev get_mcp_register_json --mcp_id 1 --mcp_type apigw --space_id demo-space
```

- 获取指定类型和 ID 的 MCP 注册 JSON，供外部客户端注册工具；不创建 MCP，也不授权 Agent。
- 必填：`mcp_id`、`mcp_type`、`space_id`；查询 APIGW 授权相关 MCP 时可补 `--agent_code`。

#### `get_mcp_by_code`

```bash
bk-cli aidev get_mcp_by_code --mcp_code demo-mcp
bk-cli aidev get_mcp_by_code --mcp_code demo-mcp --space_id demo-space --mcp_type apigw --agent_code demo-agent
```

- 按 mcp_type 和 mcp_code 精确解析 MCP 的 ID、类型及可用范围。适用于 Agent 配置引用 MCP 时；传 agent_code 可按目标 Agent 的关联场景检查 APIGW MCP。
- 必填：`mcp_code`；`space_id` 可省略或传 `all`。

#### `get_resource_mcp`

```bash
bk-cli aidev get_resource_mcp --mcp_id 1 --space_id demo-space
```

- 读取指定资源型 MCP 的详情。适用于已知资源型 MCP ID 时核对配置。
- 必填：`mcp_id`、`space_id`。

### Prompt

#### `list_prompts`

```bash
bk-cli aidev list_prompts --space_id demo-space --page 1 --page_size 20
bk-cli aidev list_prompts --space_id all --prompt_code demo --prompt_name Demo
```

- 按空间和筛选条件列出 Prompt，结果可能分页。适用于定位已有 Prompt code 或 ID；Agent 的 inline Prompt 不需先创建独立 Prompt。
- 常用筛选：`fuzzy`、`prompt_code`、`prompt_name`、`generate_type`、`group_type`、`created_by`、`tag_id`、`tag_name`。

#### `upsert_prompt`

```bash
bk-cli aidev upsert_prompt \
  --body '{"space_id":"demo-space","prompt_code":"demo-prompt","prompt_name":"Demo Prompt","content":"demo content"}'
```

- 按当前空间的 prompt_code 创建或更新独立 Prompt。适用于确需维护可复用 Prompt 时；已有编码会更新线上内容。
- 请求体结构以 `bk-cli aidev upsert_prompt -h --body-schema` 为准。

#### `get_prompt`

```bash
bk-cli aidev get_prompt --prompt_id 1 --space_id demo-space
```

- 读取指定 Prompt 的详细配置。适用于复用或变更已有 Prompt 前核对内容。
- 必填：`prompt_id`、`space_id`。

### 智能体

#### `list_agents`

```bash
bk-cli aidev list_agents --space_id demo-space --page 1 --page_size 20
bk-cli aidev list_agents --space_id all --agent_code demo-agent --agent_name Demo
```

- 按空间、agent_code 或名称查找智能体并取得 ID；结果可能分页。适用于同步前判断资源是否已存在，不应将模糊匹配结果直接当作唯一目标。
- 常用筛选：`fuzzy`、`agent_code`、`agent_name`、`generate_type`、`created_by`、`tag_id`、`tag_name`。

#### `create_agent`

```bash
bk-cli aidev create_agent \
  --body '{"space_id":"demo-space","agent_name":"Demo Agent","agent_code":"demo-agent"}'
```

- 创建指定空间的智能体，返回供后续配置更新使用的 Agent ID。适用于该 agent_code 尚不存在时；创建可能联动开发者中心，请先确认空间、编码和名称。
- 请求体结构以 `bk-cli aidev create_agent -h --body-schema` 为准。

#### `get_agent`

```bash
bk-cli aidev get_agent --agent_id 1 --space_id demo-space
bk-cli aidev get_agent --agent_id 1 --space_id demo-space --version v1.0.0
```

- 读取指定智能体的配置：不传 version 返回当前草稿配置；传 version 返回指定已发布版本。适用于同步前比对草稿、核对依赖或读取已发布子智能体配置。
- 必填：`agent_id`、`space_id`；`version` 不传时返回当前草稿配置。

#### `archive_agent`

```bash
bk-cli aidev archive_agent --agent_id 1 --body '{"space_id":"demo-space"}'
```

- 归档指定智能体；已发布的插件应用可能异步下架。适用于明确要移除智能体时，先核对目标 ID 和资源引用。
- 必填：`agent_id` 和包含真实 `space_id` 的 DELETE 请求体；通过 `--body` 显式传入空间 ID，不支持 `all`。

#### `publish_agent`

```bash
bk-cli aidev publish_agent \
  --agent_id 1 \
  --body '{"space_id":"demo-space","agent_id":1}'
```

- 提交指定智能体的发布请求并返回版本与状态。建议不传 version，由平台自动生成：首次为 1.0.0，后续递增末位；仅在确需指定版本号时传入。pending 或 running 表示后台处理中，不等于发布或部署完成。
- 必填：`agent_id` 和请求体。

#### `update_agent`

```bash
bk-cli aidev update_agent \
  --agent_id 1 \
  --body '{"space_id":"demo-space","agent_id":1}'
```

- 更新已有智能体的配置，包括模型、Prompt、知识库、Skill 和 MCP 等引用。适用于完成依赖解析及差异核对后同步配置；会改变线上智能体。
- 必填：`agent_id` 和请求体。

#### `append_agent_admins`

```bash
bk-cli aidev append_agent_admins \
  --agent_id 1 \
  --body '{"space_id":"demo-space","agent_id":1,"admins":["username"]}'
```

- 向智能体对应的开发者中心插件追加管理员，保留原有成员；不修改 Agent 使用者或空间权限。适用于 Agent 创建后补充管理员。
- 必填：`agent_id` 和请求体；`admins` 必须包含要追加的用户名，省略、`null` 或 `[]` 不会追加任何人。调用方需要空间访问和智能体管理权限。

### 知识库

#### `create_knowledgebase`

```bash
bk-cli aidev create_knowledgebase --body '{"space_id":"demo-space","name":"demo"}'
```

- 在指定空间创建知识库配置并返回 ID。适用于 knowledgebase_code 不存在时；文档内容需另行上传和导入。
- 请求体结构以 `bk-cli aidev create_knowledgebase -h --body-schema` 为准。

#### `get_knowledge_stats`

```bash
bk-cli aidev get_knowledge_stats --space_id demo-space
```

- 读取指定空间内当前调用身份可访问的知识库与知识文档统计汇总。适用于了解数量和文件大小；不按单个知识库 ID 查询，也不返回完整配置。
- 必填：`space_id`。

#### `list_knowledgebases`

```bash
bk-cli aidev list_knowledgebases
```

- 按空间和 knowledgebase_code 筛选知识库并取得 ID；编码筛选为模糊匹配，调用方需逐项精确比对。这是只读查询，虽然使用 POST，结果可能分页。适用于同步前查重或 Agent 引用解析。
- 请求参数以 `bk-cli aidev list_knowledgebases --help` 为准。

#### `delete_knowledgebase`

```bash
bk-cli aidev delete_knowledgebase --id 1 --body '{"space_id":"demo-space"}'
```

- 删除指定知识库。先确认其中知识、Agent 引用及删除影响，再执行。
- 必填：`id` 和包含真实 `space_id` 的 DELETE 请求体；通过 `--body` 显式传入空间 ID，不支持 `all`。

#### `update_knowledgebase`

```bash
bk-cli aidev update_knowledgebase \
  --id 1 \
  --body '{"space_id":"demo-space","id":1}'
```

- 更新已有知识库的配置。适用于查到 ID 且完成配置差异核对后；不会单独完成知识文档 ZIP 导入。
- 必填：`id` 和请求体。

#### `query_knowledgebase`

```bash
bk-cli aidev query_knowledgebase --body '{"space_id":"demo-space","query":"hello"}'
```

- 在指定空间对知识库内容发起检索并返回命中结果。适用于回答知识问题；不用于查找知识库配置或同步文档。
- 请求体结构以 `bk-cli aidev query_knowledgebase -h --body-schema` 为准。

### 知识文档

#### `create_knowledge`

```bash
bk-cli aidev create_knowledge \
  --body '{"space_id":"demo-space","knowledge_base_id":1,"knowledge_name":"Demo Knowledge","file_name":"demo.md","file_path":"demo.md","url":"bkrepo://demo.md"}'
```

- 在指定知识库中创建一条知识文档。适用于单篇新增；批量或整目录 ZIP 同步应选择相应接口。
- 上传结果放在 `url`；`file_path` 是知识库内相对路径，文件名必须和 `file_name` 一致，例如 `demo.md`。
- 请求体结构以 `bk-cli aidev create_knowledge -h --body-schema` 为准。

#### `batch_create_knowledges`

```bash
bk-cli aidev batch_create_knowledges --body '{"space_id":"demo-space","items":[{"knowledge_base_id":1,"knowledge_name":"Demo Knowledge","file_name":"demo.md","file_path":"demo.md","url":"bkrepo://demo.md"}]}'
```

- 一次创建多条知识文档。适用于已确认同一批输入的多篇新增；整目录 ZIP 同步使用 archive/import。
- 要求 `--body`，结构以 `bk-cli aidev batch_create_knowledges -h --body-schema` 为准。

#### `batch_update_knowledges`

```bash
bk-cli aidev batch_update_knowledges --body '{"space_id":"demo-space","items":[{"id":1,"knowledge_name":"Updated Knowledge"}]}'
```

- 一次更新多条已有知识文档。适用于每条目标 ID 和修改内容均已确认的批量操作。
- 要求 `--body`，结构以 `bk-cli aidev batch_update_knowledges -h --body-schema` 为准。

#### `batch_delete_knowledges`

```bash
bk-cli aidev batch_delete_knowledges --body '{"space_id":"demo-space","ids":[1]}'
```

- 一次删除多条知识文档。适用于已逐项核对目标 ID 和删除影响的批量清理。
- 要求 `--body`，结构以 `bk-cli aidev batch_delete_knowledges -h --body-schema` 为准。

#### `list_knowledges`

```bash
bk-cli aidev list_knowledges --body '{"space_id":"demo-space"}'
```

- 列出指定知识库或目录内的知识文档；这是只读查询，虽然使用 POST。适用于确认导入后的文档范围或定位文档 ID。
- 该列表接口通过 body 传查询条件，`space_id` 必须是真实空间 ID，不能省略或传 `all`。

#### `preview_knowledge_link`

```bash
bk-cli aidev preview_knowledge_link --body '{"space_id":"demo-space","url":"bkrepo://demo.zip"}'
```

- 预览待导入远程链接的内容。适用于创建链接型知识前检查可读取性；不创建知识文档。
- 要求 `--body`，结构以 `bk-cli aidev preview_knowledge_link -h --body-schema` 为准。

#### `get_knowledge_structure_header`

```bash
bk-cli aidev get_knowledge_structure_header --body '{"space_id":"demo-space","url":"bkrepo://demo.zip"}'
```

- 读取远程文件的结构头信息，供结构化知识导入前确认字段；不创建或更新知识。
- 要求 `--body`，结构以 `bk-cli aidev get_knowledge_structure_header -h --body-schema` 为准。

#### `verify_knowledge_link`

```bash
bk-cli aidev verify_knowledge_link --body '{"space_id":"demo-space","url":"bkrepo://demo.zip","knowledgebase_type":"document"}'
```

- 校验待使用的知识链接是否有效。适用于创建或更新链接型知识前预检；不保存文档。
- 要求 `--body`，结构以 `bk-cli aidev verify_knowledge_link -h --body-schema` 为准。

#### `preview_knowledge`

```bash
bk-cli aidev preview_knowledge --id 1 --space_id demo-space
```

- 读取指定知识文档的预览内容。适用于核对正文，不用于获取完整索引配置。
- 必填：`id`、`space_id`。

#### `get_knowledge_status`

```bash
bk-cli aidev get_knowledge_status --space_id demo-space
bk-cli aidev get_knowledge_status --space_id demo-space --anchor_path /1 --group_type document
```

- 按空间和 anchor_path 查询知识处理状态统计及可见的 ZIP 导入任务。适用于导入前检查是否已有任务，或提交后查询进度；不触发重试。
- 必填：`space_id`；可补 `anchor_path`、`group_type`。

#### `trigger_knowledge_update`

```bash
bk-cli aidev trigger_knowledge_update --body '{"space_id":"demo-space","id":1}'
```

- 触发已有知识文档的更新任务。适用于明确需要重新抓取或更新时；可能产生后台处理。
- 不传 `anchor_path` 时必须提供文档 `id`；示例更新知识文档 1。
- 要求 `--body`，结构以 `bk-cli aidev trigger_knowledge_update -h --body-schema` 为准。

#### `list_knowledge_update_frequencies`

```bash
bk-cli aidev list_knowledge_update_frequencies
```

- 列出知识文档支持的更新频率选项，供创建或更新配置时选择；不触发更新。

#### `get_knowledge_index_info`

```bash
bk-cli aidev get_knowledge_index_info --id 1 --space_id demo-space
```

- 读取指定知识文档的索引配置。适用于核对切分、清洗和检索设置；不返回文档正文。
- 必填：`id`、`space_id`。

#### `reclean_knowledge`

```bash
bk-cli aidev reclean_knowledge --body '{"space_id":"demo-space","id":1}'
```

- 按当前或指定导入链路重新清洗并重建知识产物。适用于确认现有产物需重建时；会触发后台处理。
- 请求体中 `id` 或非空 `ids` 至少提供一个，选定待重新清洗的文档。
- 要求 `--body`，结构以 `bk-cli aidev reclean_knowledge -h --body-schema` 为准。

#### `delete_knowledge`

```bash
bk-cli aidev delete_knowledge --id 1 --body '{"space_id":"demo-space"}'
```

- 删除指定知识文档。适用于明确移除单篇文档时，先确认 ID 和影响。
- 必填：`id` 和包含真实 `space_id` 的 DELETE 请求体；通过 `--body` 显式传入空间 ID，不支持 `all`。

#### `update_knowledge`

```bash
bk-cli aidev update_knowledge \
  --id 1 \
  --body '{"space_id":"demo-space","id":1}'
```

- 更新指定知识文档的内容或配置。适用于已定位文档 ID 的单篇修改。
- 必填：`id` 和请求体。

#### `import_knowledge_archive`

```bash
bk-cli aidev import_knowledge_archive \
  --body '{"space_id":"demo-space","url":"bkrepo://demo.zip","file_name":"demo","file_type":"zip","anchor_path":"/1"}'
```

- 将已上传或远程 ZIP 异步导入指定 anchor_path，并覆盖目标目录内容。适用于目录化文档同步；返回任务 ID 后用相同 anchor_path 查询 status_info，不应把提交成功当作处理完成。
- `anchor_path` 必须为 `/<知识库ID>` 或 `/<知识库ID>/<目录>`，不能只传 `/`；示例 `/1` 指知识库 1。
- 导入进度可用相同 `anchor_path` 调用 `get_knowledge_status` 查询。

### 凭证

#### `list_credential_types`

```bash
bk-cli aidev list_credential_types
```

- 列出可创建的凭证类型和配置字段。适用于尚不清楚凭证类型或表单结构时；该接口不创建凭证。

#### `list_credentials`

```bash
bk-cli aidev list_credentials --space_id demo-space --page 1 --page_size 20
bk-cli aidev list_credentials --space_id all --credential_name demo --credential_types custom
```

- 按空间和筛选条件查找凭证配置，结果可能分页。适用于定位待绑定或待维护的凭证；不要将列表结果当作密钥明文。
- 常用筛选：`credential_name`、`credential_types`、`page`、`page_size`。

#### `create_credential`

```bash
bk-cli aidev create_credential \
  --body '{"credential_name":"demo-credential","credential_type":"custom","credential_detail":{"key":"demo-key","value":"demo-value"},"space_id":"demo-space"}'
```

- 在指定空间创建凭证配置，返回凭证 ID。仅在明确需要新凭证且字段已确认时调用；凭证生命周期与绑定关系分开管理。
- 先用 `list_credential_types` 确认类型字段；`custom` 的 `credential_detail` 需要 `key` 和 `value`。
- 请求体结构以 `bk-cli aidev create_credential -h --body-schema` 为准。

#### `update_credential`

```bash
bk-cli aidev update_credential \
  --credential_id 1 \
  --body '{"credential_name":"demo-credential","credential_type":"custom","credential_detail":{"key":"demo-key","value":"demo-value"},"space_id":"demo-space","credential_id":1}'
```

- 更新指定凭证的配置。适用于已核对凭证 ID、需要变更字段时；可能影响已绑定该凭证的资源。
- 必填：`credential_id` 和请求体。

#### `batch_delete_credentials`

```bash
bk-cli aidev batch_delete_credentials --body '{"space_id":"demo-space","credential_ids":[1]}'
```

- 批量删除指定凭证；先逐项确认 ID 与引用关系。适用于一次清理多个已确认不再使用的凭证。
- 要求 `--body`，结构以 `bk-cli aidev batch_delete_credentials -h --body-schema` 为准。

#### `delete_credential`

```bash
bk-cli aidev delete_credential --credential_id 1 --body '{"space_id":"demo-space"}'
```

- 删除指定凭证。先查询引用并确认解绑或影响范围，再执行删除。
- 必填：`credential_id` 和包含真实 `space_id` 的 DELETE 请求体；通过 `--body` 显式传入空间 ID，不支持 `all`。

#### `bind_credential`

```bash
bk-cli aidev bind_credential --body '{"space_id":"demo-space","credential_id":1,"resource_type":"mcp","resource_id":1}'
```

- 将已有凭证绑定到目标资源。适用于凭证和资源均已确定时；会改变目标资源的凭证关联。
- 要求 `--body`，结构以 `bk-cli aidev bind_credential -h --body-schema` 为准。

#### `unbind_credential`

```bash
bk-cli aidev unbind_credential --body '{"space_id":"demo-space","credential_id":1,"resource_type":"mcp","resource_id":1}'
```

- 解除凭证与目标资源的绑定。适用于明确撤销该关联时；可能影响目标资源调用外部服务。
- 要求 `--body`，结构以 `bk-cli aidev unbind_credential -h --body-schema` 为准。

#### `list_credential_references`

```bash
bk-cli aidev list_credential_references --credential_id 1 --space_id demo-space
```

- 查询指定凭证被哪些资源引用。适用于更新、解绑或删除前评估影响。
- 必填：`credential_id`、`space_id`。

### 标签

#### `list_tags`

```bash
bk-cli aidev list_tags
bk-cli aidev list_tags --scope_name BUSINESS
```

- 获取当前租户指定范围的全量标签树，供智能体配置时选择标签；无需指定空间或智能体。
- 可选：`scope_name`。省略时使用部署配置的智能体标签范围（默认 `default`），业务范围为 `BUSINESS`。`agent` 不是范围名；未识别的值会回退到通用默认范围。

### 资源引用与资源集合

#### `list_resource_references`

```bash
bk-cli aidev list_resource_references --instance_id 1 --resource_type skill --space_id demo-space
```

- 查询哪些 Agent 引用了指定类型和 ID 的资源。适用于删除或变更 Skill、MCP、知识库等资源前评估影响。
- 必填：`instance_id`、`resource_type`、`space_id`；`resource_type` 常见值包括 `agent`、`collection`、`knowledgebase`、`llm`、`mcp`、`skill`、`tool`。

#### `list_collections`

```bash
bk-cli aidev list_collections --space_id demo-space --collection_name Demo
bk-cli aidev list_collections --space_id all --collection_code demo-collection
```

- 按空间和 collection_code 查找角色 Prompt Collection，取得 collection_id；结果可能分页。适用于 Agent 引用解析或同步前查重。
- 常用筛选：`collection_code`、`collection_name`、`page`、`page_size`。

#### `upsert_collection`

```bash
bk-cli aidev upsert_collection \
  --body '{"space_id":"demo-space","collection_code":"demo-collection","collection_name":"Demo Collection","content":[{"role":"system","content":"你是示例角色"}]}'
```

- 按 collection_code 创建或更新角色 Prompt Collection，并返回 collection_id。适用于同步 Agent 所引用的角色指令；已有编码会更新线上内容。
- `content` 至少包含一条消息，第一条 `role` 必须为 `system`，消息 `content` 不能为空。
- 请求体结构以 `bk-cli aidev upsert_collection -h --body-schema` 为准。

#### `get_collection`

```bash
bk-cli aidev get_collection --collection_id 1 --space_id demo-space
```

- 读取指定角色 Prompt Collection 的消息内容及配置。适用于同步前比对，或确认 Agent 引用的现有 Collection。
- 必填：`collection_id`、`space_id`。

#### `apply_cross_space_resources`

```bash
bk-cli aidev apply_cross_space_resources \
  --body '{"space_id":"demo-space","resource_type":"skill","resource_ids":[1]}'
```

- 为指定资源发起跨空间使用申请，返回请求 ID 和审批工单信息。适用于目标空间尚无资源使用权限时；提交申请不等于授权已生效。
- 请求体结构以 `bk-cli aidev apply_cross_space_resources -h --body-schema` 为准。

### 上传和导入

#### `upload_file`

```bash
bk-cli aidev upload_file --file ./artifact.zip --space_id demo-space
bk-cli aidev upload_file --file ./knowledge.zip --space_id demo-space --module knowledge
```

- 直接上传本地文件，返回路径、元数据和过期时间。
- 必填：`file`、`space_id`；`module` 默认 `skill`，常见值为 `skill` 或 `knowledge`。
- 不使用 `--body`。文件必须存在且为可读的常规文件；上传时流式读取，`--dry-run` 会校验路径并显示文件大小 `size`（字节）。

#### `get_upload_url`

```bash
bk-cli aidev get_upload_url --body '{"space_id":"demo-space","file_name":"demo.zip","module":"skill"}'
```

- 为 Skill 或知识 ZIP 申请限时、一次性的直传地址。适用于大文件直传；取得地址后仍需上传文件并校验状态。
- `module` 默认 `knowledge`，Skill 直传必须显式指定 `"module":"skill"`；知识 ZIP 使用 `"module":"knowledge"`。
- 返回的 `upload_url` 只用于一次性上传；上传成功后，用返回的 `url` 调用 Skill upsert、知识 ZIP 导入或 `check_upload_status`。

#### `check_upload_status`

```bash
bk-cli aidev check_upload_status --body '{"space_id":"demo-space","url":"bkrepo://demo.zip"}'
```

- 查询并校验直传文件是否存在、大小范围和 SHA256 格式，返回实际文件大小与摘要。调用方仍需与本地文件比对；适用于上传后或超时后确认结果，不会重新上传。
- 上传失败或超时后可查询；文件不完整时上游返回错误，不会自动重新上传。

## 排障建议

- 先用 `--dry-run` 确认请求 URL、stage、query 和 body 是否符合预期。
- 如果报权限错误，同时检查当前登录身份和应用是否已经申请该 AIDev API 权限。
- 如果缺少请求体，先运行 `bk-cli aidev <action> -h --body-schema` 查看字段，再补 `--body`。
