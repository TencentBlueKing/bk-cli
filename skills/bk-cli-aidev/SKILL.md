---
name: bk-cli-aidev
description: 当需要通过 `bk-cli aidev` 查询或管理蓝鲸 AIDev 空间、Skill、MCP、Prompt、智能体、知识库、知识文档、凭证、资源引用，或上传/导入相关制品时使用。
---

# bk-cli aidev — 蓝鲸 AIDev 资源能力

用于通过 `bk-cli aidev` 调用蓝鲸 AIDev 私有 OpenAPI。

CRITICAL — 开始前 MUST 先读取 `../bk-cli-shared/SKILL.md`。共享 skill 负责认证、context、tenant、stage、dry-run、verbose、header/body 和通用请求规则；本 skill 只补充 `aidev` 命令自己的语义与输入约定。

## 当前覆盖范围

`bk-cli aidev` 覆盖附件 OpenAPI 中的 AIDev 资源接口，包含空间、Skill、MCP、Prompt、智能体、知识库、知识文档、凭证、资源引用、资源集合和上传/导入等命令。

完整命令列表以 `bk-cli aidev --help` 为准，单个命令参数以 `bk-cli aidev <action> --help` 为准。

## 输入约定和易错点

- 所有 AIDev action 都要求应用认证 + 用户认证 + 接口资源权限。调用前需要确保当前 context 的凭据完整，并且应用已申请对应 API 权限。
- CLI flag 名保持上游 OpenAPI 参数名不变，例如 `space_id`、`skill_id`、`mcp_type`、`agent_code`、`page_size`。
- 列表类命令的 `space_id` 通常可省略或传 `all`，表示查询当前租户内所有有权限空间；传具体 ID 时只查询该空间。
- 带复杂 request body 的 action 统一通过共享 `--body '<json>'` 传入；执行时要求请求体的 action 会在本地校验 `--body` 非空。
- 查看请求体结构时使用 `bk-cli aidev <action> -h --body-schema`。
- `create_private_upload` 是文件上传命令，使用 `--file`、`--space_id` 和可选 `--module` 发送 `multipart/form-data`，不使用 `--body`。

## 常用工作流

1. 用 `retrieve_private_spaces` 确认可访问空间，记录目标 `space_id`。
2. 上传 Skill 或知识 ZIP 时，先用 `create_private_upload` 或 `create_private_upload_url` 得到制品地址，再调用 Skill upsert 或知识导入命令。
3. 管理 Skill、MCP、Prompt、智能体前，先用对应 `list_*` 命令定位 ID；写操作优先在测试空间或测试 stage 验证。
4. 管理智能体时，通常按“创建/更新 Agent → 绑定 Skill / MCP / KnowledgeBase → `publish_private_agents` 发布”的顺序操作。
5. 导入或重建知识后，用 `status_info_private_knowledges` 或 `create_private_upload_status` 查询处理状态。
6. 管理凭证时，先用 `list_credentiale_types` 确认凭证类型，再创建、绑定、解绑或删除凭证。

## Commands

### 空间

#### `retrieve_private_spaces`

```bash
bk-cli aidev retrieve_private_spaces
bk-cli aidev retrieve_private_spaces --keyword demo
```

- 返回当前用户在当前租户内有权限访问的空间。
- 不需要传 `space_id`；可用 `--keyword` 按空间名称模糊搜索。

### Skill

#### `list_private_skills`

```bash
bk-cli aidev list_private_skills --space_id demo-space --page 1 --page_size 20
bk-cli aidev list_private_skills --space_id all --fuzzy demo
```

- 查询 Skill 列表；`space_id` 省略或传 `all` 时查所有有权限空间。
- 常用筛选：`fuzzy`、`generate_type`、`group_type`、`tag_id`、`tag_name`、`page`、`page_size`。

#### `create_private_skills_upsert`

```bash
bk-cli aidev create_private_skills_upsert \
  --body '{"space_id":"demo-space","skill_code":"demo-skill","skill_name":"Demo Skill","description":"demo description","url":"bkrepo://demo.zip"}'
```

