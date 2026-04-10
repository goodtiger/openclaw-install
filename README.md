# OpenClaw 中国区安装器

面向中国网络环境的 OpenClaw 命令行安装器，支持国内镜像加速和主流 LLM 供应商。

## 快速开始

### 一键安装（推荐）

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/goodtiger/openclaw-install/main/scripts/install.sh | bash

# 指定版本
curl -fsSL https://raw.githubusercontent.com/goodtiger/openclaw-install/main/scripts/install.sh | bash -s -- --version 0.2.0
```

### 手动安装

```bash
# 1. 环境诊断
./openclaw-install doctor

# 2. 交互式安装
./openclaw-install install

# 3. 快速安装（非交互）
./openclaw-install install --yes --provider bailian --api-key sk-xxx
```

## 平台支持

| 系统 | Docker | Native |
|------|--------|--------|
| Linux | ✅ | ✅ |
| macOS | ✅ | ✅ |
| Windows | ✅ | ✅ (推荐 Docker) |

## 命令速查

| 命令 | 说明 |
|------|------|
| `install` | 交互式安装 |
| `install --yes` | 快速安装 |
| `doctor` | 环境诊断 |
| `reconfigure` | 重配置 |
| `channels list` | 查看通道 |
| `providers list` | 查看供应商 |
| `validate` | 验证配置 |
| `upgrade` | 自我升级 |
| `bridge serve --channel feishu` | 启动 Bridge |

## 内置预设

**LLM 供应商**：百炼（默认）、DeepSeek、智谱、Kimi、豆包、自定义 OpenAI

**Channel**：微信（默认）、QQ、飞书、企业微信

## 配置产物

安装后生成：

- `~/.openclaw/openclaw.json` - OpenClaw 主配置
- `~/.openclaw/bridge.json` - Bridge 服务配置
- `~/.openclaw/install-state.json` - 安装状态
- `~/.openclaw/runtime/` - 运行时文件（Docker 模式）

## 升级

```bash
./openclaw-install upgrade
```

自动从 GitHub releases 下载最新版本，支持 ghproxy 镜像回退，SHA256 校验，原子替换。

## 更新日志

### v0.2.0 — 中国区网络优化

- 🚀 一键安装脚本 `scripts/install.sh`，支持 ghproxy 镜像回退
- 🔄 `upgrade` 命令增加 GitHub API 和下载镜像回退（直连 → ghproxy）
- 🌐 所有 HTTP 客户端支持系统代理（`HTTP_PROXY`/`HTTPS_PROXY`）
- 🐳 Docker 镜像增加阿里云源候选（`registry.cn-hangzhou.aliyuncs.com`）
- 📦 Dockerfile 支持 `HTTP_PROXY`/`HTTPS_PROXY` build-arg 传递
- 🔍 `doctor` 命令增加 GitHub 连通性检测
- 🛠 新增 `GoProxyEnv` 辅助方法，支持 Go 模块代理配置（`GOPROXY` + `GOSUMDB`）
- 🐛 修复 `upgrade` 中循环内 `defer` 导致的资源泄漏问题

## 常见问题

**Q: 安装失败，提示镜像无法访问？**

A: 运行 `./openclaw-install doctor --preview` 查看镜像探测结果，安装器会自动回退到官方源。

**Q: 如何切换 LLM 供应商？**

A: 运行 `./openclaw-install reconfigure` 或手动编辑 `~/.openclaw/openclaw.json`。

**Q: 微信通道如何配置？**

A: 默认启用，安装时扫码登录即可，无需填写凭证。

**Q: 飞书/企业微信如何配置？**

A: 需要提供 AppID、AppSecret、Verification Token，安装器会生成 bridge 服务并注册后台运行。

## 构建发布

```bash
# 本地构建
go build ./cmd/openclaw-install

# 发布多平台
scripts/build-release.sh
```

生成目录：`dist/archives/`

---

**最新版本**: v0.2.0 | [Releases](https://github.com/goodtiger/openclaw-install/releases)
