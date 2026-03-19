# Code Review Checklist

> Review date: 2026-03-19
> Reviewer: Claude Code
> Status: **RESOLVED** — 所有检查项已完成修复并补充验证

---

## CRITICAL

- [x] **C1 — Reflected XSS via `echostr`**
  - File: `internal/bridge/server.go:282`
  - 原始 query 参数 `echostr` 直接写入 HTTP 响应，未设置 `Content-Type`，无长度限制，无字符校验
  - Fix: 设置 `Content-Type: text/plain; charset=utf-8`，限制长度（≤64字节），校验仅含字母数字

- [x] **C2 — Shell 注入：`channelID` 未转义注入生成的脚本**
  - File: `internal/install/assets.go:143,150`
  - `channelID` 在生成的 `.sh` / `.cmd` 脚本中未加引号，含 shell 元字符（`;`、`&`、`|`）时可执行任意命令
  - 同样影响 systemd unit（line 265-278）和 launchd plist（line 305-330）
  - Fix: 新增 `validateChannelID` 函数，强制 `^[a-zA-Z0-9_-]+$`，并在所有模板中加引号

- [x] **C3 — WeCom webhook 无认证**
  - File: `internal/bridge/server.go:279-316`
  - `handleWeCom` 未做任何签名或 token 校验，任意网络调用者均可触发 LLM 补全，消耗 API 额度
  - Fix: 实现 WeCom 消息签名验证（`msg_signature` + `timestamp` + `nonce` + SHA1）

---

## HIGH

- [x] **H1 — 飞书 token 校验可被绕过**
  - File: `internal/bridge/server.go:253`
  - 当 `payload.Token` 为空时跳过校验，攻击者省略 token 字段即可绕过
  - Fix: 移除 `payload.Token != ""` 条件，仅当配置了 token 时强制校验

- [x] **H2 — 使用已废弃的 `os.IsNotExist`**
  - File: `internal/config/files.go:19,44,76`
  - `os.IsNotExist` 不会解包 error chain；应使用 `errors.Is(err, os.ErrNotExist)`

- [x] **H3 — `Workflow.progress` 无并发保护**
  - File: `internal/install/workflow.go:66`
  - 可变字段 `progress` 在 `beginProgress`/`progressStep`/`progressDetailf` 中读写，无同步机制
  - Fix: 改为参数传递，或添加 mutex，或明确文档标注不可跨 goroutine 共享

- [x] **H4 — `seenEventIDs` sync.Map 无限增长**
  - File: `internal/bridge/server.go:70-83`
  - 过期条目仅在同一 eventID 重新出现时清除，长期运行内存泄漏
  - Fix: 添加后台 goroutine 定期清理过期条目

- [x] **H5 — 静默丢弃错误**
  - File: `internal/app/app.go:219-220`
  - `LoadInstallState` 和 `loadExistingBridgeConfig` 的错误被 `_` 忽略；权限错误或 JSON 损坏时安装器以空状态继续
  - Fix: 仅忽略 `ErrNotExist`，其他错误应向上传播

- [x] **H6 — npm registry URL 未校验**
  - File: `internal/install/workflow.go:484-494`
  - mirror 的 `BaseURL` 未校验 scheme/host 即传递给 `npm install`
  - Fix: 校验 URL 必须为 `https://` 且 host 非空

- [x] **H7 — API 密钥明文写入磁盘 + 临时文件竞争**
  - File: `internal/config/files.go:89-104`
  - `SaveJSONAtomic` 使用确定性临时文件名 `path + ".tmp"`，并发时存在竞争
  - Fix: 使用 `os.CreateTemp` 生成唯一临时文件名，错误时清理

---

## MEDIUM

- [x] **M1 — `channelID` 路径穿越**
  - File: `internal/install/assets.go:140,196,207,232,263,305`
  - `channelID` 直接拼接进 `filepath.Join`，含 `../` 可逃逸目标目录
  - Fix: 复用 C2 的 `validateChannelID`

- [x] **M2 — `valueOrDefault` 重复定义 4 次**
  - File: `app.go:712`, `config/files.go:314`, `install/channels.go:167`, `bridge/server.go:551`
  - 完全相同的函数体，维护隐患
  - Fix: 提取到共享 internal 包

- [x] **M3 — mirror probe 未读取 response body**
  - File: `internal/install/mirrors.go:65`
  - 未 drain body 即 Close，阻止 HTTP 连接复用
  - Fix: 添加 `_, _ = io.Copy(io.Discard, resp.Body)`

- [x] **M4 — `AskString` 的 `secret` 参数被忽略**
  - File: `internal/ui/prompt.go:58`
  - API Key 等敏感信息在终端明文回显
  - Fix: 使用 `golang.org/x/term.ReadPassword` 实现输入掩码

- [x] **M5 — `Request.Validate` 静默修改接收者**
  - File: `internal/install/workflow.go:286-309`
  - 名为 "Validate" 的方法不应有副作用，违反最小惊讶原则
  - Fix: 返回新的 sanitized 副本，或重命名为 `NormalizeAndValidate`

- [x] **M6 — shutdown goroutine 可能泄漏**
  - File: `internal/bridge/server.go:103-110`
  - 若 `ListenAndServe` 先于 ctx 取消退出，goroutine 将阻塞在 `<-ctx.Done()`
  - Fix: 使用 `sync.WaitGroup` 或额外 channel 协调

- [x] **M7 — 步骤计数硬编码**
  - File: `internal/install/progress.go:27-36`
  - `installStepCount` 通过硬编码算术计算，增删步骤时容易漏改
  - Fix: 改为自动计数或在安装结束时断言 `current == total`

---

## LOW

- [x] **L1 — `.yaml` 文件用 `json.Unmarshal` 解析**
  - File: `presets/presets.go:88,131`
  - 文件扩展名与解析方式不一致，易误导维护者
  - Fix: 重命名为 `.json` 或使用 YAML 解析器

- [x] **L2 — `bundle.Providers[0]` 无长度守卫**
  - File: `internal/app/app.go:382`
  - 若 providers 为空 slice 则 panic
  - Fix: 索引前检查长度

- [x] **L3 — `writeJSON` encode 错误被丢弃**
  - File: `internal/bridge/server.go:548`
  - `_ = json.NewEncoder(w).Encode(payload)` 静默忽略写入错误

- [x] **L4 — bridge HTTP handler 无限流**
  - File: `internal/bridge/server.go:150`
  - 无速率限制，可被刷量消耗 API 额度
  - Fix: 添加 `golang.org/x/time/rate` token-bucket 限流

- [x] **L5 — sorted-key map 迭代重复 3+ 次**
  - File: `app.go:650`, `progress.go:60`, `workflow.go:574`
  - Fix: 提取为共享工具函数

---

## 测试覆盖缺口

- [x] `internal/system/info.go` — 零测试
- [x] `chooseProviderPreset` 空 bundle panic 路径 — 未覆盖
- [x] `AskString` secret 掩码 — 未测试（因未实现）
- [x] table-driven test 风格使用不一致

---

## 修复优先级建议

1. **立即修复**: C1, C2, C3 — 安全漏洞，阻塞发布
2. **尽快修复**: H1, H5, H6, H7 — 高风险问题
3. **版本内修复**: H2, H3, H4, M1-M7 — 代码质量与健壮性
4. **后续迭代**: L1-L5, 测试覆盖缺口
