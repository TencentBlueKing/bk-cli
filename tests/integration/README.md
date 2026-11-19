# bk-cli 集成测试

这个目录包含 `bk-cli` 的仓库内集成测试 harness。

它通过仓库内提交的 YAML 用例文件校验真实构建出的 CLI，只与本地配套服务通信。默认使用 Docker Compose 提供隔离服务；无可用 Docker 时，可以按下面的规则在本地执行相同用例。

## 覆盖策略

- 每个公开的 `system` action 都至少应有一个已提交的 happy-path 用例
- 有风险的变更类 action 还应补充有针对性的负向覆盖
- 负向用例应保持有选择性且有明确目的，默认不追求穷尽

## 目录内容

- `cases/`：可运行场景，每个场景一个 `.yaml` 文件
- `fixtures/`：共享的种子配置和环境默认值
- `mock_api/`：用于返回 system 风格响应的单文件 Flask mock
- `runner/`：YAML 用例运行器以及容器入口
- `compose.yaml`：隔离的测试栈，包含 `httpbin`、`mock_api` 和 `test`
- `artifacts/latest/`：生成的报告和运行时日志

### 为什么要区分 `cases/` 和 `fixtures/`

- `cases/` 是我们希望验证的行为的事实来源。
- `fixtures/` 存放可复用输入，让用例保持简短且易读。

如果一个文件是真实场景，应该被人直接运行，它就属于 `cases/`。
如果它是被多个场景复用的共享支撑输入，它就属于 `fixtures/`。

## 当前目录布局规则

- `context`、`auth` 和原始 `api` 场景直接放在各自对应的目录下
- system 场景放在 `cases/system/<system>/` 下

示例：

- `cases/context/CTX-001-context-create-use-list.yaml`
- `cases/system/devops/SYSGO-001-devops-start-build.yaml`
- `cases/system/apigateway/SYSYAML-001-apigateway-list-gateways.yaml`

## 前置要求

- 与仓库基线一致的 Go 工具链
- GNU Make
- Compose 执行需要 Docker、Compose v2 和可访问的 Docker daemon
- 本地执行需要用例依赖的本地服务；下面的 PaaS 示例需要 curl、Python 和 Flask，Flask 版本参考 `mock_api/Dockerfile`

## 基本命令

构建被测 CLI：

```bash
go build -o ./bin/bk-cli .
```

运行完整集成测试套件：

```bash
make test-integration
```

按场景 ID 运行单个场景：

```bash
make test-integration SCENARIO=CTX-001
```

按用例路径运行单个场景：

```bash
make test-integration CASE=tests/integration/cases/system/devops/SYSGO-001-devops-start-build.yaml
```

清理一次被中断的运行：

```bash
make test-integration-down
```

## 无可用 Docker 时的本地验证

