# HnsX Monorepo Conventions

> 异构语言 monorepo 的工程化约定。HnsX 同时包含 Go、Python、TypeScript 三种运行时，
> 通过 `pnpm workspace` 管理 TS 子包，通过 `Makefile` 统一跨语言入口。

---

## 包管理

### TypeScript workspace

`pnpm-workspace.yaml` 声明三个子包：

- `hnsx-console` — React 19 控制台（private）
- `observability` — `@hnsx/observability` 组件库
- `sdk/node` — `@hnsx/sdk-node` SDK

```bash
# 安装所有 TS 依赖
pnpm install

# 构建 / 检查 / 测试所有 TS 包
pnpm build
pnpm type-check
pnpm lint
pnpm test
```

### Go module

单一 Go module：

- `hnsx-server/` — `github.com/hnsx-io/hnsx/server`

内部包含两个 `cmd` 入口：

- `cmd/hnsx` — 操作员 CLI（validate / run / version）
- `cmd/hnsx-server` — Control Plane 守护进程

共享的领域逻辑位于 `internal/app/commands`、`internal/app/queries` 以及无基础设施依赖的 `pkg/spec`、`pkg/runtime`。

```bash
make build-go      # 构建 CLI + server
make vet test-go   # 检查 + 测试
```

### Python worker

`hnsx-worker/` 是独立 Python 包，使用 `setuptools` + 本地 `.venv`。

```bash
make worker-install   # 创建 venv 并安装 editable
make worker-test      # 跑 pytest
```

---

## 版本管理

### TypeScript 包

使用 [Changesets](https://github.com/changesets/changesets)：

```bash
pnpm changeset              # 添加变更集
pnpm version-packages       # 根据变更集更新版本
pnpm release                # 发布（当前阶段多为 private，按需执行）
```

### Go / Python

Go 和 Python 版本独立维护：

- Go：`hnsx-server/go.mod`
- Python：`hnsx-worker/pyproject.toml`
- 构建时通过 `VERSION` make 变量打标：

  ```bash
  make build VERSION=0.2.1
  ```

---

## 通用入口

| 任务 | 命令 |
|---|---|
| 全量构建 | `make build` |
| 全量 CI（无 smoke） | `make ci` |
| 生成 protobuf | `make proto-all` |
| 类型检查（TS） | `pnpm type-check` |
| 格式化 Go | `make fmt` |
| 启动本地数据库 | `make db-up` |
| 端到端 smoke | `make smoke` |

---

## 目录边界

- `docs/` — 设计文档，变更需同步更新
- `proto/` — API 单一真相源，修改后必须 `make proto-all`
- `hnsx-server/` — Go Control Plane（单一 module，双 cmd：CLI + server）
- `hnsx-worker/` — Python Runtime Worker
- `hnsx-console/` / `observability/` / `sdk/node/` — TypeScript 工作区
- `example-domains/` — DomainSpec YAML 示例

---

## 提交规范

- 使用 [Conventional Commits](https://www.conventionalcommits.org/)
- 多包变更时优先通过 `pnpm changeset` 记录
- PR 末尾标注 `🤖 Generated with [Claude Code](https://claude.com/claude-code)`
- Commit 末尾可加 `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`

---

## 注意事项

- 修改 `observability` 后，在 `hnsx-console` 必须执行 `pnpm install --force` 才能看到最新源码。
- Python worker 的 generated proto 文件通过 `make proto-py` 生成，不要手动编辑。
- Go 的 generated proto 文件通过 `make proto` 生成，不要手动编辑。
- `.env` 文件不会进入 git，本地开发需要自行创建。
