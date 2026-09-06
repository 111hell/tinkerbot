# TinkerBot

[English](README.md) | [简体中文](README.zh-CN.md)

> Run the same AI assistant in your terminal and SIU private chats.

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
- **SIU-aware assistance** — search the current user's contacts and read recent messages in the active private conversation.
