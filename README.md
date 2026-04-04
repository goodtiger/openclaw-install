# OpenClaw 中国区安装器

这是一个面向中国网络环境的 `OpenClaw` 命令行安装器。当前版本聚焦于第一版可用性，目标是让你在国内网络下更顺畅地完成以下工作：

- 检测本机环境并给出推荐安装模式
- 在安装过程中自动选择更容易访问的镜像/代理源
- 自动生成或增量合并 `~/.openclaw/openclaw.json`
- 默认生成百炼 Coding Plan 的模型配置
- 预置国内常见 LLM 供应商配置
- 默认提供微信（个人微信 ClawBot 插件）接入，并保留 QQ、飞书、企业微信等 channel 接入
- 为 bridge 生成本地启动脚本，并在 Linux/macOS 尝试注册后台服务
- **v0.1.9 新增**：`upgrade` 命令自动升级安装器
- **v0.1.9 新增**：`channels list` 查看所有可用通道
- **v0.1.9 新增**：`providers list` 查看所有供应商预设
- **v0.1.9 新增**：`validate` 验证配置文件有效性

当前实现已经可以编译、运行、生成配置和 bridge 资产，但仍属于 v1，重点是先把安装链路打通。

## 1. 支持范围

### 1.1 平台支持

- Linux：支持 `docker` 和 `native` 两种安装模式
- macOS：支持 `docker` 和 `native` 两种安装模式
- Windows：支持 `docker` 和 `native`，但默认仍推荐 `docker`

如果你在 Windows 选择 `native` 模式，安装器会优先尝试直接使用本机的 Node.js/npm 和全局 OpenClaw CLI；首次装完 Node.js 后，建议重开一个终端再做后续手工排障。

### 1.2 安装模式说明

#### Docker 模式

Docker 模式会做这些事情：

- 检测或安装 Docker / Docker Compose
- 在 `~/.openclaw/runtime/` 生成：
  - `Dockerfile.openclaw`
  - `compose.yaml`
  - `.env`
- 使用 `node:22-bullseye` 构建本地镜像
- 在镜像内通过 npm 安装 `openclaw`
- 通过 `docker compose up -d --build` 启动 OpenClaw

适合：

- 想要隔离运行环境
- 希望 Linux/macOS/Windows 的体验尽量一致
- 本机已经装好 Docker

#### Native 模式

Native 模式会做这些事情：

- 检测或安装 `node` / `npm`
- 使用 `npm install -g openclaw` 安装 OpenClaw
- 生成本地启动脚本
- 尝试执行 `openclaw gateway start`

适合：

- 不想依赖 Docker
- 机器本身已经能正常运行 Node/npm

## 2. 已内置的预设

### 2.1 LLM 供应商预设

当前内置以下 provider preset：

- `bailian`（默认）
- `deepseek`
- `zhipu`
- `moonshot`
- `doubao`
- `custom-openai`

默认 provider 为 `bailian`，会生成百炼 Coding Plan 端点对应的：

- `models.providers.bailian`
- `agents.defaults.model.primary`
- `agents.defaults.models`

当前统一按 OpenAI Compatible 接口处理。

### 2.2 Channel 预设

当前内置以下 channel preset：

- `wechat`（默认，使用 `@tencent-weixin/openclaw-weixin` 插件）
- `qq`（使用 `@sliverp/qqbot` 插件）
- `feishu`
- `wecom`

注意：

- 微信默认通过 OpenClaw 插件方式接入，扫码登录，无需填写凭证
- QQ 同样通过插件方式接入，需要 `AppID` 和 `AppSecret` 凭证
- 飞书、企业微信通过本安装器生成的 bridge 服务接入

## 3. 构建与准备

### 3.1 本地构建

在仓库根目录执行：

```bash
go build ./cmd/openclaw-install
```

构建后会在当前目录得到可执行文件：

```bash
./openclaw-install
```

如果你要发给别人电脑使用，直接运行发布脚本即可生成多平台压缩包：

```bash
scripts/build-release.sh
```

默认会生成：

- `linux/amd64`
- `linux/arm64`
- `darwin/amd64`
- `darwin/arm64`
- `windows/amd64`

输出目录：

- `dist/packages/`
- `dist/archives/`

如果只想打指定平台：

```bash
scripts/build-release.sh linux/amd64 darwin/arm64 windows/amd64
```

### 3.2 建议先做环境诊断

第一次使用前，先运行：

```bash
./openclaw-install doctor
```

它会输出：

- 系统信息
- OpenClaw 目录位置
- 检测到的工具链
- 推荐安装模式
- 当前镜像探测结果
  - 可能的 warning

