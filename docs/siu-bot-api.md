# SIU Bot API 接入

依据提供的 `Bot API (2).md` 接入，覆盖文档中的 17 个 HTTP 接口。`internal/siu` 负责协议，`internal/events` 负责渠道事件，`internal/tools/siu` 负责模型工具。扩展 API 不需要改动 Tinker Agent 或共享会话服务。

## 配置和启动

在 `config.yaml` 中填写 SIU 地址和 Bot Token，启用 `siu` 渠道。完整配置参考 `config.example.yaml`。

```yaml
channels:
  - type: siu
    tools:
      - siu_list_messages
      - siu_search_contacts
      - siu_search_chats
      - siu_query_user
      - siu_send_photo

siu:
  base_url: "https://api-http.sundayonline.top/api/v1"
  bot_token: "${SIU_BOT_TOKEN}"
  webhook_url: "${TINKERBOT_WEBHOOK_URL}"
```

启动时调用 `WhoAmI` 验证 Bot 身份。Webhook 模式监听成功后注册非空的 `siu.webhook_url`；为空时保留服务端 webhook 配置。Polling 模式只主动调用 getUpdates，不监听 HTTP 端口，也不注册或清空服务端 Webhook。Bot 昵称、欢迎语、功能开关、输入规则和群消息策略由 SIU 管理端或 dev-cli 配置，TinkerBot 不负责修改。

## 模型工具

| 工具 | 能力 | 范围 |
| --- | --- | --- |
| `siu_list_messages` | 查询最近 30 天消息，支持游标翻页 | 当前用户与 Bot 的当前 Topic；无 Topic 时只返回无 Topic 消息 |
| `siu_search_contacts` | 搜索联系人 | 当前私聊用户；需要特权 Bot |
| `siu_search_chats` | 搜索用户可见的聊天，可选 Bot 筛选 | 当前私聊用户；需要特权 Bot |
| `siu_query_user` | 按用户名或 ID 查询基本资料 | 私聊/群聊；需要特权 Bot |
| `siu_send_photo` | 使用外部 URL 或已上传的文件 ID 发图 | 当前私聊或群聊；私聊保留 Topic |
| `siu_edit_message` | 编辑已知 Bot 文本消息 | 当前私聊或群聊 |
| `siu_revoke_message` | 撤回已知消息 | 当前私聊或群聊；由 SIU 检查撤回权限 |
| `siu_send_event` | 向客户端发送临时事件 | 当前私聊用户；事件类型由应用与客户端约定 |

默认仍是消息查询和联系人搜索两个工具，其余显式加入 `tools`。群聊隐藏并拒绝执行私聊数据工具。收件人、查询数据所属用户和 Topic 来自接入层 context；模型只能提供业务参数。`siu_query_user.user_id` 是被查询的基本资料对象，不是授权身份。

查询历史按 SIU 页取数后筛选 Topic，因此空页也可能有 `has_more: true`。继续使用 `next_before_message_id` 即可；已删除消息被过滤。普通 Bot 调用特权接口会收到 HTTP 403，接入层保留该错误，不改用其他用户身份重试。

## Go 客户端

客户端接受服务根地址或以 `/api/v1` 结尾的地址，使用 `X-Api-Key` 鉴权。可注入 `http.Client`；传 `nil` 使用 40 秒超时，覆盖最长 30 秒的长轮询。不会自动重试消息发送、更新或上传。`Bot` 结果不保留响应中的 Token。

| Go 方法 | Bot API |
| --- | --- |
| `WhoAmI` | `GET bot.whoami` |
| `SetWebhook` | `POST bot.update` |
| `SendEvent` | `POST bot.sendEvent` |
| `QueryUser` | `POST user.queryUser` |
| `SearchContacts` | `POST user.searchContacts` |
| `UploadMedia` | `POST file.uploadMedia` |
| `ListMessages` | `GET message.listMessages` |
| `ListRecentUsers` | `GET message.listRecentUsers` |
| `SearchChats` | `POST message.searchChats` |
| `GetUpdates` | `GET bot.getUpdates` |
| `SendMessage` | `POST message.sendMessage` |
| `SendPhoto` | `POST message.sendPhoto` |
| `SendStreamMessage` | `POST message.sendStreamMessage` |
| `FinishStreamMessage` | `POST message.finishStreamMessage` |
| `EditMessage` | `POST message.editMessage` |
| `RevokeMessage` | `POST message.revokeMessage` |
| `EmptyStreamMessage` | `POST message.emptyStreamMessage` |

Bot 管理、最近用户、事件拉取和重新生成流程供应用代码调用；不作为普通聊天模型工具注册。运行入口通过 `siu.receive_mode` 选择 Webhook 或 polling；不要在同一个 Bot 上同时消费两条事件链路。`NoTrim: true` 表示读取后不清理事件。

查询联系人和聊天的响应使用 Protobuf JSON：camelCase 字段名、字符串形式的 uint64。客户端用独立类型保留这些字段，避免大 ID 经浮点数转换而丢失精度。

