# 以 Endpoint(模型 × 协议)为监控最小单位

可用性监控的最小单位不是模型,而是 Endpoint = 一个 Model 在一种 Protocol(anthropic `/v1/messages` 或 openai `/v1/chat/completions`)下的组合,各自独立探测、独立出状态、独立统计。原因:实测同一模型在两种协议下可用性不同(如 `gpt-5.4` 仅 openai 协议可用,`kimi-k3` 当时仅 anthropic 协议可用,`claude-opus-4-8` 的 openai 协议报 no_available_providers),而消费方(Claude Code 走 anthropic,其他工具走 openai)关心的正是协议级差异。代价:探测量与数据量翻倍,通过 5 分钟间隔与 90 天原始数据保留期控制。
