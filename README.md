# OpenClaw 中国区安装器

面向中国网络环境的 OpenClaw 命令行安装器，支持国内镜像加速和主流 LLM 供应商。

## 快速开始

### 一键安装（推荐）

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/goodtiger/openclaw-install/main/scripts/install.sh | bash

# 指定版本
curl -fsSL https://raw.githubusercontent.com/goodtiger/openclaw-install/main/scripts/install.sh | bash -s -- --version 0.3.1
```

安装脚本自动检测系统架构，下载对应二进制，执行 SHA256 校验，并添加 PATH 提示。

### Homebrew 安装

```bash
# 添加 tap（首次）
brew tap goodtiger/tap

# 安装
brew install openclaw-install
```

### 手动安装

```bash
# 1. 环境诊断
openclaw-install doctor

# 2. 交互式安装
openclaw-install install

# 3. 快速安装（非交互）
openclaw-install install --yes --provider bailian --api-key sk-xxx
```

## 平台支持

| 系统 | 架构 | Docker | Native |
|------|------|--------|--------|
| Linux | amd64 / arm64 | ✅ | ✅ |
| macOS | amd64 / arm64 (Apple Silicon) | ✅ | ✅ |
| Windows | amd64 | ✅ | ✅ (推荐 Docker) |

## 命令速查

### 安装与升级

| 命令 | 说明 |
|------|------|
| `install` | 交互式安装 |
| `install --yes` | 快速安装（非交互） |
| `upgrade` | 自我更新到最新版本 |
| `upgrade --rollback-on-failure` | 升级失败时自动回滚 |
| `rollback` | 从备份手动恢复上一个版本 |
| `reconfigure` | 重配置 provider/channel |

### 诊断与验证

| 命令 | 说明 |
|------|------|
| `doctor` | 环境诊断（默认检查项） |
| `doctor --list` | 列出所有可用检查项 |
| `doctor --all` | 运行全部检查项（含扩展检查） |
| `doctor --fix` | 尝试自动修复失败的检查项 |
| `doctor --check=docker,node` | 运行指定检查项（逗号分隔） |
| `doctor --json` | JSON 格式输出（CI/CD 集成） |
| `doctor --json --strict` | 任何警告/失败返回非零退出码 |
| `doctor --preview` | 预览自动检测的配置值 |
| `validate` | 验证配置文件格式与有效性 |

### 通道与供应商

| 命令 | 说明 |
|------|------|
| `channels list` | 查看所有通道及已启用状态 |
| `providers list` | 查看所有内置供应商预设 |
| `bridge serve --channel feishu` | 启动 Bridge 服务 |

### 其他

| 命令 | 说明 |
|------|------|
| `version` | 输出版本号 |
| `help` | 显示帮助信息 |

## 内置预设

**LLM 供应商**：百炼（默认）、DeepSeek、智谱、Kimi、豆包、自定义 OpenAI

**Channel**：微信（默认）、QQ、钉钉、飞书、企业微信

## 配置产物

安装后生成：

- `~/.openclaw/openclaw.json` — OpenClaw 主配置
- `~/.openclaw/bridge.json` — Bridge 服务配置
- `~/.openclaw/install-state.json` — 安装状态
- `~/.openclaw/runtime/` — 运行时文件（Docker 模式）
- `~/.openclaw/state/update-check.json` — 版本检查缓存

## 特性详解

### 版本自动提示

每次执行命令后，安装器会自动检查新版本（24 小时缓存，不阻塞操作）。发现更新时会提示：

```
💡 A new version is available: v0.3.1 → v0.3.2
   Run: openclaw-install upgrade
