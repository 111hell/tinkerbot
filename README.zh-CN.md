# TinkerBot

SIU 文件附件现支持 PDF、DOCX、PPTX、XLSX、UTF-8 CSV/TXT 正文提取。先运行 `uv run --script scripts/convert-document.py --check` 安装公开依赖，再从项目目录启动；无需新增配置。图片 URL 已接入多模态输入，实际理解能力取决于模型支持及 URL 可访问性；音视频和 OCR 尚未接入。限额及截断规则见 [文件正文读取](docs/siu-bot-api.md#文件正文读取)，完整能力与边界见 [架构说明](docs/architecture.md#当前-agent-能力与边界)。

[English](README.md) | [简体中文](README.zh-CN.md)

> 在终端和 SIU 聊天中运行同一个 AI 助手。

TinkerBot 是一个基于 [Tinker](https://github.com/111hell/tinker) 的单 Bot 应用。无需维护两套 Bot，即可在 CLI 和 SIU 中使用同一个助手；启用渠道共享会话、模型配置、Skill 和工具。

## 快速开始

需要 Go 1.27+，以及相邻目录中的 Tinker：`../tinker`。

```sh
cp config.example.yaml config.yaml
# 在 config.yaml 中填写 model.api_key。
go run ./cmd/tinkerbot -channel cli
```

## 特性

- **可扩展渠道**：围绕共享 Agent 运行时接入更多渠道；当前内置 CLI 和 SIU。
- **持久化会话**：使用本地 SQLite 保存独立会话，重启后继续使用。
- **联网查询**：`web_search` 搜索公开网页，`web_visit` 读取网页正文，CLI、SIU 私聊和群聊均可用。模板已启用；旧配置将全局 `tools` 设为 `[web_search, web_visit]` 后重启。基于 AnySearch，支持匿名调用；可通过 `web.api_key` 配置密钥。查询词和 URL 会发给该服务，服务不可用时会返回失败；网页访问不支持交互操作。
- **自然交互**：普通问候和自我介绍直接回应；只有任务需要时才查询 SIU 资料，默认不向用户展开内部权限或接口细节。
- **Skill 与工具**：加载 `SKILL.md` Skill，并按全局或渠道配置工具。
- **SIU 助手能力**：支持私聊、群聊、Topic 流式回复和停止/分叉；按需启用联系人、聊天搜索、用户查询、图片发送、编辑/撤回和客户端事件工具。

Bot 配置、完整客户端接口和使用限制见 [SIU Bot API 接入说明](docs/siu-bot-api.md)。

## 查看日志

日志以 JSON 写入 stderr，默认 `log_level: info` 即可看到事件接收、历史加载、每次模型请求、工具调用、回复发送及耗时。用 `conversation_id` / `message_id` 串起一轮处理，`session_id` 定位共享运行时，`reply_message_id` 对应 SIU 回复。重复及不支持的事件在 `debug` 级别记录。日志不主动输出消息正文、推理正文、工具参数或结果正文；错误信息仍由底层组件提供。

在项目根目录启动并保存日志（仅包含本次启动后的日志）：

```sh
mkdir -p /tmp/tinkerbot
go run ./cmd/tinkerbot -channel siu 2>>/tmp/tinkerbot/bot.jsonl
```

另开终端查看，或按消息 ID 筛选（需要 jq）：

```sh
tail -f /tmp/tinkerbot/bot.jsonl
jq -c 'select(.message_id == "替换为消息ID")' /tmp/tinkerbot/bot.jsonl
```

最后一条阶段日志可以帮助判断正在等待模型、执行工具还是发送回复。`agent generation completed` 仅代表生成结束；`SIU reply delivered` 才代表发送接口成功。

## 本地连接 SIU internal（长轮询）

在你自己的 `config.yaml` 中修改以下字段，保留已有模型、工具和存储配置：

```yaml
siu:
  receive_mode: polling
  base_url: "https://api-http.sundayonline.top"
  bot_token: "${SIU_BOT_TOKEN}"
  webhook_url: ""
```

`listen_addr` 在 polling 模式下不使用，可以保留或设为空。省略 receive_mode 则仍使用 Webhook。先确认 Bot 没有已配置的远端 Webhook，且没有其他接收实例；本地空 webhook_url 不会替你清空远端配置。

以下命令从指定凭证文件读取 token 放进当前进程环境，不输出凭证。请勿在启用 shell 跟踪（`set -x`）时运行：

```sh
cd /Users/janeshirley/proj/myagent
export SIU_BOT_TOKEN="$(python3 -c 'import json; print(json.load(open("/Users/janeshirley/proj/w/siu/.siu/internal-full-features-bot-20260914.json"))["token"])')"
go run ./cmd/tinkerbot -config config.yaml -channel siu
```

预期日志先显示 `SIU bot authorized`（测试 Bot ID 167545），再显示 `SIU polling started`；收到消息后出现 `SIU polling received`。无需公网回调、Funnel 或内网穿透。退出后可 `unset SIU_BOT_TOKEN`。日志保存方法见上文。

消费默认会在 HTTP 响应返回前 trim，存在响应丢失和进程退出时的事件丢失窗口；不是持久确认队列。切换模式前请阅读 [消费语义与切换说明](docs/siu-bot-api.md#polling-消费语义与模式切换)。

事件按即时通知处理：每次启动 polling 都丢弃时间戳不晚于本次上线时间的事件，包括停机期间的消息、Fork、stop 等；无时间戳也丢弃。不补积压，不保存接收模式或游标，无需切换参数或额外配置。该时间边界要求 Bot 与 SIU 时钟一致，不是精确去重；本次在线期间的请求间隙消息仍照常接收。