`APIError` 保留 `StatusCode`、`Code` 和 `Message`。通过 `errors.As` 检查 HTTP 状态，通过 `siu.IsCode(err, "CHAT_MESSAGE_STREAM_CLOSED")` 检查业务错误，HTTP 非 2xx 时同样有效。

### 上传并发送图片

应用代码可以使用 `UploadImage(ctx, contentType, data)` 完成申请 URL 和 PUT 上传，自动计算长度和 Base64 MD5，再把返回的 `FileID` 传给 `SendPhoto`。

大文件或流式读取可分别使用 `UploadMedia` 和 `PutMedia(ctx, uploadURL, reader, metadata)`；两步必须使用一致的 MIME、长度和 MD5。存储上传不携带 Bot Token，也不解析 Bot Response 格式。

发送已有文件 ID 时宽高必须大于零；外部 URL 和文件 ID 同时提供时 SIU 优先使用文件 ID。`thumbnail_file_id` 目前无法指定独立缩略图，这是服务端的现有行为。

### 流式回复与重新生成

TinkerBot 默认合并正文和模型返回的 reasoning：累计 48 个 Unicode 字符发送一片；收到新事件时，缓冲超过 300ms 也会发送。切换正文/思考或模型消息结束时立即发完余量。reasoning 通过 `RichReasoning` 的 `text` 记录发送，顺序与模型输出一致。上游模型未返回 reasoning 时不生成占位思考内容。

