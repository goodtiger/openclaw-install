# OpenClaw 中国区安装器

这是一个面向中国网络环境的 `OpenClaw` 命令行安装器。当前版本聚焦于第一版可用性，目标是让你在国内网络下更顺畅地完成以下工作：

- 检测本机环境并给出推荐安装模式
- 在安装过程中自动选择更容易访问的镜像/代理源
- 自动生成或增量合并 `~/.openclaw/openclaw.json`
- 默认生成百炼 Coding Plan 的模型配置
- 预置国内常见 LLM 供应商配置
- 默认提供 QQ `qqbot` 插件接入，并保留飞书、企业微信的 bridge 化 channel 接入
- 为 bridge 生成本地启动脚本，并在 Linux/macOS 尝试注册后台服务

当前实现已经可以编译、运行、生成配置和 bridge 资产，但仍属于 v1，重点是先把安装链路打通。

## 1. 支持范围

### 1.1 平台支持

- Linux：支持 `docker` 和 `native` 两种安装模式
- macOS：支持 `docker` 和 `native` 两种安装模式
- Windows：当前仅支持 `docker` 模式

如果在 Windows 选择 `native` 模式，程序会直接报错并拒绝继续。

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

- `qq`（默认，使用 `@sliverp/qqbot` 插件）
- `feishu`
- `wecom`

注意：

- QQ 默认不是 bridge，而是通过 `openclaw plugins install @sliverp/qqbot@latest` 和 `openclaw channels add --channel qqbot --token "AppID:AppSecret"` 配置
- 飞书、企业微信仍然通过本安装器生成的 bridge 服务接入

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

如果你在受限网络或沙箱里运行，`doctor` 可能会提示镜像探测失败并回退到默认源。这不是致命错误，表示本次无法验证可达性，但安装流程仍然可以继续。

## 4. 快速开始

### 4.1 交互式安装

最常见的方式：

```bash
./openclaw-install install
```

程序会依次询问你：

1. 安装模式：`docker` 或 `native`
2. LLM 供应商预设，默认是 `bailian`
3. `baseUrl`
4. `apiKey`
5. 主模型
6. fallback 模型
7. 是否启用 QQ / 飞书 / 企业微信，其中 QQ 默认启用
8. 每个 channel 所需的凭证字段；飞书/企微额外需要监听地址和回调路径
9. 最终确认

### 4.2 非交互快速配置

如果你只想先把 provider 配好，不先接 channel，可以使用：

```bash
./openclaw-install install \
  --yes \
  --mode native \
  --provider bailian \
  --api-key sk-xxxx \
  --primary-model qwen3.5-plus \
  --skip-verify
```

说明：

- `--yes` 会尽量使用默认值
- 这条命令默认会继续要求你填写 QQ 的 `AppID` 和 `AppSecret`
- `--skip-verify` 会跳过安装后的验证步骤

### 4.3 仅重新写配置，不重新安装

如果 OpenClaw 已经装好，只想切换 provider 或 channel：

```bash
./openclaw-install reconfigure
```

或者：

```bash
./openclaw-install reconfigure \
  --yes \
  --mode native \
  --provider bailian \
  --api-key your-key \
  --primary-model qwen-max
```

`reconfigure` 会：

- 备份已有 `openclaw.json`
- 重新生成 `bridge.json`
- 更新 runtime 资产
- 保留未被安装器托管的自定义配置
- 不重新安装 OpenClaw 本体

## 5. 命令说明

### 5.1 `doctor`

用法：

```bash
./openclaw-install doctor
```

用途：

- 查看当前系统是否更适合 `docker` 还是 `native`
- 判断是否已经装好 `docker` / `node` / `npm` / `openclaw`
- 查看镜像候选链是否能探测到可用源

### 5.2 `install`

用法：

```bash
./openclaw-install install [flags]
```

支持参数：

- `--mode`
- `--provider`
- `--base-url`
- `--api-key`
- `--primary-model`
- `--fallback-models`
- `--channels`
- `--yes`
- `--skip-verify`

参数说明：

- `--mode`：可选 `docker` 或 `native`
- `--provider`：provider preset id，例如 `bailian`
- `--base-url`：覆盖预设中的 API 地址
- `--api-key`：供应商 API Key
- `--primary-model`：主模型
- `--fallback-models`：逗号分隔的 fallback 模型列表
- `--channels`：逗号分隔的 channel id，例如 `qq,feishu`
- `--yes`：尽量直接使用默认值
- `--skip-verify`：跳过安装后的校验

重要说明：

- `--yes` 不是完全无人值守模式
- QQ 默认启用，因此即使使用 `--yes`，程序通常仍会继续询问 `QQ Bot AppID` 和 `QQ Bot AppSecret`
- 如果你启用了飞书或企微，程序仍然会继续询问必要的凭证字段

### 5.3 `reconfigure`

用法：

```bash
./openclaw-install reconfigure [flags]
```