- 按 `skill_code` 创建或更新 Skill；不存在则创建，存在则更新。
- 请求体必填：`space_id`、`skill_code`、`skill_name`、`description`、`url`。
- `url` 通常来自 `create_private_upload` 或 `create_private_upload_url` 的上传结果。
- 默认允许覆盖已存在的同版本；需要禁止时在 body 中传 `"allow_version_overwrite":false`。

#### `retrieve_private_skills`

```bash
bk-cli aidev retrieve_private_skills --skill_id 1 --space_id demo-space
bk-cli aidev retrieve_private_skills --skill_id 1 --space_id demo-space --version v1.0.0
```

- 获取指定 Skill 详情。
- 必填：`skill_id`、`space_id`；`version` 不传时返回最新版本。

#### `delete_private_skills`

```bash
bk-cli aidev delete_private_skills --skill_id 1
```

- 归档指定 Skill。
- 必填：`skill_id`。

#### `retrieve_private_skills_download`

```bash
bk-cli aidev retrieve_private_skills_download --skill_id 1 --space_id demo-space
```

- 获取指定 Skill 文件的临时下载链接。
- 必填：`skill_id`、`space_id`。

#### `retrieve_private_skills_versions`

```bash
bk-cli aidev retrieve_private_skills_versions --skill_id 1 --space_id demo-space
```

- 获取指定 Skill 的历史版本列表。
- 必填：`skill_id`、`space_id`。

### MCP

#### `list_private_mcps`

```bash
bk-cli aidev list_private_mcps --space_id demo-space --mcp_type apigw
bk-cli aidev list_private_mcps --space_id all --fuzzy demo --page 1 --page_size 20
```

- 查询 MCP 列表；`space_id` 省略或传 `all` 时查所有有权限空间。
- `mcp_type` 可用于过滤 `apigw` 或 `resource`；未传时同时返回资源型和 APIGW MCP。
- 常用筛选：`agent_code`、`mcp_code`、`mcp_name`、`created_by`、`updated_by`、`tag_names`。

#### `create_private_mcps`

```bash
bk-cli aidev create_private_mcps \
  --body '{"space_id":"demo-space","mcp_name":"Demo MCP","mcp_code":"demo-mcp","url":"bkrepo://demo.zip","protocol":"stdio","credential_type":"api_key","description":"demo description"}'
```

- 在当前用户有创建权限的空间中创建 MCP。
- 请求体结构以 `bk-cli aidev create_private_mcps -h --body-schema` 为准。

#### `retrieve_private_mcps`

```bash
bk-cli aidev retrieve_private_mcps --mcp_id 1 --mcp_type apigw --space_id demo-space
bk-cli aidev retrieve_private_mcps --mcp_id 1 --mcp_type resource --space_id demo-space
```

- 获取指定 MCP 详情。
- 必填：`mcp_id`、`mcp_type`、`space_id`；`mcp_type` 常见值为 `apigw` 或 `resource`。
- 查询 APIGW 授权相关 MCP 时可补 `--agent_code`。

#### `retrieve_private_mcps_register_json`

```bash
bk-cli aidev retrieve_private_mcps_register_json --mcp_id 1 --mcp_type apigw --space_id demo-space
```

- 获取指定 MCP 的注册 JSON 结构，可直接用于注册 MCP 工具。
- 必填：`mcp_id`、`mcp_type`、`space_id`；查询 APIGW 授权相关 MCP 时可补 `--agent_code`。

#### `retrieve_private_mcps_by_code`

```bash
bk-cli aidev retrieve_private_mcps_by_code --mcp_code demo-mcp
bk-cli aidev retrieve_private_mcps_by_code --mcp_code demo-mcp --space_id demo-space --mcp_type apigw --agent_code demo-agent
```

- 按 MCP 编码精确查询，要求结果唯一。
- 必填：`mcp_code`；`space_id` 可省略或传 `all`。

