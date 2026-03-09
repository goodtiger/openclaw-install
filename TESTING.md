# OpenClaw 安装器真实环境测试手册

这份文档用于指导你在真实机器上验证当前版本的 `openclaw-install`。目标不是一次性把所有能力都跑满，而是按风险从低到高逐步确认：

1. 安装器本身可运行
2. OpenClaw 主体可安装
3. 配置文件可正确生成与合并
4. LLM provider 可真实联通
5. Channel bridge 可接住回调并返回消息

建议严格按顺序测试，不要一上来就同时启用多个 provider 和多个 channel。

## 1. 测试目标

本轮真实测试建议只验证下面这些能力：

- `doctor` 是否能正确识别当前机器环境
- `install` 是否能完成第一次真实安装
- `reconfigure` 是否能安全覆盖安装器托管字段
- `openclaw.json` / `bridge.json` / `install-state.json` 是否生成正确
- Docker 或 Native 安装链路至少有一条可用
- 至少一个国内 LLM provider 可以成功返回结果
- 至少一个 channel bridge 可以跑起来并通过健康检查

这轮不建议一开始就测试：

- 多 provider 自动切换
- 三个 channel 同时启用
- 企业微信复杂企业内部权限模型
- Windows Docker 路径之外的能力

## 2. 测试前准备

### 2.1 机器要求

建议准备一台 Linux 机器做第一轮实测，优先级如下：

1. Ubuntu / Debian
2. 其他常见 Linux 发行版
3. macOS
4. Windows

原因很简单：当前实现里 Linux 路径最容易观察和排错。

### 2.2 准备一个干净窗口

建议新开一个 shell，不要复用很多历史环境变量的终端。至少先确认：

```bash
pwd
which go
which docker || true
which npm || true
which openclaw || true
echo "$HOME"
```

### 2.3 编译当前安装器

在仓库根目录执行：

```bash
go build ./cmd/openclaw-install
```

确认二进制存在：

```bash
ls -l ./openclaw-install
```

### 2.4 明确这次测试会改哪里

当前安装器会使用以下路径：

- `~/.openclaw/openclaw.json`
- `~/.openclaw/bridge.json`
- `~/.openclaw/install-state.json`
- `~/.openclaw/.backups/`
- `~/.openclaw/runtime/`

如果你的机器上已经有真实使用中的 OpenClaw，请先备份：

```bash
mkdir -p ~/.openclaw/manual-backups
cp -a ~/.openclaw ~/.openclaw/manual-backups/pre-openclaw-install-$(date +%Y%m%d_%H%M%S) 2>/dev/null || true
```

### 2.5 提前准备 provider 凭证

第一轮建议只准备一个 provider，例如：

- 阿里百炼
- DeepSeek
- 智谱

你至少需要准备：

- API Key
- 想要测试的主模型名

### 2.6 如果要测 channel，提前准备这些信息

#### QQ

- QQ Bot AppID
- QQ Bot AppSecret

#### 飞书

- App ID
- App Secret
- Verification Token
- Encrypt Key（如果启用）

#### 企业微信

- Webhook URL 或企业应用回调所需字段
- Token
- Encoding AES Key

第一轮只建议选一个 channel 做实测。

## 3. 建议的测试顺序

建议按以下顺序：

1. 环境诊断
2. 只安装 OpenClaw，不启用 channel
3. 验证 provider 配置是否写入正确
4. 验证 OpenClaw 主体是否能启动
5. 再单独启用一个 channel
6. 最后做 `reconfigure`

## 4. 第一阶段：环境诊断

先运行：

```bash
./openclaw-install doctor
```

你需要重点看这几项：

- `Recommended mode`
- `docker`
- `docker compose`
- `node`
- `npm`
- `openclaw`
- `Package manager`

### 4.1 结果判断

#### 可以继续

满足任意一条即可继续：

- `Recommended mode: docker` 且 Docker / Compose 已可用
- `Recommended mode: native` 且 Node / npm 已可用
- 虽然依赖没装，但机器上有包管理器，例如 `apt-get`

#### 建议先修环境

出现下面情况，建议先停下来：

- Windows 且 Docker 未安装
- Linux/macOS 既没有 Docker，也没有 Node/npm，同时没有可用包管理器
- `doctor` 输出异常，连最基本系统信息都不对

### 4.2 关于镜像 warning

如果你看到类似：

- `mirror category ... fell back to ... after probe failures`

说明只是镜像探测没能成功验证，不代表实际安装一定失败。真实下载时是否能成功，还要看你机器的实际网络。

## 5. 第二阶段：只测 OpenClaw 主体安装

这一步先不要启用任何 channel。

### 5.1 Docker 路径测试

如果你想优先走 Docker：

```bash
./openclaw-install install \
  --mode docker \
  --provider bailian \
  --api-key YOUR_API_KEY \
  --primary-model qwen3.5-plus
```

交互时如果不想测试 QQ，就明确回答 `no`；否则默认会进入 QQ 凭证采集。

安装完成后，检查：