镜像选择现在默认按"国内优先、官方回退"处理，例如 npm 会优先尝试 `npmmirror`，Go 会优先尝试 `goproxy.cn`。

如果你在受限网络或沙箱里运行，`doctor` 可能会提示镜像探测失败并回退到默认源。这不是致命错误，表示本次无法验证可达性，但安装流程仍然可以继续。

## 4. 命令说明

### 5.1 命令列表

```text
openclaw-install install            # 交互式安装
openclaw-install doctor             # 环境诊断
openclaw-install reconfigure        # 重配置（不重新安装）
openclaw-install bridge serve       # 启动单个 bridge 通道
openclaw-install channels list      # 查看所有可用通道
openclaw-install providers list     # 查看所有供应商预设
openclaw-install validate           # 验证配置文件
openclaw-install upgrade            # 自我升级到最新版本
openclaw-install version            # 显示版本
```

### 5.2 `doctor` - 环境诊断

```bash
./openclaw-install doctor
./openclaw-install doctor --preview    # 预览检测到的配置值
```

用途：

- 查看系统信息（OS/Arch）
- 检查工具链（docker/node/npm/openclaw/git/curl）
- 推荐安装模式（docker/native）
- 显示镜像源探测结果

### 5.3 `install` - 安装 OpenClaw

```bash
./openclaw-install install
./openclaw-install install --yes       # 使用默认值快速安装
```

交互式安装会询问：

1. 安装模式：`docker` 或 `native`
2. LLM 供应商预设
3. API key 和 baseUrl
4. 主模型和 fallback 模型
5. 启用的通道（微信默认启用）
6. 各通道的凭证配置

### 5.4 `reconfigure` - 重配置

```bash
./openclaw-install reconfigure
./openclaw-install reconfigure --yes --provider bailian --api-key sk-xxx
```

用途：

- 不重新安装 OpenClaw 本体
- 修改 provider/channel 配置
- 备份旧配置并增量合并

### 5.5 `channels list` - 查看通道

```bash
./openclaw-install channels list
```

示例输出：

```text
可用通道：
  wechat     微信（个人微信 ClawBot 插件）            [插件] [已启用]
  qq         QQ（qqbot 插件）                   [插件]
  feishu     飞书                             [bridge]
  wecom      企业微信                           [bridge]

注：[插件] = OpenClaw 插件，[bridge] = 独立 bridge 服务
```

### 5.6 `providers list` - 查看供应商

```bash
./openclaw-install providers list
```

示例输出：

```text
可用供应商：
  bailian         阿里百炼 Coding Plan          [默认]  qwen3.5-plus
  deepseek        DeepSeek                    deepseek-chat
  zhipu           智谱 AI                       glm-4-plus
  moonshot        Moonshot/Kimi               moonshot-v1-8k
  doubao          豆包                          doubao-1-5-pro-32k
  custom-openai   自定义 OpenAI Compatible       gpt-4o-mini
```

### 5.7 `validate` - 验证配置

```bash
./openclaw-install validate
./openclaw-install validate --openclaw
./openclaw-install validate --bridge
./openclaw-install validate --state
```

验证：

- `openclaw.json` - 检查 JSON 格式和必要字段
- `bridge.json` - Bridge 服务配置
- `install-state.json` - 安装状态

### 5.8 `upgrade` - 自我升级

```bash
./openclaw-install upgrade
./openclaw-install upgrade --force
```

流程：

1. 查询 GitHub 最新版本
2. 下载对应平台二进制
3. SHA256 校验验证
4. 原子替换当前二进制

### 5.9 `bridge serve` - 启动 Bridge

```bash
./openclaw-install bridge serve --channel feishu
./openclaw-install bridge serve --channel wecom
```

用于单独启动 bridge 类型的通道服务。

### 5.10 `version` - 显示版本

```bash
./openclaw-install version
```

## 5. 配置文件与运行产物

### 4.1 `~/.openclaw/openclaw.json`

这是 OpenClaw 主配置文件。安装器会写入或更新这些内容：

- `gateway.port`
- `gateway.bind`
- `models.providers`
- `models.mode`
- `channels`
- `agents.defaults.model.primary`
- `agents.defaults.models`

安装器不会粗暴覆盖整个文件，而是使用“增量合并”的方式处理：

- 保留未知字段
- 保留未由安装器管理的自定义配置
- 删除上一轮由安装器托管的 provider
- 删除上一轮由安装器托管的 channel

其中 `gateway.bind` 会按新版 OpenClaw 配置写成：

- `loopback`，对应 Native 模式
- `lan`，对应 Docker 模式
- 写入这一次新的 provider 和 channel

这意味着你手动加的其他配置通常会保留下来。

### 4.2 `~/.openclaw/bridge.json`