#### `retrieve_private_resource_mcps`

```bash
bk-cli aidev retrieve_private_resource_mcps --mcp_id 1 --space_id demo-space
```

- 获取指定资源型 MCP 的详情。
- 必填：`mcp_id`、`space_id`。

### Prompt

#### `list_private_prompts`

```bash
bk-cli aidev list_private_prompts --space_id demo-space --page 1 --page_size 20
bk-cli aidev list_private_prompts --space_id all --prompt_code demo --prompt_name Demo
```

- 查询 Prompt 列表；`space_id` 省略或传 `all` 时查所有有权限空间。
- 常用筛选：`fuzzy`、`prompt_code`、`prompt_name`、`generate_type`、`group_type`、`created_by`、`tag_id`、`tag_name`。

#### `upsert_private_prompts`

```bash
bk-cli aidev upsert_private_prompts \
  --body '{"space_id":"demo-space","prompt_code":"demo-prompt","prompt_name":"Demo Prompt","content":"demo content"}'
```

- 按 `prompt_code` 创建或更新 Prompt。
- 请求体结构以 `bk-cli aidev upsert_private_prompts -h --body-schema` 为准。

#### `retrieve_private_prompts`

```bash
bk-cli aidev retrieve_private_prompts --prompt_id 1 --space_id demo-space
```

- 获取指定 Prompt 详情。
- 必填：`prompt_id`、`space_id`。

### 智能体

#### `list_private_agents`

```bash
bk-cli aidev list_private_agents --space_id demo-space --page 1 --page_size 20
bk-cli aidev list_private_agents --space_id all --agent_code demo-agent --agent_name Demo
```

- 查询智能体列表；`space_id` 省略或传 `all` 时查所有有权限空间。
- 常用筛选：`fuzzy`、`agent_code`、`agent_name`、`generate_type`、`created_by`、`tag_id`、`tag_name`。

#### `create_private_agents`

```bash
bk-cli aidev create_private_agents \
  --body '{"space_id":"demo-space","agent_name":"Demo Agent","agent_code":"demo-agent"}'
```

- 在当前用户有创建权限的空间中创建智能体。
- 请求体结构以 `bk-cli aidev create_private_agents -h --body-schema` 为准。

#### `retrieve_private_agents`

```bash
bk-cli aidev retrieve_private_agents --agent_id 1 --space_id demo-space
bk-cli aidev retrieve_private_agents --agent_id 1 --space_id demo-space --version v1.0.0
```

- 获取指定智能体详情。
- 必填：`agent_id`、`space_id`；`version` 不传时返回当前草稿配置。

#### `delete_private_agents`

```bash
bk-cli aidev delete_private_agents --agent_id 1
```

- 归档指定智能体；已发布的插件应用会由上游异步下架。
- 必填：`agent_id`。

#### `publish_private_agents`

```bash
bk-cli aidev publish_private_agents \
  --agent_id 1 \
  --body '{"space_id":"demo-space","agent_id":1}'
```

- 发布智能体配置；不传版本号时由上游自动递增。
- 必填：`agent_id` 和请求体。

#### `update_private_agents`

```bash
bk-cli aidev update_private_agents \
  --agent_id 1 \
  --body '{"space_id":"demo-space","agent_id":1}'
```

- 更新智能体名称、描述、模型配置、提示词、知识库、工具、MCP 等配置。
- 必填：`agent_id` 和请求体。

#### `append_private_agent_admins`

```bash
bk-cli aidev append_private_agent_admins \
  --agent_id 1 \
  --body '{"space_id":"demo-space","agent_id":1}'
```

- 追加 PaaS 插件管理员；保留原成员，不修改使用者或空间 IAM 权限。
- 必填：`agent_id` 和请求体；调用方需要空间访问和智能体管理权限。

### 知识库

#### `create_private_knowledgebase`

```bash
bk-cli aidev create_private_knowledgebase --body '{"space_id":"demo-space","name":"demo"}'
```

