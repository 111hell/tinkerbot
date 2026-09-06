# TinkerBot

一个 Tinker Agent，同时接入多个 channel。接入层负责协议适配、会话 ID 和工具能力配置；运行层只处理独立的 session。

完整设计见 [架构说明](https://github.com/111hell/tinkerbot/blob/main/docs/architecture.md)，包含架构、启动、消息处理、工具隔离和 SIU 交付流程图。

## 启动

需要 Go 1.27。本地依赖保持为：

```go
replace github.com/111hell/tinker => ../tinker
```

在 YAML 中填写 `model.api_key` 后运行 CLI：

```sh
go run ./cmd/myagent -config config.example.yaml -channel cli
```

同时运行 CLI 和 SIU，另需填写 YAML 中的 `siu.base_url`、`siu.bot_token`、`siu.webhook_url`：

```sh
go run ./cmd/myagent -config config.example.yaml
```

只保留一份完整模板，可通过 `-channel cli`、`-channel siu` 或 `-channel cli,siu` 选择启动渠道。

| 文件 | 用途 |
| --- | --- |
| [config.example.yaml](https://github.com/111hell/tinkerbot/blob/main/config.example.yaml) | 唯一配置模板，包含共享设置及 CLI / SIU |
| `config.yaml` | 本地实际配置，默认启动时读取，已被 Git 忽略 |

所有配置及说明集中在 YAML 中，无需 `.env`。也支持通过 `${VAR}` 引用进程环境变量，但程序不会自动读取 `.env`。建议在已被 Git 忽略的 `config.yaml` 中填写实际值；默认读取本地配置的命令为 `go run ./cmd/myagent`。

## Channel 与工具配置

```yaml
tools: [] # 全局通用工具
channels:
  - type: cli
    tools: []
  - type: siu
    tools: [siu_list_messages, siu_search_contacts]
```

所有 channel 共享模型、系统提示词、Skill 和同一个 Agent 实例。工具列表按“渠道显式配置 → 全局 `tools` → 原渠道默认值”解析，采用整体覆盖而非合并。模型请求只包含最终启用的工具定义，执行时再次检查权限。

- 渠道省略 `tools` 时继承全局列表；渠道 `tools: []` 禁用本渠道所有工具。
- 全局 `tools: []` 让未单独配置工具的渠道全部禁用工具。
- 全局和渠道都未配置时，保持原行为：CLI 无工具，SIU 启用两个查询工具。
- SIU 查询还要求接入层提供可信用户上下文；仅给 CLI 添加工具名称不会自动获得 SIU 用户身份。
- 当前支持 CLI 和 SIU 各一个适配器同时运行。Telegram 尚未实现。

保留旧的 `channel.type` 单渠道配置，但不能同时配置 `channel` 和 `channels`。命令行 `-channel cli,siu` 可以覆盖启动列表，并保留 YAML 中对应渠道的工具配置。未配置任何渠道时默认 CLI。

## Session 与生命周期

CLI 使用 `cli:default`，SIU 使用 `siu:<TopicID>`；这些 ID 由接入层生成，运行层将其视为不透明字符串。

不同 session 可并发执行，历史和取消互相隔离；同一 session 的读取、运行、初始化和清空通过运行层串行化。每轮统一使用 `agent.run_timeout`，默认 2 分钟。

`sqlite.path` 非空时，所有渠道的 session 保存在 SQLite `agent_sessions` 表中，重启后保留。空值使用内存。配置默认值和统一模板均使用 `myagent.db`；需要临时会话时改为 `path: ""`。旧 SIU `conversations` 表在首次访问时用于补充新 session，不与新表混用。

CLI 支持逐行输入、流式回答、`/new` 清空当前会话、`/exit` 或 Ctrl-D 退出 CLI。回答写 stdout，提示和错误写 stderr，单行上限约 1 MiB。退出 CLI 不会停止 SIU；Ctrl-C / SIGTERM 停止整个进程。单个渠道运行失败会记录错误，其余渠道继续运行。

SIU 接入层保留 Webhook、Topic 调度、新消息取消旧运行、Fork、删除和流式回复回退。SIU 不再创建独立 Agent，也不再使用单独的系统提示词或固定 30 分钟运行超时。

运行成功时保存完整历史；模型错误、超时或增量交付中断时不提交该轮。最终 SIU 交付发生在保存之后，因此最终交付失败时历史可能已经保存。当前没有历史摘要或裁剪。

## 代码边界

- `internal/channel`：多渠道生命周期和 CLI / SIU 适配器。
- `internal/events`、`internal/conversation`、`internal/run`：SIU 的事件、旧历史和协议取消适配。
- `internal/chat`：与渠道无关的共享运行服务、session 串行化和历史存储接口。
- `internal/toolscope`：与渠道无关的工具可见性和执行权限校验。
- `internal/storage`：SQLite 新 session 表及旧 SIU 历史表。
- 本地 Tinker：模型与工具循环；本次无需修改 Tinker 源码。

`skills_dir` 中的 Skill 正文在启动时加入共享系统提示词。Skill 是共享提示内容，工具权限由 session 能力控制。

## 验证

```sh
go build ./...
go vet ./...
```

仓库不包含 `_test.go` 文件，以上命令用于构建和静态检查。真实模型和 SIU 服务需另行联调。
