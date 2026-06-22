# bk-cli 扩展开发指南

> 受众：bk-cli 开发者，需要扩展 bk-cli 的能力
> 你负责说清目标和边界，agent 负责按仓库约定实现

## 说明

[bk-cli](https://github.com/TencentBlueKing/bk-cli) 项目 **100% 是由 agent 开发的**。所以拓展需求，建议使用 Agent (Codex or Claude Code or Cursor) 完成对应的需求开发。

项目本身提供了足够强的约束/指引以及`make`命令：

1. AGENT 特有的指引文档 [AGENTS.md](https://github.com/TencentBlueKing/bk-cli/blob/master/AGENTS.md) and [CLAUDE.md](https://github.com/TencentBlueKing/bk-cli/blob/master/CLAUDE.md)
2. 提供了项目整体的设计文档 [design.md](https://github.com/TencentBlueKing/bk-cli/blob/master/docs/design.md)
3. 提供了 SKILL [create-bk-cli-system](https://github.com/TencentBlueKing/bk-cli/tree/master/.agents/skills/create-bk-cli-system) 请安装并使用它进行需求开发
4. 提供了全套 `make` 命令，涵盖 lint/test/test-integration 做规范/单元测试/集成测试

## 建议

- 请使用最好的模型， `GPT-5.4` or `Opus`
- 变更不大，不建议用 `SDD`，建议使用 `plan mode`(建议）或 `superpowers`
- 一个 PR 只包含小范围的新增或变更，方便 Review 和 测试

## 前置准备

- 如果是新增一个系统，需要准备：
	- 系统名：`bk-cli <系统名>`
	- 系统描述：用于 `bk-cli <系统名> -h` 时展示，方便 agent 根据 help text 感知能通过这个命令做什么事情
	- 系统对应网关名：各个操作将会调用的网关 **最终会使用这个网关名调用对应接口**
- 如果是为已有的某个系统，新增一个操作。如果是调用接口，需要提前准备对应网关接口的 openapi 声明【建议从自动化导入的 yaml 中获取，或者从网关站点导出 yaml 获取】；包含 资源名/资源描述/资源路径，最重要的是需要包含 `authConfig` (可以参考以下示例，删掉不需要的字段，例如 `responses`声明等，节省 token)
- 如果一个已有系统很大，并且 API 列表天然按模块划分，例如 `devops` 下区分 `pipeline`、`codecc`、`stream`，需要先决定命令形态：
  - 扁平 action：`bk-cli devops get_build_list`
  - 一层 subsystem：`bk-cli devops pipeline get_build_list`
- 如果不同模块背后对应不同 API Gateway，优先考虑一层 subsystem。每个 subsystem 可以独立声明自己的 `gateway_name`。
- 当前只支持一层 subsystem，不支持 `bk-cli <system> <subsystem> <sub_subsystem> <action>`。
- 如果是 BCS 这类 OpenAPI request body 很复杂的系统，默认不要把 body 字段拆成大量命令 flags；使用共享 `--body '<json>'`，把请求体示例放进 `examples`，并在 YAML action 里补 `body_schema`。如果上游 request body 是必填的，同时补 `body_required: true`，让 CLI 在发起请求前校验 `--body`。默认 help 按 `Usage`、`Examples`、schema 查看提示的顺序展示，完整 schema 通过 `bk-cli <system> [subsystem] <action> -h --body-schema` 查看。

```yaml
  /api/v2/open/gateways/:
    get:
      operationId: v2_open_list_gateways
      description: 获取网关列表
      tags:
      - v2_open
      parameters:
        - name: name
          in: query
          schema:
            type: string
          description: 网关名称，用于过滤网关
          example: bk-apigateway
        - name: fuzzy
          in: query
          schema:
            type: boolean
          description: 是否模糊匹配，true：模糊匹配（name 包含），false：精确匹配
          example: true
        - name: keyword
          in: query
          schema:
            type: string
          description: 搜索关键字，模糊匹配 name 或 description
          example: apigateway
      x-bk-apigateway-resource:
        authConfig:
          userVerifiedRequired: false
          appVerifiedRequired: true
          resourcePermissionRequired: false
        descriptionEn: None
```

- 如果新增操作的逻辑中，包含其他的调用逻辑，需要提前准备好逻辑的细节

## 步骤

```bash
# 1. 准备好 go 1.25.5 的开发环境

# 2. 克隆代码
$ git clone https://github.com/TencentBlueKing/bk-cli.git

# 3. 进入目录， 启动 agent
$ cd bk-cli
$ codex or claude or cursor

# 4. 准备好prompt， 丢给 agent 执行

```

## Prompts

请根据需求，复制并完善 prompt，建议**对命令的说明/参数等描述越详细越好，避免需求不清导致的实现效果不佳。**

### 1. 为某个系统增加新的操作

注意，替换 `<system>` 和 `<action>`

```text
请阅读 @AGENTS.md @docs/design.md，使用 SKILL @.agents/skills/create-bk-cli-system/SKILL.md 完成以下需求：

为 bk-cli 的 <system> 增加 <action>。

目标：
- 命令形态：bk-cli <system> <action> --<arg1> xxx --<arg2> xxx
  - action 说明
  - arg1 string 必填
  - arg2 int 可选 默认值 xxx
- 将会转换并调用接口
  <前置准备好的网关接口 openapi 声明 yaml>

特殊说明：
- <如果有的话>

要求：
- 你可以阅读现有已实现系统的代码，确认现有实现模式，按仓库约定做最小扩展
- 不要修改公共契约
- 如果发现必须修改公共契约，停止实现并给出 proposal issue 建议
- 补必要的测试、帮助信息和文档
- 检查所有 `Change Checklist` in AGENTS.md
- 最后汇报验证命令和结果
```

### 2. 新增一个系统

注意，替换 `<system>` 和 `<action>` , 建议**首次只新增一个 action**

```text
请阅读 @AGENTS.md @docs/design.md，使用 SKILL @.agents/skills/create-bk-cli-system/SKILL.md 完成以下需求：

为 bk-cli
1. 新增系统 <system>
	- 系统名： <system name>
	- 系统描述：<system description>
	- 系统对应网关名： <system apigateway name>
2. 为系统新增 <action>

目标：
- 命令形态：bk-cli <system> <action> --<arg1> xxx --<arg2> xxx
  - action 说明
  - arg1 string 必填
  - arg2 int 可选 默认值 xxx
- 将会转换并调用接口
  <前置准备好的网关接口 openapi 声明 yaml>

特殊说明：
- <如果有的话>

要求：
- 你可以阅读现有已实现系统的代码，确认现有实现模式，按仓库约定做最小扩展
- 不要修改公共契约
- 如果发现必须修改公共契约，停止实现并给出 proposal issue 建议
- 补必要的测试、帮助信息和文档
- 检查所有 `Change Checklist` in AGENTS.md
- 最后汇报验证命令和结果
```

### 3. 为已有系统新增 subsystem

注意，替换 `<system>`、`<subsystem>` 和 `<action>`。如果 API 列表本身已经分成多个模块，建议先让 agent 对比“扁平 action”和“一层 subsystem”两种命令形态。

```text
请阅读 @AGENTS.md @docs/design.md，使用 SKILL @.agents/skills/create-bk-cli-system/SKILL.md 完成以下需求：

为 bk-cli 的 <system> 新增一层 subsystem：<subsystem>。

目标：
- 命令形态：bk-cli <system> <subsystem> <action> --<arg1> xxx --<arg2> xxx
  - subsystem 说明：<subsystem description>
  - action 说明：<action description>
  - arg1 string 必填
  - arg2 int 可选 默认值 xxx
- subsystem 对应网关名：<subsystem apigateway name>
- 将会转换并调用接口
  <前置准备好的网关接口 openapi 声明 yaml>

特殊说明：
- <如果有的话>

要求：
- 不要修改现有 <system> 下已发布命令的路径和行为
- 如果 action 名称会和父 system 下已有 action 或 subsystem 冲突，停止并说明冲突
- 不要创建多层 subsystem
- 不要修改公共契约
- 如果发现必须修改公共契约，停止实现并给出 proposal issue 建议
- 补必要的测试、帮助信息和文档
- 检查所有 `Change Checklist` in AGENTS.md
- 最后汇报验证命令和结果
```

## 开发完成后的检查

- AGENT 有按要求实现对应代码/单元测试文件 (`_test.go`)/集成测试配置
- `make lint` / `make test` / `make test-integration`  等全部成功
- `make build` 后本地做测试（注意，变更类操作请在对应测试环境验证，避免生产环境误操作）
- 确定有没有修改 skills 中对应系统的 SKILL.md, 需要补充或更新变更
- 换一个模型，进行 code review (例如：如果用 gpt-5.4 开发的，换 opus review)
- 进行人工 Review
- 提交 PR

## 其他

### 涉及公共契约的变更

“公共契约”包括但不限于：

- 输出结构、错误码、退出码
- context / credential 解析规则
- `--dry-run`、`--verbose`、`--insecure`、`--header`、`--body`、timeout、tenant 等共享语义
- YAML schema 和共享 request contract
- subsystem 层级规则、命令路径兼容性、system/subsystem/action 命名冲突规则

如果扩展过程中发现“必须修改公共契约”才能继续，请**停止当前扩展**，改走例外流程：

1. 新建 `proposal issue`
2. 与仓库维护者讨论并确认方向
3. 公共契约改动使用**单独的 PR**
4. **禁止**把公共契约改动和 `system` / `action` 扩展放在同一个 PR 里

## 或者，你可以直接提一个需求单到 bk-cli 仓库

地址： [TencentBlueKing/bk-cli issues](https://github.com/TencentBlueKing/bk-cli/issues)

描述清楚需求/需要添加的系统/命令，支持的参数等

我们将使用 AGENT 自动发现需求单，完成开发测试后提交 PR

PS: 合并后，可能需要用户协助进行命令的灰度测试

## 相关链接

- [bk-cli 仓库](https://github.com/TencentBlueKing/bk-cli)
- [bk-cli README.md](https://github.com/TencentBlueKing/bk-cli/blob/master/README.md)
- [bk-cli 用户指南](https://github.com/TencentBlueKing/bk-cli/blob/master/docs/user-guide.md)
- [bk-cli FAQ](https://github.com/TencentBlueKing/bk-cli/blob/master/docs/faq.md)
- [bk-cli 设计文档](https://github.com/TencentBlueKing/bk-cli/blob/master/docs/design.md)