- 创建新的知识库。
- 请求体结构以 `bk-cli aidev create_private_knowledgebase -h --body-schema` 为准。

#### `info_private_knowledgebase`

```bash
bk-cli aidev info_private_knowledgebase --space_id demo-space
```

- 获取知识库的知识统计信息。
- 必填：`space_id`。

#### `list_private_knowledgebase`

```bash
bk-cli aidev list_private_knowledgebase
```

- 查询知识库列表。
- 请求参数以 `bk-cli aidev list_private_knowledgebase --help` 为准。

#### `delete_private_knowledgebase`

```bash
bk-cli aidev delete_private_knowledgebase --id 1
```

- 删除指定知识库。
- 必填：`id`。

#### `update_private_knowledgebase`

```bash
bk-cli aidev update_private_knowledgebase \
  --id 1 \
  --body '{"space_id":"demo-space","id":1}'
```

- 更新指定知识库。
- 必填：`id` 和请求体。

#### `query_private_knowledgebase`

```bash
bk-cli aidev query_private_knowledgebase --body '{"space_id":"demo-space","query":"hello"}'
```

- 基于当前用户身份和指定空间发起知识库查询。
- 请求体结构以 `bk-cli aidev query_private_knowledgebase -h --body-schema` 为准。

### 知识文档

#### `create_private_knowledges`

```bash
bk-cli aidev create_private_knowledges \
  --body '{"space_id":"demo-space","knowledge_base_id":1,"knowledge_name":"Demo Knowledge","file_name":"demo.md","file_path":"bkrepo://demo.md"}'
```

- 创建新的知识文档。
- 请求体结构以 `bk-cli aidev create_private_knowledges -h --body-schema` 为准。

#### `batch_create_private_knowledges`

```bash
bk-cli aidev batch_create_private_knowledges --body '{"space_id":"demo-space","items":[]}'
```

- 批量创建多个知识文档。
- 要求 `--body`，结构以 `bk-cli aidev batch_create_private_knowledges -h --body-schema` 为准。

#### `batch_update_private_knowledges`

```bash
bk-cli aidev batch_update_private_knowledges --body '{"space_id":"demo-space","items":[]}'
```

- 批量更新多个知识文档。
- 要求 `--body`，结构以 `bk-cli aidev batch_update_private_knowledges -h --body-schema` 为准。

#### `batch_delete_private_knowledges`

```bash
bk-cli aidev batch_delete_private_knowledges --body '{"space_id":"demo-space","ids":[]}'
```

- 批量删除多个知识文档。
- 要求 `--body`，结构以 `bk-cli aidev batch_delete_private_knowledges -h --body-schema` 为准。

#### `list_private_knowledges`

```bash
bk-cli aidev list_private_knowledges --body '{"space_id":"demo-space"}'
```

- 获取指定知识库下的知识列表。
- 该列表接口通过 body 传查询条件。

#### `preview_link_private_knowledges`

```bash
bk-cli aidev preview_link_private_knowledges --body '{"space_id":"demo-space","url":"bkrepo://demo.zip"}'
```

- 预览远程链接内容。
- 要求 `--body`，结构以 `bk-cli aidev preview_link_private_knowledges -h --body-schema` 为准。

#### `structure_header_private_knowledges`

```bash
bk-cli aidev structure_header_private_knowledges --body '{"space_id":"demo-space","url":"bkrepo://demo.zip"}'
```

- 获取远程文件的结构头信息。
- 要求 `--body`，结构以 `bk-cli aidev structure_header_private_knowledges -h --body-schema` 为准。

#### `verify_private_knowledges`

```bash
bk-cli aidev verify_private_knowledges --body '{"space_id":"demo-space","url":"bkrepo://demo.zip","knowledgebase_type":"document"}'
```

- 校验知识链接是否有效。
- 要求 `--body`，结构以 `bk-cli aidev verify_private_knowledges -h --body-schema` 为准。

#### `preview_private_knowledges`

```bash
bk-cli aidev preview_private_knowledges --id 1 --space_id demo-space
```

