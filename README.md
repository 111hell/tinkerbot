# TinkerBot

[English](README.md) | [简体中文](README.zh-CN.md)

> Run the same AI assistant in your terminal and SIU chats.

TinkerBot is a single-bot application built on [Tinker](https://github.com/111hell/tinker). It lets you use one assistant across CLI and SIU without maintaining separate bots: conversations, model settings, Skills, and tools are shared across enabled channels.

## Quick Start

Requires Go 1.27+ and a sibling Tinker checkout at `../tinker`.

```sh
cp config.example.yaml config.yaml
# Set model.api_key in config.yaml.
go run ./cmd/tinkerbot -channel cli
```

## Features

- **Extensible channels** — add channel adapters around the shared agent runtime; CLI and SIU are included.
- **Persistent conversations** — keep separate chat histories in local SQLite across restarts.
- **Skills and tools** — load `SKILL.md` Skills and configure tools globally or per channel.
- **SIU-aware assistance** — private and group chats, Topic streaming, stop, skip, and forks; configurable tools for contacts, chat search, user profiles, images, message editing/revocation, and client events.

See the [SIU Bot API integration guide (Chinese)](docs/siu-bot-api.md) for configuration, client methods, and limitations.

## Logs

JSON logs go to stderr. The default `log_level: info` shows event acceptance, history loading, each model request, tool calls, reply delivery, and timings. Correlate a turn with `conversation_id` / `message_id`; `session_id` identifies the shared runtime and `reply_message_id` identifies the SIU reply. Duplicate and unsupported events are logged at `debug`. Logs do not explicitly include message/reasoning bodies, tool arguments, or result bodies; error details still come from underlying components.

Start from the project root and retain logs from this launch:

```sh
mkdir -p /tmp/tinkerbot
go run ./cmd/tinkerbot -channel siu 2>>/tmp/tinkerbot/bot.jsonl
```

In another terminal, follow logs or filter by message ID (requires jq):

```sh
tail -f /tmp/tinkerbot/bot.jsonl
jq -c 'select(.message_id == "YOUR_MESSAGE_ID")' /tmp/tinkerbot/bot.jsonl
```

The last stage helps identify whether the bot is waiting for the model, executing a tool, or sending a reply. `agent generation completed` means generation ended; `SIU reply delivered` means the delivery API succeeded.

## Local SIU access with long polling

Set these fields in your own `config.yaml`, preserving your model, tools, and storage settings:

```yaml
siu:
  receive_mode: polling
  base_url: "https://api-http.sundayonline.top"
  bot_token: "${SIU_BOT_TOKEN}"
  webhook_url: ""
```

Export `SIU_BOT_TOKEN` securely, then run `go run ./cmd/tinkerbot -config config.yaml -channel siu`. The [Chinese local setup](README.zh-CN.md#本地连接-siu-internal长轮询) includes the test credential-file command. Polling ignores listen_addr and does not open a listener or register/clear a webhook. Omitting receive_mode preserves the existing Webhook behavior.

Only one receiver may run per Bot. Before switching, stop the old receiver and clear the server webhook using SIU administration (`bot.update` with `{"webhook_url":""}`). An empty local webhook_url does not clear the server value. Switching back requires stopping the poller before starting Webhook mode.

Events are live notifications: every polling start discards events timestamped at or before that start, including messages and controls from downtime. No receive-mode state, cursor persistence or backlog flags are needed. The boundary uses SIU's millisecond timestamps and assumes synchronized clocks; it is not exact deduplication. Undated events are discarded too.

Polling uses 30-second server waits with a 40-second HTTP timeout and cancelable 3-second retry delays. Events enter the existing Dispatcher/Scheduler; full-queue events are retained locally for retry while later controls can still be received. A 1000-event ingress backlog stops the channel with an error. SIU trims before returning a response: lost responses or process exits can lose events, and neither SQLite history nor in-process deduplication provides durable delivery. See [the consumption and switching contract](docs/siu-bot-api.md#polling-消费语义与模式切换).
