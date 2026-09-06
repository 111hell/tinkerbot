# TinkerBot

[English](README.md) | [简体中文](README.zh-CN.md)

> 在终端和 SIU 私聊中运行同一个 AI 助手。

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
- **Skill 与工具**：加载 `SKILL.md` Skill，并按全局或渠道配置工具。
- **SIU 助手能力**：查询当前用户的联系人，以及当前私聊中的近期消息。
