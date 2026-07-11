# CLI 常用命令

HnsX CLI 采用 `hnsx <verb> <resource>` 的资源导向命名，类似 `kubectl` / `gh`。

## 初始化与运行

```bash
hnsx init --template customer-service --output-dir my-domain
hnsx validate --domain my-domain/domain.yaml
hnsx run --domain my-domain/domain.yaml --trigger '{"question":"hello"}'
```

## 资源管理

```bash
hnsx domain list
hnsx domain show customer-service
hnsx domain apply --file my-domain/domain.yaml

hnsx session list
hnsx session show <session-id>

hnsx trace list
hnsx trace show <trace-id>
```

## 治理

```bash
hnsx policy list
hnsx secret set OPENAI_API_KEY
hnsx approval list
hnsx approval approve <approval-id>
```

## 输出格式

所有 list/show 命令支持三种输出：

```bash
hnsx session list                 # 默认 human 表格
hnsx session list --output json   # JSON
hnsx session list --output quiet  # 仅 id
```

## 配置优先级

`--flag` > `HNSX_*` 环境变量 > `~/.config/hnsx/*.yaml`

## TUI

```bash
hnsx        # 无参数进入 TUI
hnsx --no-tui  # 本次禁用 TUI
```
