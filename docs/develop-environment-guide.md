### 前置要求

- Go 1.25.5+
- [Ginkgo](https://onsi.github.io/ginkgo/)（BDD 测试框架）

### 开发流程

```bash
# 克隆并构建
git clone https://github.com/TencentBlueKing/bk-cli.git
cd bk-cli
go build -o bk-cli .

# 运行测试
go test ./... -count=1

# 使用详细输出运行测试
go test ./internal/config/ -v -count=1

# 运行容器化集成测试
make test-integration

# 仅运行一个集成场景
make test-integration SCENARIO=CTX-001

# 按 case 文件路径运行
make test-integration CASE=tests/integration/cases/context/CTX-001-context-create-use-list.yaml

# 构建优化后的二进制文件
go build -ldflags="-s -w" -o bk-cli .

# 跨平台发布构建（需要 goreleaser）
make release-build VERSION=0.1.0

# 发布 npm 包
make npm-publish

# 生成 CHANGELOG.md（基于 conventional commits）
make changelog VERSION=0.1.0

# 完整发布流程（创建 tag + 生成 changelog + 设置 npm 版本 + 构建 + 发布）
make release VERSION=0.1.0
```

### 测试约定

- **框架**: Ginkgo v2 + Gomega（`Describe` / `Context` / `It`）
- **位置**: 测试与源码同目录放置（`*_test.go`）
- **隔离**: 每个测试通过 `BK_CLI_CONFIG_DIR` 使用临时目录
- **Mock**: 在 HTTP 边界进行 mock，而不是内部接口
- **命名**: `test_{function}_{scenario}_{expected}`
- **集成测试**: `tests/integration/` 使用 Docker Compose + 仓库内置 YAML runner 运行真实 `bin/bk-cli`
- **场景标识**: 每个 `.yaml` 文件一个场景，声明唯一 `id`、步骤命令与期望结果

### 代码约定

- 每个 package 单一职责
- 函数尽可能控制在 30 行以内
- 不保留死代码或被注释掉的代码块
- 每个新增依赖都必须有合理理由
- `internal/*` 绝不能导入 `cmd/*`
- 默认在 stdout 输出 JSON 信封，在 stderr 输出错误
- 所有会构造远端请求或执行更新的命令都必须支持 `--dry-run`，并在 `--help` 中提供示例