```bash
ls ~/.openclaw
ls ~/.openclaw/runtime
```

你应该至少看到：

- `openclaw.json`
- `bridge.json`
- `install-state.json`
- `runtime/compose.yaml`
- `runtime/Dockerfile.openclaw`

然后执行：

```bash
cd ~/.openclaw/runtime
docker compose ps
docker compose logs --tail=200
```

### 5.2 Native 路径测试

如果你想优先走 Native：

```bash
./openclaw-install install \
  --mode native \
  --provider bailian \
  --api-key YOUR_API_KEY \
  --primary-model qwen3.5-plus
```

交互时同样建议先不要启用任何 channel；如果要保留默认 QQ，就准备好 `AppID` 和 `AppSecret`。

安装完成后，检查：

```bash
which openclaw
openclaw --version
openclaw config validate
```

再检查安装器生成的运行脚本：

```bash
ls -l ~/.openclaw/runtime
cat ~/.openclaw/runtime/run-openclaw.sh
```

## 6. 第三阶段：检查生成的配置

安装完成后，先不要急着接 channel，先看配置内容。

### 6.1 检查主配置

```bash
sed -n '1,220p' ~/.openclaw/openclaw.json
```

重点确认：

- `agents.defaults.model.primary` 是你选的模型
- `models.providers.<provider-id>` 存在
- `gateway.port` 为 `18789`
- `gateway.bind` 为 `loopback`（native）或 `lan`（docker）
- 如果没启用 channel，`channels` 应该为空对象或不包含新增 channel

### 6.2 检查 bridge 配置

```bash
sed -n '1,220p' ~/.openclaw/bridge.json
```

重点确认：

- `provider.baseUrl`
- `provider.apiKey`
- `provider.primaryModel`
- `channels` 是否与交互时选择一致

### 6.3 检查安装器状态

```bash
sed -n '1,220p' ~/.openclaw/install-state.json
```

重点确认：

- `mode`
- `managedProviderId`
- `managedChannels`
- `runtimeDir`

## 7. 第四阶段：验证 OpenClaw 主体可用

### 7.1 Native 模式

先看是否已安装：

```bash
openclaw --version
openclaw config validate
```

如果安装器已经拉起 gateway，可以再试：

```bash
openclaw gateway status
```

如果没有正常起来，可手动尝试：

```bash
openclaw gateway start
openclaw gateway status
```

### 7.2 Docker 模式

进入 runtime 目录：

```bash
cd ~/.openclaw/runtime
docker compose config
docker compose ps
docker compose logs --tail=200
```

如果容器没起来，重点看：

- npm 安装失败
- Docker 拉取基础镜像失败
- `openclaw gateway start --foreground` 启动失败

### 7.3 这一阶段的通过标准

通过标准至少满足一条：

- Native 模式下 `openclaw --version` 和 `openclaw config validate` 正常
- Docker 模式下容器处于运行状态，日志没有连续报错

## 8. 第五阶段：真实验证 LLM provider

当前安装器已经把 provider 信息写进配置，但 OpenClaw 主体是否会立即按预期消费，还取决于 OpenClaw 本身行为。为了把问题切开，第一轮建议直接验证 bridge 到 provider 的链路。

### 8.1 先确认 channel 配置路径

如果你暂时不测真实 channel，可以先区分两类路径：

- QQ：默认是 OpenClaw 插件 channel，不走 `bridge serve`
- 飞书 / 企业微信：走 bridge 服务

QQ 可以先检查：

```bash
openclaw plugins list
openclaw channels list
```

飞书或企业微信可以再单独启动 bridge 并做健康检查，例如：

```bash
./openclaw-install bridge serve --channel feishu
curl http://127.0.0.1:19091/healthz
```

### 8.2 provider 成功的判断

如果后面你实际把消息打到 bridge，而 bridge 没报 provider 错误，说明：

- API Key 至少可用
- Base URL 至少可达
- 模型名至少没有立即报错

如果 provider 有问题，通常会在 bridge 日志里看到：

- HTTP 401 / 403
- 模型不存在
- Base URL 404
- timeout

## 9. 第六阶段：单个 channel 联调

第一轮只测一个 channel，建议优先级：

1. QQ
2. 飞书
3. 企业微信

### 9.1 测 QQ

建议先执行：

```bash
./openclaw-install reconfigure --mode native --provider bailian --api-key YOUR_API_KEY --primary-model qwen3.5-plus
```

交互里：

- 只启用 `QQ`
- 填 `QQ Bot AppID`
- 填 `QQ Bot AppSecret`

完成后检查：

```bash
openclaw plugins list
openclaw channels list
```

如果插件和 channel 都配置成功，再从 QQ 侧实际发一条消息验证。

重点观察：

- `@sliverp/qqbot` 是否已安装
- `qqbot` channel 是否已出现在 `openclaw channels list`
- 实际消息是否能走通

### 9.2 测飞书

先重新配置：

```bash
./openclaw-install reconfigure --mode native --provider bailian --api-key YOUR_API_KEY --primary-model qwen3.5-plus
```

