# HnsX Web Console 整体设计

> HnsX Web Console 不是低代码编辑器，而是 Harness 的**运维与审计控制中心**。核心目标是让用户能管理 Domain、触发 Session、实时观测执行过程、查看 Trace 与 Eval 结果。

---

## 1. 设计定位

### 1.1 不是低代码平台

- 不提供拖拽式 Workflow 编辑器。
- 不替用户生成复杂 prompt。
- 不隐藏 Harness 的声明式配置。

### 1.2 是运维与审计中心

- 管理 Domain：注册、版本化、查看配置。
- 触发与监控 Session：实时看 Agent 执行过程。
- 查询 Trace：复盘一次 Session 的完整执行记录。
- 查看 Eval：评测结果、版本对比、回归检测。
- 审批 Human-in-the-loop：处理需要人工确认的操作。
- 查看 Metric / AuditLog：成本、性能、安全事件。

### 1.3 用户角色

| 角色 | 主要功能 |
|---|---|
| **平台管理员** | Domain Registry 管理、Runtime 集群、Secret/Policy、审计日志 |
| **Harness 开发者** | 编写/调试 Domain YAML、运行 Eval、查看 Trace |
| **业务运营** | 查看 Session 状态、处理人工审批、查看 Metric 大盘 |
| **安全/合规** | 查看 AuditLog、Policy 违规、成本分析 |

---

## 2. 技术栈

| 层级 | 技术 | 理由 |
|---|---|---|
| 框架 | React 19 + TypeScript | 现代、类型安全、生态成熟 |
| 构建 | Vite | 快速 HMR、简单配置 |
| 路由 | React Router v7 | 声明式路由 |
| 状态管理 | Zustand | 轻量全局状态 |
| 服务端状态 | TanStack Query | 缓存、轮询、错误处理 |
| UI 组件 | Tailwind CSS + Shadcn/ui | 原子化样式 + 无障碍基础组件 |
| 表格 | TanStack Table | 复杂表格、排序、筛选 |
| 表单 | React Hook Form + Zod | 类型安全表单校验 |
| 编辑器 | Monaco Editor | YAML/JSON 编辑，不造轮子 |
| 图表 | Recharts / Tremor | 趋势图、柱状图 |
| Trace 可视化 | 嵌入 Grafana / Tempo / Jaeger | 不自研复杂 Trace UI |
| 实时推送 | SSE | Session 实时 Observation |
| 协议 | Protobuf 生成的 TS 类型 + REST API | 与控制面对接 |

---

## 3. 路由设计

```
/                              # 首页：Dashboard
/domains                       # Domain 列表
/domains/:id                   # Domain 详情（编辑器 + 信息 + 版本）
/domains/:id/run               # 触发新 Session
/sessions                      # Session 列表
/sessions/:id                  # Session 详情（实时/历史 Trace 时间线）
/traces                        # Trace 查询
/traces/:id                    # Trace 详情
/evals                         # EvalSet 列表
/evals/:setId                  # EvalSet 详情
/evals/:setId/runs/:runId      # EvalRun 报告
/observability                 # Metric 大盘（嵌入 Grafana）
/audit                         # AuditLog 列表
/approvals                     # 人工审批待办
/settings                      # 系统设置（Secret、Policy、Runtime）
```

---

## 4. Layout 设计

### 4.1 整体布局

```text
┌─────────────────────────────────────────────────────────────┐
│  TopBar: Logo + Search + Notifications + User                │
├──────────┬──────────────────────────────────────────────────┤
│          │                                                  │
│ Sidebar  │              Main Content                        │
│          │                                                  │
│          │                                                  │
└──────────┴──────────────────────────────────────────────────┘
```

### 4.2 Sidebar 导航

- Dashboard
- Domains
- Sessions
- Traces
- Evals
- Observability
- Audit
- Approvals
- Settings

### 4.3 响应式

