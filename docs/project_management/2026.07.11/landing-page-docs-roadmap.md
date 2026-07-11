# HnsX 官网 + 文档站 Roadmap（Rspress）

> 日期：2026-07-11
> 目标：把 HnsX 从「实验项目 GitHub 仓库」升级为「有 Landing Page + 产品文档」的可产品化站点
> 北极星：[`docs/vision.md`](../../vision.md) — Harness as a Service
> 当前分支：`feat/tui_migration`
> 技术选型：**Rspress**（基于 Rspack 的静态站点生成器，原生支持 Markdown 文档 + Landing Page）

---

## 0. 范围与边界

### 做这个
- 新增一个独立的 `website/` 包，用 Rspress 承载：
  - **Landing Page**：产品定位、核心能力、Quick Start、CTA
  - **文档站**：把现有 `docs/` 里的设计文档、know-how、API 设计等以可导航方式呈现
  - **博客/更新日志**（可选，先留坑）
- 接入 monorepo workspace，支持 `pnpm install` 统一依赖管理
- 本地开发：`pnpm dev` 预览；构建：`pnpm build` 输出静态站点到 `website/dist`
- 部署目标：**GitHub Pages**（先免费跑通），后续可切到 `hnsx.dev`

### 不做这个
- 不动现有 `docs/` 目录结构和已有 Markdown 内容（只加 `_meta.json` 做导航）
- 不替换 `hnsx-console`（控制台保持独立，官网只做展示+文档）
- 不一次性写光所有文档内容，先把框架和关键页面跑通

---

## 1. 本轮基线

| 现状 | 问题 |
|---|---|
| 仓库有 `docs/` 但只是 GitHub 直接渲染 | 没有品牌首页、没有导航、没有搜索 |
| `hnsx-console` 是运营/审计控制台 | 不是给新用户看的产品官网 |
| 没有独立部署的静态站点 | 无法挂 `hnsx.dev`、无法做 SEO/分享卡片 |
| monorepo 用 pnpm workspace | 适合把 website 作为新 workspace package 加入 |

---

## 2. 方案（推荐）

### 目录结构

```text
HnsX/
├── docs/                          # 保留现有设计文档（新增 _meta.json 做导航）
├── website/                       # 新增 Rspress 站点
│   ├── docs/                      # Rspress 文档根
│   │   ├── index.md               # Landing Page（Rspress home 页）
│   │   ├── _meta.json             # 顶层导航
│   │   ├── blog/                  # 产品博客
│   │   │   ├── _meta.json
│   │   │   ├── why-harness.md
│   │   │   ├── api-at-a-glance.md
│   │   │   └── vision-in-practice.md
│   │   └── design/                # 软链/映射到 docs/ 下设计文档
│   │       └── ...
│   ├── rspress.config.ts          # Rspress 配置
│   ├── package.json
│   ├── styles/                    # 自定义全局样式
│   │   └── index.css
│   └── .gitignore
├── pnpm-workspace.yaml            # 增加 website
└── .github/workflows/website.yml  # 自动部署到 GitHub Pages（待 Phase 4）
```

### 关键技术决策

1. **website 作为独立 package**：
   - 自己的 `package.json`、`rspress` 依赖，避免和 `hnsx-console` 的 React 19 冲突
   - 加入 root `pnpm-workspace.yaml`
   - `private: true`

2. **复用现有 `docs/`**：
   - 在 `website/docs/design/` 下创建 **symlink** 指向 `../../docs/`
   - Rspress 会自动把 symlink 目标中的 Markdown 纳入构建
   - 好处：不移动文件、不破坏现有 GitHub 直接浏览、一处修改两处生效

3. **Landing Page**：
   - 使用 Rspress 原生 frontmatter 写法：`pageType: home`
   - Hero 区：一句话定位 + 两个 CTA 按钮（Quick Start / GitHub）
   - Features 区：Domain / Session / Policy / Observability / Eval / Multi-Agent
   - Quick Start 区：3 步命令
   - Footer：GitHub / Docs / Console 链接

4. **样式与品牌**：
   - 颜色沿用项目 CSS 变量：`--chart-1..5`、`--success`、`--warning`、`--danger`、`--info`
   - 字体使用 `Geist`（和 console 一致）
   - 初版没有 logo SVG，先用文字 logo + 简洁几何图形占位

5. **部署**：
   - GitHub Actions workflow：监听 `website/**` 和 `docs/**` 变更，推送到 `gh-pages` 分支
   - 仓库 Settings → Pages → Source 设为 `gh-pages` 分支
   - 站点路径：默认 `https://narcilee7.github.io/HnsX/`
   - `base` 按环境切换：本地开发用 `/`（避免 `localhost:3000/` 404），GitHub Actions 里设 `GH_PAGES=true` 切到 `/HnsX/`
   - 后续买/配 `hnsx.dev` 时只需改 DNS + workflow 里去掉 `GH_PAGES`，让 `base` 保持 `/`

---

## 3. Phase 划分

### Phase 1 — 骨架跑通（1 天）

目标：`cd website && pnpm dev` 能看到 Hello World 首页。

- [x] 创建 `website/package.json`，安装 `rspress` 及类型依赖
- [x] 更新 root `pnpm-workspace.yaml` 加入 `website`
- [x] 创建 `website/rspress.config.ts`（基础配置：title、description、themeConfig）
- [x] 创建 `website/docs/index.md`（Landing Page 占位）和 `website/docs/_meta.json`
- [x] 创建 `website/styles/index.css` 引入项目基础色变量
- [x] 跑通 `pnpm install` / `pnpm dev` / `pnpm build`