仓库的环境选择与交付规则以 [AGENTS.md 的 Verification Workflow](AGENTS.md#verification-workflow) 为准。
对于 CLI、YAML 用例、fixture、mock 响应或 runner 逻辑变更，无可用 Docker 时，本地真实 CLI 与现有 runner 执行相同用例并通过，即满足本地集成检查；无需逐任务申请 Docker 豁免。
其他适用检查仍需通过。Compose 配置、Dockerfile、容器入口、网络、挂载或服务启动的变更仍需在可用环境完成 Compose 验证。

下面以只依赖现有 `mock_api` 的 `PAAS-001` 为例。所有命令均从仓库根目录执行。
先在一个终端启动现有 mock 服务，结束验证后停止该进程：

```bash
PYTHONDONTWRITEBYTECODE=1 python3 -m flask \
  --app tests/integration/mock_api/app.py run --host 127.0.0.1 --port 8080
```

在另一个终端执行：

```bash
mkdir -p ./bin/integration
make build BINARY=./bin/integration/bk-cli BK_TE_DOMAIN=te.example
go build -o ./bin/integration/bk-cli-int ./tests/integration/cmd/inttest

set -a
. tests/integration/fixtures/env/common.env
set +a
export MOCK_API_BASE_URL=http://127.0.0.1:8080
export BK_CLI_BIN="$PWD/bin/integration/bk-cli"
export RUNNER_BIN="$PWD/bin/integration/bk-cli-int"
export CASES_DIR="$PWD/tests/integration/cases"
export FIXTURES_DIR="$PWD/tests/integration/fixtures"

curl --fail --silent --show-error "$MOCK_API_BASE_URL/healthz"

REPORT_DIR="$PWD/tests/integration/artifacts/native/PAAS-001" \
  SCENARIO=PAAS-001 CASE= sh tests/integration/runner/entrypoint.sh
REPORT_DIR="$PWD/tests/integration/artifacts/native/PAAS-001-path" \
  SCENARIO= CASE=tests/integration/cases/system/paas/PAAS-001-paas-get-deployment-result.yaml \
  sh tests/integration/runner/entrypoint.sh
```

`te.example` 对应 Makefile 中 `INTEGRATION_BK_TE_DOMAIN` 的默认值；若任务使用其他值，本地构建也要使用相同值。
按上述方式运行所有受影响的场景，并至少按精确路径运行一个变更的 system 用例。
保留原 YAML 步骤和断言，只调整本地路径、服务地址等环境值。
依赖 `httpbin` 的用例还需启动对应的本地服务，并将 `HTTPBIN_BASE_URL` 设置为其地址；无法提供所需服务时，要记录尚未完成的检查。

保存执行命令、退出码、`results.json`、`results.xml` 和步骤日志，并说明本地执行尚未验证容器编排。
无可用 Docker 时，Compose 不作为这些变更的必需本地检查；未执行不能记为通过，已失败的尝试也应如实保留。
现有集成 CI 继续运行 Compose 全套用例，其结果与本地结果分别报告。

## 报告

harness 会将报告写入 `tests/integration/artifacts/latest/`：

- `results.json`
- `results.xml`
- `compose.log`

每个场景的 stdout 和 stderr 日志也会写入
`tests/integration/artifacts/latest/runtime/`。

## 如何新增一个用例

1. 选择 `cases/` 下正确的目录。
2. 为一个场景创建一个 `.yaml` 文件。
3. 添加 `id`、`name` 和有序的 `steps`。
4. 保持最小化 setup：
   通常是先 `context init`，然后 `auth login`，最后执行目标命令。
5. 只断言能够证明该行为所需的字段。
6. 单独运行新用例进行验证。

### 最小结构

```yaml
id: EXAMPLE-001
name: example scenario
steps:
  - name: run command
    command:
      - version
    expect:
      exit_code: 0
      checks:
        - path: ok
          equals: true
```

## 什么时候使用 `httpbin`

当你只需要通用 HTTP 行为时，使用 `${HTTPBIN_BASE_URL}`：

- 回显 headers、query strings 或 request bodies
- 校验请求组装结果
- 检查 dry-run 或原始 API 行为

## 什么时候使用 `mock_api`

当你需要 system 特定行为时，使用 `${MOCK_API_BASE_URL}`：

- BlueKing 风格路由
- system 风格的 JSON payload
- 可确定复现的上游失败
- 按场景键控的响应

把自定义上游行为保存在
[mock_api/app.py](./mock_api/app.py)。

## 为新增覆盖推荐的工作流

对于一个新 system：

1. 先添加 CLI 命令本身。
2. 在 `cases/system/<system>/` 下为每个公开 action 添加一个 happy-path 集成用例。
3. 只有在 `httpbin` 不够用时才添加 mock 路由。
4. 分别通过场景 ID 和用例路径进行验证。

对于已有 system 中的新 action：

1. 在该 system 对应目录下添加一个新的场景文件。
2. 复用 `fixtures/` 中现有的环境默认值。
3. 只有当 action 需要自定义行为时才扩展 `mock_api/app.py`。
4. 仅当 action 会产生变更或运维风险较高时，再添加负向用例。

## 验证

修改 runner 或新增、修改用例后，先执行：

```bash
go test ./tests/integration/runner -count=1
```

有可用 Docker 时执行：

```bash
make test-integration SCENARIO=<SCENARIO_ID>
make test-integration CASE=tests/integration/cases/system/<system>/<file>.yaml
```

无可用 Docker 时，按上面的本地验证示例执行所有受影响场景，并至少按精确路径执行一个变更的 system 用例。

## 需要避免的事情

- 使用真实 BlueKing 端点或真实凭证
- 在无关场景之间共享可变状态
- 把可复用输入直接放进 `cases/`
- 把生成的产物提交进 git
- 对不稳定或偶然性的响应字段做过度断言

面向 agent 的覆盖新增说明位于
[AGENTS.md](./AGENTS.md)。
