# 安装

HnsX 提供三种安装方式，按推荐程度排序。

## 1. curl 安装（推荐）

```bash
curl -sSL https://raw.githubusercontent.com/narcilee7/HnsX/main/scripts/install.sh | sh
```

脚本会自动检测平台、下载最新 release、校验 checksum 并把 `hnsx` 加到 `PATH`。

## 2. Homebrew（macOS / Linux）

```bash
brew tap narcilee7/hnsx
brew install narcilee7/hnsx/hnsx
```

## 3. 源码构建

```bash
git clone https://github.com/narcilee7/HnsX.git
cd HnsX
make build-cli
./bin/hnsx version
```

## 升级

```bash
hnsx update --check
hnsx update
```

## 验证安装

```bash
hnsx version
hnsx doctor
```

## 下一步

- 想**本地点点前端**体验产品 → 看 [`local-dev.md`](./local-dev.md)
- CLI 命令速查 → [`cli-basics.md`](./cli-basics.md)
- Domain YAML 字段参考 → [`domain-spec.md`](./domain-spec.md)