- 桌面：Sidebar 常驻
- 平板：Sidebar 可折叠
- 手机：BottomBar 或 Drawer

---

## 5. 页面设计

### 5.1 Dashboard

- 核心指标卡片：总 Session 数、成功率、平均成本、活跃 Domain 数
- 最近 Session 列表
- 待处理人工审批数
- 最近的 Policy 违规/Eval 回归告警
- 成本趋势图（7 天）

### 5.2 Domain 列表

- 表格：ID、Version、Mode、Agents、Last Updated、Status
- 搜索 + 筛选
- 操作：View、Run、Eval、Delete
- 注册新 Domain 按钮

### 5.3 Domain 详情

三栏 Tab：

1. **Editor**：Monaco Editor 编辑 YAML，支持 Validate、Save、Run
2. **Info**：Harness 五要素可视化（Agents、Skills、Tools、Policy、Sandbox）
3. **Versions**：历史版本对比、回滚

### 5.4 Session 列表

- 表格：Session ID、Domain、State、Started At、Duration、Cost
- 状态筛选：running / completed / failed / paused
- 实时自动刷新
- 操作：View Trace、Rerun

### 5.5 Session 详情

- **时间线视图**：Observation 按时间展开，支持嵌套 Step/Turn/Tool call
- **实时模式**：SSE 推送新 Observation
- **结构化信息**：Domain、Agent、Cost、Duration、State
- **操作**：Approve/Reject（如果是 paused）

### 5.6 Trace 查询

- 按 Domain / Session / Time Range / Agent 筛选
- 列表展示 Trace 摘要
- 点击跳转 Session 详情

### 5.7 Eval

- **EvalSet 列表**：ID、Description、Cases 数、Last Run
- **EvalSet 详情**：Cases 列表、期望结果
- **EvalRun 报告**：总分、Case 明细、与 Baseline 对比、Observation 对比
- 操作：Run Eval、Set Baseline、Export Report

### 5.8 Observability

- 嵌入 Grafana Dashboard
- 提供默认 Dashboard：Session / Agent / Tool / Cost / Policy
- 用户可自定义

### 5.9 AuditLog

- 表格：Timestamp、Action、Actor、Resource、Decision、Reason
- 不可变标识
- 导出 CSV/JSON

### 5.10 Approvals

- 待办列表：Session、Step、请求的操作、风险说明
- 操作：Approve / Reject / Add Comment
- 历史审批记录

---

## 6. 组件设计

### 6.1 通用组件

| 组件 | 说明 |
|---|---|
| `PageHeader` | 页面标题 + 面包屑 + 操作按钮 |
| `DataTable` | 基于 TanStack Table 的通用表格 |
| `StatusBadge` | Session/Step 状态标签 |
| `CostBadge` | 成本显示 |
| `Timestamp` | 统一时间格式化 |
| `Loading` / `Empty` / `Error` | 状态占位组件 |

### 6.2 业务组件

| 组件 | 说明 |
|---|---|
| `DomainEditor` | Monaco Editor + Validate/Save/Run 工具栏 |
| `HarnessVisualizer` | 展示 Agents/Skills/Tools/Policy/Sandbox |
| `ObservationTimeline` | Observation 时间线，支持嵌套展开 |
| `ObservationCard` | 单个 Observation 展示 |
| `SessionController` | 触发 Session、展示状态 |
| `EvalReportView` | EvalRun 报告展示 |
| `AuditLogTable` | 审计日志表格 |
| `ApprovalCard` | 人工审批卡片 |

### 6.3 布局组件

| 组件 | 说明 |
|---|---|
| `AppShell` | TopBar + Sidebar + Main |
| `Sidebar` | 导航菜单 |
| `TopBar` | 顶部栏 |

---

## 7. 状态管理

### 7.1 Zustand 全局状态

- `authStore`：用户认证信息
- `notificationStore`：全局通知
- `settingsStore`：用户偏好设置

