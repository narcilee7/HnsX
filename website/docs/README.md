# HnsX 用户文档

> 这里的所有文档**对外公开**，用于支撑 HnsX 官网和 GitHub Pages 站点。
> 内部团队用的设计文档、商业分析、产品决策放在仓库的 `docs/` 目录，
> 不在公开站点上挂链接。

---

## 目录结构

```
documents/
├── README.md                       # 你在这里
├── guide/                          # 用户上手指南（来自 website/docs/guide）
│   ├── README.md                   # 指南索引
│   ├── install.md                  # 安装
│   ├── quick-start.md              # 快速开始
│   ├── cli-basics.md               # CLI 常用命令
│   └── domain-spec.md              # Domain 入门
├── blog/                           # 公开博客（来自 website/docs/blog）
│   ├── README.md                   # 博客索引
│   ├── why-harness.md              # 为什么需要 Harness
│   ├── api-at-a-glance.md          # API 一览
│   └── vision-in-practice.md       # 愿景落地
├── know-how/                       # 技术深度（公开版）
│   ├── README.md                   # know-how 索引
│   ├── 建模Harness.md              # 如何建模 Harness
│   ├── 编排Agent.md                # 如何编排 Agent 并集成 Harness
│   ├── 观测Harness.md              # 如何观测 Harness 与 Agent
│   └── 评测Harness.md              # 如何评测 Harness 与 Agent
└── use-cases/                      # 行业用例（公开模板）
    └── README.md
```

---

## 文档分层约定

| 目录 | 对外可见 | 来源 | 谁能改 |
|---|---|---|---|
| `documents/` | ✅ 是 | GitHub Pages 站点 | 任何人（PR 走 review） |
| `docs/` | ❌ 否（内部） | 设计 / 商业 / 决策 | 仅核心团队 |
| 顶层 `README.md`、`LICENSE`、`CONTRIBUTING.md` 等 | ✅ 是 | GitHub 仓库首页 | 任何人（PR 走 review） |

---

## 写文档时的几点规则

1. **不要在 `documents/` 出现**：
   - ARR / 收入 / 商业目标
   - 团队规模 / 融资计划
   - 竞争对手详细对比表（含定价、份额）
   - 内部代号 / 内部未发布功能
   - 客户名单 / 商业案例（除非已签 NDA 公开）
2. **可以出现在 `documents/`**：
   - 公开技术架构 / API / CLI / DomainSpec
   - 安装 / 部署 / 故障排查
   - know-how（技术深度）
   - 公开博客（产品哲学 + 用法）
3. **新增 public 文档的流程**：
   - 在 `documents/<分类>/` 下新增 `.md` 文件
   - 在对应 `documents/<分类>/README.md` 加链接
   - 更新 `website/rspress.config.ts` 的 `nav`（如需导航暴露）
   - 提 PR，等 review

---

## 与 website/ 的关系

`website/` 是 Rspress 文档站源码（构建产物部署到 GitHub Pages）。
理论上 `documents/` 的内容会被同步到 `website/docs/`（手工或 CI 同步），
然后由 Rspress 渲染成静态站点。CI 同步脚本待实现（见
[docs/PROVENANCE.md § 未来工作](../docs/PROVENANCE.md)）。

**当前是手工 cp 同步**：每改一个文件，记得 `cp documents/guide/install.md website/docs/guide/install.md`。
Phase 2 之前我们会改成 GitHub Action 自动同步。

---

## 与 docs/ 的关系

- `documents/` = 公开（用户能看）
- `docs/` = 私有（用户看不到，但仓库里能看到代码）
- README.md 的链接**只能**指向 `documents/` 或 `website/docs/`，不能指向 `docs/ANALYSIS.md` / `docs/DECISIONS.md` / `docs/ROADMAP.md`

详见 [docs/README.md § 与 documents/ 的关系](../docs/README.md)。