#### 验收
```bash
cd website
pnpm install
pnpm dev      # 本地 3000 端口可访问
pnpm build    # 生成 website/dist，无报错
```

---

### Phase 2 — Landing Page 内容 + 品牌样式（1–2 天）

目标：首页看起来像个产品，而不是脚手架。

- [x] 用 Rspress frontmatter 配置 Hero：标题、描述、两个按钮、背景
- [x] 添加 Features 区（6 张卡片，对应核心能力）
- [x] 添加 Quick Start 区（复制即可运行的 3 条命令）
- [x] 添加 Footer 与导航
- [x] 自定义 CSS：字体、配色、按钮、卡片阴影、响应式
- [ ] 添加 favicon（若无 logo，先用 emoji/纯文字占位）

#### 验收
- 桌面端首页无错位
- 移动端（< 768px）Hero 文字换行合理、按钮堆叠
- Lighthouse 首屏性能 > 70（静态站默认即可达到）

---

### Phase 3 — 文档导航与搜索（1–2 天）

目标：让现有 `docs/` 的内容能被网站导航和搜索到。

- [x] 在 `website/docs/` 下创建 `design/` 目录，用 **symlink** 指向 `../../docs/`
- [x] 为 `website/docs/design/` 及子目录写 `_meta.json`，按主题分组（vision / tech / know-how / server-design / console-design / SOP）
- [x] 在 `website/docs/blog/` 下新建产品博客：
  - `why-harness.md`（Why harness）
  - `api-at-a-glance.md`（API 一览）
  - `vision-in-practice.md`（愿景落地）
- [x] 在 `website/docs/guide/` 下新建入门指南：
  - `quick-start.md`（5 分钟跑通）
  - `install.md`（curl / brew / 源码）
  - `domain-spec.md`（Domain 入门）
  - `cli-basics.md`（常用 CLI）
- [x] 配置 Rspress `themeConfig.nav` 与 `sidebar`，让用户能在 landing、guide、blog、docs 间跳转
- [x] 启用 Rspress 内置搜索（基于 FlexSearch）

#### 验收
- 左侧 sidebar 能展开所有设计文档分组
- 顶部 nav 可在 Home / Guide / Blog / Design / API / GitHub 间切换
- 搜索框能搜到 `docs/vision.md` 的关键词

---

### Phase 4 — 构建与 CI/CD（1 天）

目标：每次 push 到 `feat/tui_migration`（或 main）自动部署。

- [x] 创建 `.github/workflows/website.yml`：
  - trigger: `push` 到 main / `feat/tui_migration`，或 `website/**`、`docs/**` 变更
  - job: pnpm install → 带 `GH_PAGES=true` 构建 website → deploy to `gh-pages`
- [x] Rspress `base` 配置按环境切换：本地 `/`，GitHub Pages `/HnsX/`
- [ ] 验证 GitHub Pages 站点能正常访问，资源路径正确
- [ ] 更新 `README.md` 和 `docs/cli-roadmap.md` 里的 `hnsx.dev` 链接说明（若已部署）

#### 验收
```bash
# push 后
open https://narcilee7.github.io/HnsX/
# 首页、文档页、搜索均正常
```

---

### Phase 5 — 打磨与 SEO（0.5–1 天）

目标：站点在社交媒体上分享时像模像样。

- [x] 配置 Rspress `head`：title、description、og:image、twitter:card
- [x] 生成/绘制 Open Graph 图片（1200×630）和 favicon
- [x] 自定义 404 页面
- [ ] 检查所有内部链接是否可点（无死链）
- [ ] 加 `sitemap.xml`（Rspress 插件或构建后生成）

---

## 4. 风险与依赖

| 风险 | 应对 |
|---|---|
| Rspress 与 React 19 workspace 冲突 | website 独立依赖，不继承 workspace React |
| `docs/` 里 Markdown 有非标准语法导致 Rspress 渲染异常 | Phase 3 先 build，遇到报错再逐个修复 frontmatter |
| GitHub Pages 子路径 `/HnsX/` 导致资源 404 | 配置 Rspress `base: '/HnsX/'` 并在 workflow 后用 `actions/deploy-pages` |
| 后续切自定义域名 `hnsx.dev` | 只需改 DNS + `base: '/'`，workflow 不变 |

---

## 5. 验收总标准

- [x] `cd website && pnpm build` 成功，产物在 `website/dist`
- [ ] GitHub Pages 站点可访问，首页、文档页、搜索、导航正常
- [x] 站点风格与 `hnsx-console` 一致（字体、主色、简洁感）
- [x] 不破坏现有 `docs/` 的 GitHub 直接浏览
- [x] 不破坏现有 monorepo 的 `pnpm -r build` / `pnpm -r test`

---

## 6. 参考

- Rspress 文档：https://rspress.dev/
- Rspress 多语言 / home 页配置：https://rspress.dev/guide/basic/home-page
- GitHub Pages + pnpm + Rspack 部署示例：community actions `peaceiris/actions-gh-pages`
- 现有设计文档入口：[`docs/vision.md`](../../vision.md)、[`docs/tech_overview.md`](../../tech_overview.md)
