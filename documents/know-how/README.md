# Know-how — 技术深度文档

> 这是 HnsX 的**技术深度系列**。讲清"我们为什么这样设计、这样实现"。
> 读者假设：已经会用 HnsX、想了解底层原理的工程师 / 架构师。

如果你是第一次接触 HnsX，请先读 [guide/quick-start.md](../guide/quick-start.md)。

---

## 目录

| 文档 | 内容 | 一句话 |
|---|---|---|
| [建模 Harness](./建模Harness.md) | DomainSpec 的实体、关系、生命周期、配置分层 | "Harness 长什么样，为什么这样切" |
| [编排 Agent](./编排Agent.md) | Step ≠ Agent call、4 种编排模式、Agent 集成 | "Agent 在 HnsX 里怎么被驾驭" |
| [观测 Harness](./观测Harness.md) | 18 类 Observation、Trace、OTel 映射、审计 | "怎么把 Agent 跑出来的东西变成可审计的" |
| [评测 Harness](./评测Harness.md) | EvalSet / EvalRun / Scorer / Baseline | "怎么量化 Harness 与 Agent 的好坏" |

## 阅读顺序建议

1. 先读 [建模](./建模Harness.md) — 建立概念基础
2. 再读 [编排](./编排Agent.md) — 看运行时怎么动起来
3. 然后 [观测](./观测Harness.md) — 看怎么审计 / 排错
4. 最后 [评测](./评测Harness.md) — 看怎么持续进化

## 与 docs/ 的关系

这些文档的**完整版（含内部细节、决策记录、未发布功能）**在
仓库 [docs/know-how/](../../docs/know-how/) 目录。本目录是公开版，
经过裁剪：
- ❌ 删除 ARR / 商业目标 / 内部代号
- ❌ 删除未发布功能
- ✅ 保留技术原理 + 公开 API + 示例
- ✅ 保留"为什么这样设计"的论证

详见 [docs/README.md § 与 documents/ 的关系](../../docs/README.md)。