- 预览指定知识内容。
- 必填：`id`、`space_id`。

#### `status_info_private_knowledges`

```bash
bk-cli aidev status_info_private_knowledges --space_id demo-space
bk-cli aidev status_info_private_knowledges --space_id demo-space --anchor_path / --group_type document
```

- 获取知识状态统计。
- 必填：`space_id`；可补 `anchor_path`、`group_type`。

#### `trigger_private_knowledges`

```bash
bk-cli aidev trigger_private_knowledges --body '{"space_id":"demo-space"}'
```

- 触发知识更新操作。
- 要求 `--body`，结构以 `bk-cli aidev trigger_private_knowledges -h --body-schema` 为准。

#### `update_frequency_type_private_knowledges`

```bash
bk-cli aidev update_frequency_type_private_knowledges
```

- 获取知识更新频率类型列表。

#### `index_info_private_knowledges`

```bash
bk-cli aidev index_info_private_knowledges --id 1 --space_id demo-space
```

- 获取指定知识的索引配置。
- 必填：`id`、`space_id`。

#### `reclean_private_knowledges`

```bash
bk-cli aidev reclean_private_knowledges --body '{"space_id":"demo-space"}'
```

- 按当前或指定导入链路重建知识产物。
- 要求 `--body`，结构以 `bk-cli aidev reclean_private_knowledges -h --body-schema` 为准。

#### `delete_private_knowledges`

```bash
bk-cli aidev delete_private_knowledges --id 1
```

- 删除指定知识。
- 必填：`id`。

#### `update_private_knowledges`

```bash
bk-cli aidev update_private_knowledges \
  --id 1 \
  --body '{"space_id":"demo-space","id":1}'
```

- 更新指定知识内容。
- 必填：`id` 和请求体。

#### `archive_import_private_knowledges`

```bash
bk-cli aidev archive_import_private_knowledges \
  --body '{"space_id":"demo-space","url":"bkrepo://demo.zip","file_name":"demo.md","file_type":"demo-file_type","anchor_path":"/"}'
```

- 异步导入上传或远程 ZIP，覆盖目标目录内容。
- 导入进度可用相同 `anchor_path` 调用 `status_info_private_knowledges` 查询。

### 凭证

#### `list_credentiale_types`

```bash
bk-cli aidev list_credentiale_types
```

- 获取所有可用的凭证类型及其配置参数。

#### `list_credentials`

```bash
bk-cli aidev list_credentials --space_id demo-space --page 1 --page_size 20
bk-cli aidev list_credentials --space_id all --credential_name demo --credential_types api_key
```

- 查询凭证列表；`space_id` 省略或传 `all` 时查所有有权限空间。
- 常用筛选：`credential_name`、`credential_types`、`page`、`page_size`。

#### `create_credentials`

```bash
bk-cli aidev create_credentials \
  --body '{"credential_name":"demo-credential","credential_type":"api_key","credential_detail":"demo-credential-detail","space_id":"demo-space"}'
```

- 创建新的凭证配置。
- 请求体结构以 `bk-cli aidev create_credentials -h --body-schema` 为准。

#### `update_credentials`

```bash
bk-cli aidev update_credentials \
  --credential_id 1 \
  --body '{"credential_name":"demo-credential","credential_type":"api_key","credential_detail":"demo-credential-detail","space_id":"demo-space","credential_id":1}'
```

- 更新指定凭证配置。
- 必填：`credential_id` 和请求体。

#### `batch_delete_credentials`

```bash
bk-cli aidev batch_delete_credentials --body '{"space_id":"demo-space","credential_ids":[]}'
```

- 批量删除凭证。
- 要求 `--body`，结构以 `bk-cli aidev batch_delete_credentials -h --body-schema` 为准。

#### `delete_credentials`

```bash
bk-cli aidev delete_credentials --credential_id 1
```

- 按 `credential_id` 删除单个凭证。
- 必填：`credential_id`。

#### `bind_credentials`