交互里：

- 只启用 `Feishu`
- 填 App ID / App Secret / Verification Token
- 填回调监听地址和 path

然后启动：

```bash
./openclaw-install bridge serve --channel feishu
```

在飞书开放平台里，把事件回调地址指到：

```text
http://你的机器地址:你填写的端口/你填写的path
```

验证点：

- challenge 校验是否通过
- 发送消息后是否进入 bridge
- bridge 是否能向飞书发送回复

### 9.3 测企业微信

步骤与飞书类似，但第一轮建议尽量用最简单的 webhook 场景。

重点先验证：

- bridge 可启动
- `GET echostr` 握手可通过
- 收到 POST 后不会直接报错

## 10. 第七阶段：验证 reconfigure 不会误伤

这一步很重要，因为它关系到后续“重新配置”是否安全。

### 10.1 手动插入一个自定义字段

先手动编辑：

```bash
sed -n '1,220p' ~/.openclaw/openclaw.json
```

然后在文件里加一个你自己的字段，例如：

```json
"my_custom_setting": {
  "enabled": true
}
```

### 10.2 执行 reconfigure

例如：

```bash
./openclaw-install reconfigure \
  --yes \
  --mode native \
  --provider bailian \
  --api-key YOUR_API_KEY \
  --primary-model qwen-max \
  --skip-verify
```

### 10.3 再检查结果

重点确认：

- `my_custom_setting` 还在
- 原来的 provider 被替换成新的 provider
- 上一轮由安装器托管的 channel 被移除或更新
- 备份文件已生成到 `~/.openclaw/.backups/`

## 11. 推荐测试矩阵

第一轮不需要全覆盖，建议最小测试矩阵如下。

### 11.1 最小闭环

- Linux
- Native 模式
- 百炼
- 不启用 channel

### 11.2 安装链路闭环

- Linux
- Docker 模式
- 百炼
- 不启用 channel

### 11.3 Channel 闭环

- Linux
- Native 模式
- 百炼
- QQ 或飞书，二选一

### 11.4 配置安全闭环

- 已安装环境
- 手工增加自定义配置
- 执行一次 `reconfigure`

## 12. 常见失败点与处理

### 12.1 Docker 拉镜像慢或失败

先确认：

```bash
docker pull node:22-bullseye
```

如果这条都很慢，说明问题在 Docker 网络，不在安装器逻辑。

### 12.2 npm 装 `openclaw` 失败

先单独验证：

```bash
npm config get registry
npm view openclaw version
npm install -g openclaw
```

如果这里失败，说明是 npm 网络或权限问题。

### 12.3 `openclaw gateway start` 失败

建议手动执行并看直接输出：

```bash
openclaw gateway start
```

如果失败，优先看：

- OpenClaw 本体是否装好
- 当前 shell 是否能找到 `openclaw`
- `openclaw.json` 是否含有非法字段

### 12.4 bridge 能启动，但消息没有回

优先分三段排查：

1. bridge 是否收到请求
2. provider 是否调用成功
3. channel 上游发送接口是否可用

也就是说，不要一上来就怀疑 OpenClaw 主体。当前 bridge 的 provider 调用链路和 OpenClaw 主体运行链路是相对独立的。

## 13. 回滚方法

如果真实测试把现有环境弄乱了，可以这样恢复。

### 13.1 恢复配置

查看备份：

```bash
ls -l ~/.openclaw/.backups
```

恢复你想要的备份：

```bash
cp ~/.openclaw/.backups/openclaw.json.backup.YYYYMMDD_HHMMSS ~/.openclaw/openclaw.json
```

### 13.2 停掉 Docker 模式

```bash
cd ~/.openclaw/runtime
docker compose down
```

### 13.3 停掉 Native 模式

```bash
openclaw gateway stop
```

### 13.4 停掉 bridge

如果是前台调试启动的，直接 `Ctrl+C`。

如果是 Linux 用户级服务：

```bash
systemctl --user stop openclaw-bridge-feishu.service
systemctl --user stop openclaw-bridge-wecom.service
```

QQ 现在走 OpenClaw plugin，不再对应 `openclaw-bridge-qq.service`。如果要停用 QQ，请用 `openclaw channels remove --channel qqbot` 或按你的 OpenClaw 环境方式处理。

## 14. 建议你实际怎么跑第一轮

如果你要最稳地开始，我建议就是下面这套：

1. `./openclaw-install doctor`
2. 选 Linux 机器
3. 先跑 `native` 模式
4. provider 选 `bailian`
5. 第一轮不启用任何 channel
6. 验证 `openclaw.json` / `bridge.json` / `install-state.json`
7. 验证 `openclaw --version` 和 `openclaw config validate`
8. 再跑一次 `reconfigure`，只启用 `qq`
9. 确认 `openclaw plugins list` 和 `openclaw channels list`
10. 再做 QQ 消息联调

这样你可以把问题拆成三层：

- 安装器本身
- OpenClaw 主体
- channel bridge

每层都清楚，就不会在第一轮测试里把问题搅在一起。