这是 bridge 服务的专用配置文件，包含：

- 当前 provider 信息
- 每个启用 channel 的监听地址
- 回调路径
- 凭证字段
- DM / Group policy

### 4.3 `~/.openclaw/install-state.json`

这是安装器自己的状态文件，用来记录：

- 安装器版本
- 安装时间
- 安装模式
- 当前托管的 provider id
- 当前托管的 channel 列表
- 选择到的镜像名称
- runtime 路径

安装器后续的 `reconfigure` 会依赖这个文件判断“哪些字段是上一轮由自己管理的”。

### 4.4 `~/.openclaw/.backups/`

如果发现已有 `openclaw.json`，安装器会在这里自动创建备份。

### 6.5 `~/.openclaw/runtime/`

运行期资产会写在这个目录。

Docker 模式常见文件：

- `Dockerfile.openclaw`
- `compose.yaml`
- `.env`
- `bridge-<channel>.sh`

Native 模式常见文件：

- `run-openclaw.sh` 或 `run-openclaw.cmd`
- `bridge-<channel>.sh` 或 `bridge-<channel>.cmd`

## 6. Channel 配置说明

### 7.1 微信（个人微信）

当前默认按 OpenClaw 插件方式接入。

不需要手动填写凭证，安装后通过扫码登录绑定微信账号。

前置条件：

- iOS 微信版本 8.0.70 及以上
- 微信 → 我 → 设置 → 插件 → 启用 ClawBot

安装器会执行：

```bash
openclaw plugins install "@tencent-weixin/openclaw-weixin"
openclaw config set plugins.entries.openclaw-weixin.enabled true
openclaw channels login --channel openclaw-weixin
```

工作方式：

- 安装器安装 `openclaw-weixin` 插件
- 启用插件后发起扫码登录
- 用户用微信扫码确认绑定
- 微信消息通过 OpenClaw 原生 channel/plugin 链路处理

### 7.2 QQ

当前按 OpenClaw 插件方式接入（可选 channel，非默认）。

需要的主要字段：

- `QQ Bot AppID`
- `QQ Bot AppSecret`

安装器会执行：

```bash
openclaw plugins install @sliverp/qqbot@latest
openclaw channels add --channel qqbot --token "AppID:AppSecret"
```

工作方式：

- 安装器安装 `qqbot` 插件
- 使用 `AppID:AppSecret` 组装 token 并写入 OpenClaw channel 配置
- QQ 消息通过 OpenClaw 原生 channel/plugin 链路处理

### 7.3 飞书

当前按事件回调模式接入。

需要的主要字段：

- `App ID`
- `App Secret`
- `Verification Token`
- `Encrypt Key`（可选）

工作方式：

- 飞书把事件投递到 bridge 回调地址
- bridge 验证 challenge / token
- bridge 调用 LLM
- bridge 再调用飞书开放接口发送回复

### 7.4 企业微信

当前支持基础回调/机器人 webhook 方式。

可能使用到的字段：

- `Corp ID`
- `Agent ID`
- `Agent Secret / Webhook Key`
- `Callback Token`
- `Encoding AES Key`
- `Webhook URL`

说明：

- 当前实现优先保证 bridge 能接住请求并完成基础回复链路
- 不同企业微信接入形态差异较大，v1 更偏基础能力打通

## 7. Bridge 服务注册行为

如果启用了 bridge 类型 channel，安装器会为每个 channel 生成独立 bridge 启动脚本。微信和 QQ 都走插件方式，不会生成 bridge 脚本。飞书和企业微信使用 bridge 服务。

### 8.1 Linux

会尝试为 bridge 类型 channel 注册用户级 `systemd` 服务：

- `openclaw-bridge-feishu.service`
- `openclaw-bridge-wecom.service`

服务文件位置通常在：

```bash
~/.config/systemd/user/
```

### 8.2 macOS

会尝试注册 `launchd` 的 `LaunchAgent`。

### 8.3 Windows

当前只生成脚本，不自动注册后台服务。需要你手动启动：

```bash
openclaw-install bridge serve --channel feishu
```

## 8. 推荐使用流程

### 9.1 第一次部署

建议按下面顺序：

```bash
./openclaw-install doctor
./openclaw-install install
```

### 9.2 先验证配置生成，再碰真实环境

如果你想先做安全测试，可以把 `HOME` 指向一个临时目录：

```bash
HOME=/tmp/openclaw-smoke ./openclaw-install reconfigure \
  --yes \
  --mode native \
  --provider bailian \
  --api-key test-key \
  --primary-model qwen3.5-plus \
  --skip-verify
```

这样不会修改你真实的 `~/.openclaw`。