```

可通过环境变量关闭：`export OPENCLAW_NO_UPDATE_NOTIFIER=1`

### Doctor 诊断系统

模块化架构，支持 14 项独立检查：

| 检查项 | 默认 | 说明 |
|--------|------|------|
| docker | ✅ | Docker 二进制检测 |
| docker-compose | ✅ | Docker Compose 可用性 |
| docker-daemon | ✅ | Docker 守护进程健康 |
| node | ✅ | Node.js 运行时 |
| npm | ✅ | npm 包管理器 |
| openclaw | ✅ | OpenClaw CLI |
| git | ✅ | Git 版本控制 |
| curl | ✅ | curl 下载工具 |
| package-manager | ✅ | 系统包管理器检测 |
| github-connectivity | ✅ | GitHub 网络连通性 |
| mirror-resolution | ✅ | 镜像源可达性 |
| windows-docker-recommendation | ✅ | Windows 平台 Docker 建议 |
| go-version | 扩展 | Go 版本检查 |
| proxy-env | 扩展 | 代理环境变量检测 |

### 可操作的错误信息

所有错误消息均附带修复建议，不再只显示 "失败"：

```
❌ 无法连接到 GitHub API
💡 设置代理: export HTTPS_PROXY=http://your-proxy:port
```

### 升级回滚机制

升级器在替换二进制前会：
1. 创建备份（`{binary}.backup`）
2. 验证新版本可正常执行
3. 若验证失败，自动恢复备份

也可手动回滚：`openclaw-install rollback`

### 安装脚本安全校验

`scripts/install.sh` 支持 SHA256 校验验证，确保下载的归档文件未被篡改。校验失败时拒绝安装。

## 更新日志

### v0.3.1 — 新增 DingTalk 插件通道

- 新增钉钉（DingTalk）插件通道，支持企业内部机器人 Stream 模式，无需公网 IP
- 安装流程会自动通过 `openclaw config set` 写入 `clientId`、`clientSecret` 与访问策略
- 补充钉钉通道预设和对应配置测试

### v0.3.0 — 体验全面提升

- 🔒 `install.sh` 增加 SHA256 校验，防止下载文件被篡改（借鉴 k3s 模式）
- 🔔 每次命令后自动提示新版本（24 小时缓存，不阻塞操作）
- 📊 `doctor --json` 输出结构化 JSON，支持 CI/CD 流水线集成
- 🛠 `doctor --strict` 模式，任何警告/失败返回非零退出码
- 🏗 `doctor` 命令重构为模块化架构，新增 `--list`/`--fix`/`--all`/`--check` 标志
- 💡 所有错误消息附带可操作的修复建议（`💡` 提示）
- 🔄 `upgrade --rollback-on-failure` 升级失败自动回滚
- ↩️ 新增 `rollback` 命令，支持手动恢复备份版本
- 🍺 新增 Homebrew Formula，支持 `brew install openclaw-install`
- 🤖 自动生成 Homebrew tap 的 GitHub Actions 工作流

### v0.2.0 — 中国区网络优化

- 🔄 `upgrade` 命令新增 `--rollback-on-failure` 标志
- 🔄 新增 `rollback` 命令
- 🚀 一键安装脚本 `scripts/install.sh`，支持 ghproxy 镜像回退
- 🌐 所有 HTTP 客户端支持系统代理
- 🐳 Docker 镜像增加阿里云源候选
- 📦 Dockerfile 支持代理 build-arg 传递
- 🔍 `doctor` 命令增加 GitHub 连通性检测
- 🛠 新增 `GoProxyEnv` 辅助方法

## 常见问题

**Q: 安装失败，提示镜像无法访问？**

A: 运行 `openclaw-install doctor --preview` 查看镜像探测结果，安装器会自动回退到官方源。

**Q: 如何切换 LLM 供应商？**

A: 运行 `openclaw-install reconfigure` 或手动编辑 `~/.openclaw/openclaw.json`。

**Q: 微信通道如何配置？**

A: 默认启用，安装时扫码登录即可，无需填写凭证。

**Q: 飞书/企业微信如何配置？**

A: 需要提供 AppID、AppSecret、Verification Token，安装器会生成 bridge 服务并注册后台运行。

**Q: 如何关闭版本更新提示？**

A: 设置环境变量 `export OPENCLAW_NO_UPDATE_NOTIFIER=1`。

**Q: 升级失败怎么恢复？**

A: 如果使用 `--rollback-on-failure` 会自动恢复。否则运行 `openclaw-install rollback` 手动恢复。

**Q: 如何在 CI/CD 中使用 doctor 命令？**

A: 使用 `openclaw-install doctor --json --strict`，JSON 格式输出 + 失败时返回非零退出码。

## 构建发布

```bash
# 本地构建
go build ./cmd/openclaw-install

# 发布多平台
scripts/build-release.sh
```

生成目录：`dist/archives/`

---

**最新版本**: v0.3.1 | [Releases](https://github.com/goodtiger/openclaw-install/releases)
