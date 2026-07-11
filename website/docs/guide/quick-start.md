# 快速开始

> 目标：5 分钟内让 HnsX 跑起来，完成一次端到端 Session。

## 前置条件

- macOS / Linux / WSL
- Docker & Docker Compose（用于本地 Postgres + Server + Worker）
- Node 20+ 和 pnpm（用于控制台）
- Go 1.22+（用于源码构建 server / CLI）

## 1. 安装 CLI

```bash
curl -sSL hnsx.dev/install.sh | sh
```

macOS 用户也可以：

```bash
brew install narcilee7/hnsx/hnsx
```

## 2. 启动本地全栈

```bash
cd deployments/local
docker compose up -d
```

这会拉起 Postgres、HnsX Server、Worker，以及可选的 Tempo + Grafana。

## 3. 触发第一个 Session

```bash
hnsx try customer-service
```

如果一切正常，你会看到一次完整的客服分诊 Session：Trigger → Session → Turn → Observation。

## 4. 查看 Trace

```bash
hnsx trace list
hnsx trace show <trace-id>
```

或者在浏览器打开 `http://127.0.0.1:50052` 进入控制台。

## 下一步

- 学习 [Domain 入门](/guide/domain-spec)
- 阅读 [为什么需要 Harness](/blog/why-harness)
