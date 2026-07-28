---
name: bk-cli-paas
description: 当需要通过 `bk-cli paas` 查询蓝鲸 PaaS 应用模块部署信息、部署任务结果、创建云原生应用，或触发支持多模块的部署动作时使用。
---

# bk-cli paas — 蓝鲸 PaaS 应用部署能力

用于通过 `bk-cli paas` 调用蓝鲸 PaaS 应用创建和部署相关 API。

CRITICAL — 开始前 MUST 先用 Read 工具读取 `../bk-cli-shared/SKILL.md`。共享 skill 负责认证、context、tenant、stage、dry-run、verbose、header/body 和通用请求规则；本 skill 只补充 `paas` 命令自己的语义与输入约定。

## 当前覆盖范围

当前 skill 覆盖四个命令：

- `get_deployment_result`
- `module_env_released_info`
- `deploy_with_module`
- `create_cloud_native_app`

## 输入约定

- 这些接口都要求应用认证 + 用户认证 + 接口资源权限。调用前需要确保当前 context 的凭据同时满足应用身份和用户身份，并且对应应用已申请这些 API 的接口权限。
- 应用 ID 使用 PaaS 应用 `app_code` 或接口原始参数名 `code`。
- 模块名使用 PaaS 模块名称，默认模块通常为 `default`。
- 环境参数使用接口原始参数名：`env` 或 `environment`，常见值为 `stag` 或 `prod`。
- `deploy_with_module` 是 YAML action，部署版本信息通过共享 `--body '<json>'` 传入；`version_name` 和 `version_type` 必填，`revision` 可选。执行时必须显式提供非空 `--body`。
- `create_cloud_native_app` 是 YAML action，创建参数通过共享 `--body '<json>'` 传入；执行时必须显式提供非空 `--body`。完整请求体结构可用 `bk-cli paas create_cloud_native_app -h --body-schema` 查看。

## Commands

#### `get_deployment_result`

```bash
bk-cli paas get_deployment_result \
  --app_code bk-demo \
  --module default \
  --deployment_id 12345
```

- 查询部署任务结果。
- 调用路径：`GET /bkapps/applications/{app_code}/modules/{module}/deployments/{deployment_id}/result/`

#### `module_env_released_info`

```bash
bk-cli paas module_env_released_info \
  --code bk-demo \
  --module_name default \
  --environment prod
```

- 查询应用模块环境部署信息。
- 调用路径：`GET /bkapps/applications/{code}/modules/{module_name}/envs/{environment}/released_info/`

#### `deploy_with_module`

```bash
bk-cli paas deploy_with_module \
  --app_code bk-demo \
  --module default \
  --env prod \
  --body '{"revision":"{commit_id}","version_type":"branch","version_name":"master"}'
```

- 触发支持多模块的 App 部署。
- 调用路径：`POST /bkapps/applications/{app_code}/modules/{module}/envs/{env}/deployments/`
- 请求体字段：`revision` 为源码仓库版本号，可选；`version_name` 为 branch 或 tag 名称，必填；`version_type` 为版本类型，必填，svn 支持 `trunk`/`tag`，git 支持 `branch`。

#### `create_cloud_native_app`

```bash
bk-cli paas create_cloud_native_app \
  --body '{"code":"bk-demo","name":"bk-demo","source_config":{"source_origin":1,"source_repo_url":"https://github.com/octocat/helloWorld.git","source_repo_auth_info":{},"source_dir":"","source_init_template":"docker"},"bkapp_spec":{"build_config":{"build_method":"dockerfile","dockerfile_path":"Dockerfile"}}}'
```

- 创建云原生应用。
- 调用路径：`POST /bkapps/cloud-native/`
- 请求体根字段：`code`、`name`、`source_config`、`bkapp_spec` 必填；`app_tenant_mode`、`auth_code`、`is_plugin_app`、`advanced_options` 按需传入。
- `source_config.source_origin` 表示源码来源，常见值：`1` 为已授权代码仓库，`6` 为云原生镜像。
- `bkapp_spec.build_config.build_method` 支持 `buildpack`、`dockerfile`、`custom_image`；`dockerfile` 场景通常需要 `dockerfile_path`，`custom_image` 场景需要 `image_repository` 且通常需要 `processes`。