```bash
bk-cli aidev bind_credentials --body '{"space_id":"demo-space","credential_id":1,"resource_type":"skill","resource_id":1}'
```

- 将凭证绑定到指定资源。
- 要求 `--body`，结构以 `bk-cli aidev bind_credentials -h --body-schema` 为准。

#### `unbind_credentials`

```bash
bk-cli aidev unbind_credentials --body '{"space_id":"demo-space","credential_id":1,"resource_type":"skill","resource_id":1}'
```

- 解绑凭证与资源的关联。
- 要求 `--body`，结构以 `bk-cli aidev unbind_credentials -h --body-schema` 为准。

#### `list_references_credentials`

```bash
bk-cli aidev list_references_credentials --credential_id 1 --space_id demo-space
```

- 获取凭证被哪些资源引用。
- 必填：`credential_id`、`space_id`。

### 资源引用与资源集合

#### `list_private_resource_references`

```bash
bk-cli aidev list_private_resource_references --instance_id 1 --resource_type skill --space_id demo-space
```

- 按资源类型和实例 ID 查询引用该资源的 Agent。
- 必填：`instance_id`、`resource_type`、`space_id`；`resource_type` 常见值包括 `agent`、`collection`、`knowledgebase`、`llm`、`mcp`、`skill`、`tool`。

#### `list_private_collections`

```bash
bk-cli aidev list_private_collections --space_id demo-space --collection_name Demo
bk-cli aidev list_private_collections --space_id all --collection_code demo-collection
```

- 查询资源集合列表；`space_id` 省略或传 `all` 时查所有有权限空间。
- 常用筛选：`collection_code`、`collection_name`、`page`、`page_size`。

#### `upsert_private_collections`

```bash
bk-cli aidev upsert_private_collections \
  --body '{"space_id":"demo-space","collection_code":"demo-collection","collection_name":"Demo Collection","content":[]}'
```

- 创建或更新资源集合。
- 请求体结构以 `bk-cli aidev upsert_private_collections -h --body-schema` 为准。

#### `detail_private_collections`

```bash
bk-cli aidev detail_private_collections --collection_id 1 --space_id demo-space
```

- 查询资源集合详情。
- 必填：`collection_id`、`space_id`。

#### `create_private_space_manage_apply_resources`

```bash
bk-cli aidev create_private_space_manage_apply_resources \
  --body '{"space_id":"demo-space","resource_type":"skill","resource_ids":[]}'
```

- 发起用户态跨空间资源申请。
- 请求体结构以 `bk-cli aidev create_private_space_manage_apply_resources -h --body-schema` 为准。

### 上传和导入

#### `create_private_upload`

```bash
bk-cli aidev create_private_upload --file ./artifact.zip --space_id demo-space
bk-cli aidev create_private_upload --file ./knowledge.zip --space_id demo-space --module knowledge
```

- 直接上传本地文件，返回路径、元数据和过期时间。
- 必填：`file`、`space_id`；`module` 默认 `skill`，常见值为 `skill` 或 `knowledge`。
- 不使用 `--body`。

#### `create_private_upload_url`

```bash
bk-cli aidev create_private_upload_url --body '{"space_id":"demo-space","file_name":"demo.md"}'
```

- 创建可上传一次的临时上传 URL。
- 文件直传后再分别调用知识 ZIP 导入或 Skill upsert。

#### `create_private_upload_status`

```bash
bk-cli aidev create_private_upload_status --body '{"space_id":"demo-space","url":"bkrepo://demo.zip"}'
```

- 查询 knowledge / skill 共用上传状态。
- 上传失败或超时后可查询；文件不完整时上游返回错误，不会自动重新上传。

## 排障建议

- 先用 `--dry-run` 确认请求 URL、stage、query 和 body 是否符合预期。
- 如果报权限错误，同时检查当前登录身份和应用是否已经申请该 AIDev API 权限。
- 如果缺少请求体，先运行 `bk-cli aidev <action> -h --body-schema` 查看字段，再补 `--body`。