### 9.3 已装好 OpenClaw，只切换模型

```bash
./openclaw-install reconfigure \
  --yes \
  --mode native \
  --provider zhipu \
  --api-key your-key \
  --primary-model glm-4-plus
```

### 9.4 调试单个 channel

```bash
./openclaw-install bridge serve --channel feishu
```

如果要用自定义 bridge 配置：

```bash
./openclaw-install bridge serve --channel feishu --config /path/to/bridge.json
```

## 9. 验证安装结果

### 10.1 验证配置文件

检查是否生成：

```bash
ls ~/.openclaw
```

你应该至少看到：

- `openclaw.json`
- `bridge.json`
- `install-state.json`
- `runtime/`

### 10.2 验证 OpenClaw

Native 模式可以先试：

```bash
openclaw --version
openclaw config validate
```

Docker 模式可以这样验证 compose 配置：

```bash
cd ~/.openclaw/runtime
docker compose config
```

### 10.3 验证 bridge

例如启动 QQ bridge：

```bash
openclaw-install channels list
```

QQ 默认是插件 channel，不走 `bridge serve`。如果你要验证 QQ，优先看：

```bash
openclaw plugins list
openclaw channels list
```

如果要验证飞书或企微 bridge，再启动对应 bridge 进程并访问健康检查端点。

## 10. 常见问题

### 11.1 `doctor` 一直提示镜像探测回退

这通常说明：

- 当前网络无法访问探测地址
- 当前环境限制了 HTTP 探测
- 你在沙箱/容器/受限服务器里运行

当前策略是国内镜像优先、官方源回退。即使 `doctor` 提示回退，也不一定代表安装一定失败，只表示这一步无法确认最佳镜像。

### 11.2 `install` 过程中要求输入 sudo 权限

这是正常的。安装器会在缺少以下依赖时尝试安装：

- Docker
- Docker Compose
- Node.js
- npm

如果当前用户没有权限，会退回到 `sudo` 路径。

### 11.3 `--yes` 为什么还在提问

因为当前版本的 `--yes` 是“尽量接受默认值”，不是完整无人值守。

特别是：

- 微信默认启用，使用 `--yes` 会触发扫码登录流程
- 某些监听地址和路径也仍然可能继续询问

### 11.4 `reconfigure` 会不会把我手写的配置覆盖掉

正常情况下不会整文件覆盖。

当前策略是：

- 删除上一轮由安装器托管的 provider
- 删除上一轮由安装器托管的 channel
- 保留其他未知字段和自定义字段
- 再写入新一轮托管配置

### 11.5 为什么没有自动帮我申请飞书/企微/QQ 的机器人凭证

当前版本只负责：

- 采集你已有的凭证
- 生成配置
- 启动 bridge

不负责自动去各家平台创建应用或申请密钥。

## 11. 已知限制

当前 v1 的限制包括：

- Windows 下仍然默认推荐 Docker；`native` 属于 best-effort 路径，首次安装 Node.js 后最好重开终端
- `--yes` 不是完整无人值守安装
- channel 侧仍然需要你自己提前准备平台凭证
- 企业微信适配目前偏基础链路打通，不是完整平台集成
- Docker 模式当前使用“本地构建 Node 镜像并 npm 安装 OpenClaw”的方式，而不是直接拉官方 OpenClaw 镜像
- 镜像候选链已实现，但实际可用性仍取决于你所在网络环境
- 微信和 QQ 插件 channel 依赖本机 OpenClaw CLI 在真实环境里成功执行 `plugins install` 和 `channels login` / `channels add`

## 12. 当前仓库中最重要的实现位置

如果你需要继续开发或排查问题，优先看这些文件：

- [cmd/openclaw-install/main.go](/media/data/code/openclaw-install/cmd/openclaw-install/main.go)
- [internal/app/app.go](/media/data/code/openclaw-install/internal/app/app.go)
- [internal/install/workflow.go](/media/data/code/openclaw-install/internal/install/workflow.go)
- [internal/install/assets.go](/media/data/code/openclaw-install/internal/install/assets.go)
- [internal/config/files.go](/media/data/code/openclaw-install/internal/config/files.go)
- [internal/bridge/server.go](/media/data/code/openclaw-install/internal/bridge/server.go)

## 13. 建议的下一步

如果你准备进入真实测试，建议顺序是：

1. `./openclaw-install doctor`
2. `./openclaw-install install`
3. 先只配置一个 provider，不先上 channel
4. 确认 OpenClaw 主体可以运行
5. 再单独接入一个 channel
6. 最后再做体验和交互优化

如果你要做真实机器联调，直接看这份手册：

- [TESTING.md](/media/data/code/openclaw-install/TESTING.md)