参数基本与 `install` 相同，但不会重新安装 OpenClaw 本体。

适合场景：

- 切换到新的 LLM 供应商
- 修改 `baseUrl`
- 调整主模型和 fallback
- 增加或关闭 channel

### 5.4 `bridge serve`

用法：

```bash
./openclaw-install bridge serve --channel feishu
```

可选参数：

- `--channel`：必填，当前用于 bridge 类型 channel，支持 `feishu`、`wecom`
- `--config`：bridge 配置文件路径，默认是 `~/.openclaw/bridge.json`

作用：

- 单独启动某一个 channel 的 bridge 进程
- 适合本地调试 bridge 行为
- 适合排查回调路径和消息转发问题

例如：

```bash
./openclaw-install bridge serve --channel feishu
```

## 6. 配置文件与运行产物

安装器会使用 `~/.openclaw` 作为主目录。

### 6.1 `~/.openclaw/openclaw.json`

这是 OpenClaw 主配置文件。安装器会写入或更新这些内容：

- `meta.installer`
- `gateway.port`
- `gateway.bind`
- `models.primary`
- `models.fallbacks`
- `models.providers`
- `channels`

安装器不会粗暴覆盖整个文件，而是使用“增量合并”的方式处理：

- 保留未知字段
- 保留未由安装器管理的自定义配置
- 删除上一轮由安装器托管的 provider
- 删除上一轮由安装器托管的 channel
- 写入这一次新的 provider 和 channel

这意味着你手动加的其他配置通常会保留下来。

### 6.2 `~/.openclaw/bridge.json`

这是 bridge 服务的专用配置文件，包含：

- 当前 provider 信息
- 每个启用 channel 的监听地址
- 回调路径
- 凭证字段
- DM / Group policy

### 6.3 `~/.openclaw/install-state.json`

这是安装器自己的状态文件，用来记录：

- 安装器版本
- 安装时间
- 安装模式
- 当前托管的 provider id
- 当前托管的 channel 列表
- 选择到的镜像名称
- runtime 路径

安装器后续的 `reconfigure` 会依赖这个文件判断“哪些字段是上一轮由自己管理的”。

### 6.4 `~/.openclaw/.backups/`

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

## 7. Channel 配置说明

### 7.1 QQ

当前默认按 OpenClaw 插件方式接入，而不是 bridge。

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

### 7.2 飞书

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

### 7.3 企业微信

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

## 8. Bridge 服务注册行为

如果启用了 bridge 类型 channel，安装器会为每个 channel 生成独立 bridge 启动脚本。QQ 默认走插件方式，不会生成 QQ bridge 脚本。

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

## 9. 推荐使用流程

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

## 10. 验证安装结果

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
openclaw version
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

## 11. 常见问题

### 11.1 `doctor` 一直提示镜像探测回退

这通常说明：

- 当前网络无法访问探测地址
- 当前环境限制了 HTTP 探测
- 你在沙箱/容器/受限服务器里运行

结果不一定代表安装一定失败，只表示这一步无法确认最佳镜像。

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

- 默认 QQ 启用后仍然需要输入 `AppID` / `AppSecret`
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

## 12. 已知限制

当前 v1 的限制包括：

- Windows 仅支持 Docker 模式
- `--yes` 不是完整无人值守安装
- channel 侧仍然需要你自己提前准备平台凭证
- 企业微信适配目前偏基础链路打通，不是完整平台集成
- Docker 模式当前使用“本地构建 Node 镜像并 npm 安装 OpenClaw”的方式，而不是直接拉官方 OpenClaw 镜像
- 镜像候选链已实现，但实际可用性仍取决于你所在网络环境
- QQ 插件 channel 依赖本机 OpenClaw CLI 在真实环境里成功执行 `plugins install` 和 `channels add`

## 13. 当前仓库中最重要的实现位置

如果你需要继续开发或排查问题，优先看这些文件：

- [cmd/openclaw-install/main.go](/media/data/code/openclaw-install/cmd/openclaw-install/main.go)
- [internal/app/app.go](/media/data/code/openclaw-install/internal/app/app.go)
- [internal/install/workflow.go](/media/data/code/openclaw-install/internal/install/workflow.go)
- [internal/install/assets.go](/media/data/code/openclaw-install/internal/install/assets.go)
- [internal/config/files.go](/media/data/code/openclaw-install/internal/config/files.go)
- [internal/bridge/server.go](/media/data/code/openclaw-install/internal/bridge/server.go)

## 14. 建议的下一步

如果你准备进入真实测试，建议顺序是：

1. `./openclaw-install doctor`
2. `./openclaw-install install`
3. 先只配置一个 provider，不先上 channel
4. 确认 OpenClaw 主体可以运行
5. 再单独接入一个 channel
6. 最后再做体验和交互优化

如果你要做真实机器联调，直接看这份手册：

- [TESTING.md](/media/data/code/openclaw-install/TESTING.md)