首片传 `ToUserID`、事件中的 `TopicID` 和用户消息的 `UserMessageID`；后续分片增加首片返回的 `MessageID`，保留三种 ID，并递增 `Index`。`RichReasoning.JSONLine()` 生成 `{type,id,payload,parent_id?,replace?}` 的单行 JSON 字符串。`id`、`parent_id` 为整数；相同 ID 的 `text` 更新发送累计全文并设置 `replace: true`，与 Agentiu 的 `Stream.Thinking` 实现一致；最终行不带 `replace`。不同思考阶段使用不同 ID，工具执行状态通过父子行呈现；完成接口提交按 ID 去重后的最终行。协议参考：[Rich Reasoning JSONL Protocol](https://app.notion.com/p/33b70ee03838801db5a3f63c020e97fd)。

完成接口只接收用户 ID、用户消息 ID、Bot 回复 ID，不传 Topic。`Text: nil` 由服务端合并文本，指向空字符串则清空；`RichReasoning: nil` 合并已有记录，非 nil 的空数组清空记录。

重新生成使用 `EmptyStreamMessage` 清空旧回复，随后保留原回复 ID、用户消息 ID 和 Topic，从 `Index: 0` 重新发送，最后调用完成接口。清空不会解除停止状态；SIU 停止后的五分钟内可能拒绝相同用户消息的重新生成。

## 事件和会话

- 有 Topic 的私聊继续使用 `siu:<TopicID>`，兼容原有持久化历史。无 Topic 私聊使用 `siu:user:<UserID>`，群聊使用 `siu:group:<GroupID>`；这些内部键从不作为 API `topic_id`。
- 只有有 Topic 的私聊使用流式接口；无 Topic 私聊和群聊累积正文后发送普通消息。群聊消息按 SIU 的 `group_message_policy` 投递，Bot 必须在群内；群内每位发言者在模型输入中携带用户 ID。
- `stop_responding` 按用户消息 ID 停止对应运行，之后不再发送或完成回复；记录保留五分钟，处理停止事件先于用户消息到达的情况。
- `skip_thinking` 按 Bot 回复消息 ID 停止 reasoning 交付，正文继续；也处理事件早于首片 HTTP 响应返回的情况，不影响同 Topic 中的其他回复。
- `fork_topic` 依据复制后的消息建立新 Topic 上下文；`delete_topic` 取消运行并清理上下文。编辑事件不触发新回复，按会话串行使缓存失效，下一次私聊从 SIU 重新加载。
- 图片、视频、文件和文件组使用类型化附件；文档未定义内部结构的语音、自定义附件保留原始 JSON。当前 Tinker 输入为文本，模型收到的是附件元数据和 URL，尚不具备图片理解、语音转写或文件正文读取能力。

Webhook 接收 `Event[]`，入队后返回 204，模型在后台处理。进程内保留最近 4096 个事件 ID 去重；未知事件被忽略，单个坏事件不阻止同批其他事件。最多同时处理 8 个会话、等待 100 个事件；队列满返回 503，控制事件仍可处理。SIU 不主动重试 Webhook，因此这不是持久队列，也不承诺必达或跨重启去重。

同会话新消息取消旧运行。首次加载私聊最多读取最近 100 条；群聊仅有本应用收到的历史，编辑使群聊缓存重置。若首片失败且没有已知回复 ID，会回退普通消息；已有回复时用完整正文完成原回复。完成结果不确定时记录错误，不另发一条重复消息。

## 验证

`go test ./...` 包含本地 HTTP 协议、上传、字段存在性、工具作用域、Webhook 和流式控制回归；`go test -race ./...` 检查并发；`go vet ./...` 和 `go build ./...` 做静态与构建检查。这些测试不连接真实 SIU、模型、数据库或集群；上线前仍需用实际 Bot 验证权限、Webhook 可达性和客户端渲染。

## Polling 消费语义与模式切换

本次核对的 SIU 源码：`apps/im/internal/httpserver/operations/bot/get_updates.go` → `apps/im/pkg/bot/event/store.go` 的 `Fetch/Trim`。

- `start_id` 默认 `0`，表示从仍保留的最早事件开始；Redis XREAD 只返回严格大于该 ID 的事件。它不是 Telegram offset，也不是处理成功确认。
- 默认 `no_trim=false`。服务端先 `XTRIM MINID` 再 `XDEL`，删除至本批最后一个 ID（包含该条），然后才返回 HTTP 响应。即使传入 start_id，trim 也会删除该批最后 ID 之前的未返回事件。
- `no_trim=true` 完全不做上述清理。反复不带游标读取会重复拿到旧事件；仅用内存游标则重启会重放。TinkerBot 使用默认消费模式，不传 start_id，也不混用 no_trim；返回后按本次上线时间过滤旧通知。
- 接收到的重复 ID 由现有 Dispatcher 的最近 4096 条进程内缓存去重；队列满的事件不标记为成功，在本地重试。坏事件会记录拒绝原因，继续处理后续事件。停止/跳过可绕过等待中的普通事件；删除 Topic 会取消运行并作废该 Topic 更早的待入队事件。
- 这不是“处理成功后确认”的可靠队列。响应丢失、进程退出、入队后生成失败都可能导致已消费事件无法恢复；trim 本身也不是一个原子确认协议。服务器使用近似 MAXLEN 100，长时间停机或大流量也可能淘汰事件。不能承诺必达或跨进程恰好一次。SQLite 目前保存会话历史，不保存接收队列。
- 本地接收缓冲满 1000 条时会明确报错并停止继续消费；已经消费但尚未处理的事件不会自动恢复。排查 `SIU polling stopped with consumed events still pending` 日志中的数量。该模式适合单实例本地 Bot；需要可靠消费时，应先由 SIU 提供持久确认/重投协议。

同一个 Bot 只能运行一个接收实例。SIU 会将事件写入 Redis，同时为已配置 Webhook 的 Bot 发送回调；服务端 getUpdates 不会替你排斥 Webhook 或第二个 poller。多个 poller 还可能竞争甚至读到同一批事件。

从 Webhook 切换：先停止旧 Bot 接收实例，用 SIU 管理端清空已有回调（API 等价操作为 `POST /api/v1/bot/bot.update`，请求体 `{"webhook_url":""}`，使用 Bot 认证），再配置 `receive_mode: polling`、`webhook_url: ""` 并启动。仅把本地 webhook_url 留空不会清空服务端地址。已经进入 Webhook 投递队列的请求可能仍会到达旧地址，应停掉旧消费者。

### 仅处理本次在线期间的事件

所有事件都是即时通知，不承诺离线补偿。每次进入 polling 循环时记录一次 `started_at_ms`，仅接纳 `event.timestamp > started_at_ms` 的事件。SIU 契约使用 Unix 毫秒；上线前、同一毫秒和缺少有效时间戳的事件均消费后丢弃，不会触发回复或控制操作。日志 `SIU polling offline event discarded` 记录事件 ID、类型与时间；无需人工清空队列。

- 首次切换、再次从 Webhook 切回、正常重启：策略完全相同；无模式表、持久游标或额外启动参数，SQLite 仍只负责原有会话历史。
- 停机期间的用户消息、群消息、Fork、编辑、删除 Topic、stop、skip 等不补处理；需要新操作时由用户重新发起。
- 边界固定到本次 polling 结束，过滤每个返回批次，包含稍后才送达的旧事件。不会在每次请求时重置到“最新”，所以请求间隙产生的新事件仍会接收。
- 进程未退出时的短暂网络失败仍沿用原边界并重试；这不等于只接纳网络连接正常瞬间的事件，也没有额外的两分钟过期限制。

参考 Agentiu 的思想是“过滤过期通知，不恢复历史任务”。其当前 `channel/siu/poll.go` 和 `events.go:filterStale` 实际使用滚动两分钟窗口，并保留无时间戳事件；它仍可能接纳停机期间最近两分钟的消息。TinkerBot 按这里的在线语义采用启动边界并丢弃无时间戳事件，没有照搬这个窗口。

这是基于时间的接收策略，**不是精确去重**：Bot 与 SIU 时钟偏差会误收或误丢边界附近的事件，同毫秒事件被保守丢弃。若必须获得与时钟无关的严格上线边界，需要 SIU 返回服务端 Stream 尾部游标并保证后续按该游标读取。当前改动不修改 SIU 的存储策略，也不解决返回前 trim 的丢消息窗口。

切回 Webhook：先停止 poller，再配置 `receive_mode: webhook`、可达的 `webhook_url` 和 `listen_addr` 后启动。TinkerBot 不自动修改其他 Bot 配置。

当前 `WhoAmI` 响应不包含 Webhook 地址，程序无法据此验证远端是否已清空；必须由操作者确认。本地 polling 配置若仍带非空 webhook_url，会在启动前拒绝。