### 7.2 TanStack Query 服务端状态

- Domain 列表/详情
- Session 列表/详情
- Trace 列表/详情
- EvalSet/EvalRun
- AuditLog
- Approvals

### 7.3 本地状态

- 表单状态用 React Hook Form
- UI 状态用 useState/useReducer

---

## 8. API 设计（前端视角）

### 8.1 REST API

| 端点 | 说明 |
|---|---|
| `GET /api/v1/domains` | Domain 列表 |
| `GET /api/v1/domains/:id` | Domain 详情 |
| `POST /api/v1/domains` | 注册 Domain |
| `PUT /api/v1/domains/:id` | 更新 Domain |
| `POST /api/v1/domains/:id/run` | 触发 Session |
| `GET /api/v1/sessions` | Session 列表 |
| `GET /api/v1/sessions/:id` | Session 详情 |
| `GET /api/v1/sessions/:id/trace` | Session Trace |
| `GET /api/v1/traces` | Trace 查询 |
| `GET /api/v1/evals` | EvalSet 列表 |
| `POST /api/v1/evals/:id/run` | 运行 Eval |
| `GET /api/v1/evals/:id/runs/:runId` | EvalRun 报告 |
| `GET /api/v1/audit` | AuditLog |
| `GET /api/v1/approvals` | 待审批列表 |
| `POST /api/v1/approvals/:id/approve` | 审批通过 |
| `POST /api/v1/approvals/:id/reject` | 审批拒绝 |

### 8.2 SSE 端点

| 端点 | 说明 |
|---|---|
| `GET /api/v1/sessions/:id/events` | Session 实时 Observation 流 |

---

## 9. 权限设计

| 页面 | 管理员 | 开发者 | 运营 | 安全 |
|---|---|---|---|---|
| Dashboard | ✓ | ✓ | ✓ | ✓ |
| Domains | ✓ | ✓ | 只读 | 只读 |
| Sessions | ✓ | ✓ | ✓ | ✓ |
| Traces | ✓ | ✓ | ✓ | ✓ |
| Evals | ✓ | ✓ | 只读 | 只读 |
| Observability | ✓ | ✓ | ✓ | ✓ |
| Audit | ✓ | 只读 | 只读 | ✓ |
| Approvals | ✓ | ✓ | ✓ | ✓ |
| Settings | ✓ | 部分 | 无 | 部分 |

---

## 10. 性能与体验

### 10.1 性能

- 大 Trace 分页/虚拟滚动加载
- 表格排序筛选前端/后端结合
- Monaco Editor 懒加载
- 图片/图表按需加载

### 10.2 实时性

- Session 详情页 SSE 实时更新
- Session 列表定时轮询 + 后台刷新
- 审批列表实时推送

### 10.3 错误处理

- API 错误统一 Toast 提示
- 网络断开重连提示
- 表单校验即时反馈

---

## 11. 实现阶段

### Phase 1：骨架

- Layout + 路由
- Domain 列表/详情（Monaco Editor）
- Session 列表/详情（静态 Trace）

### Phase 2：核心功能

- SSE 实时 Observation
- Trace 查询
- Eval 列表/报告

### Phase 3：运维能力

- AuditLog
- Approvals
- Metric 大盘（嵌入 Grafana）

### Phase 4：高级功能

- Domain 版本对比
- Eval baseline 对比
- 自定义 Dashboard

---

## 12. 设计原则

1. **信息密度优先**：控制台类产品，一页展示足够信息。
2. **不造轮子**：复杂组件用开源方案（Monaco、TanStack、Grafana）。
3. **实时可观测**：Session 执行过程必须能实时看到。
4. **审计不可变**：AuditLog 展示不可修改、不可删除。
5. **权限清晰**：不同角色看到不同功能和数据。
6. **API 驱动**：Console 只是 Control Plane API 的消费者。
