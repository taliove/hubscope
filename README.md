# HubScope

对内的 AI Hub(模型网关)可用性监控与质量评估网站:Go 单二进制,内嵌 Vue 前端与 SQLite。

- **可用性监控**:以 Endpoint(模型 × 协议)为单位,每 5 分钟一轮 Probe(非流式 + 流式),记录成败、延迟、TTFT、token 用量,产出红黄绿状态与趋势曲线
- **模型评估**:4 个内置能力 Suite(基础指令遵循/推理数学/代码/中文),规则判定 + LLM 裁判混合打分,每周定时 + 手动触发
- **告警**:Endpoint 连续失败/恢复时推送飞书群机器人
- **管理后台**:Hub 实例、模型启停、Webhook、裁判模型全部在线配置

领域术语见 [CONTEXT.md](./CONTEXT.md),需求见 [docs/specs/](./docs/specs/),架构决策见 [docs/adr/](./docs/adr/),部署见 [docs/deployment.md](./docs/deployment.md)。协作规则见 [CLAUDE.md](./CLAUDE.md)。

## Quick Start

```sh
make build                                    # 前端构建 + 单二进制 → bin/hubscope
# bootstrap the first super_admin (ADR-0011; CLI delivered by ticket 69):
./bin/hubscope admin create --username admin --password '<new-password>'
./bin/hubscope                                 # 打开 http://localhost:8080 → 登录 → 添加 Hub → 自动发现模型
```

## Development

```sh
make dev          # 本地跑后端(前端未构建时用 stub 页面)
make test         # 全量门禁:go vet + go test + 前端 typecheck + 前端构建
cd web && pnpm dev  # 前端热更新开发(代理 /api → :8080)
```

## Configuration

全部经环境变量注入(Hub 凭证在管理后台维护、存数据库,见 ADR-0001):

| Variable         | Default          | Purpose                |
|------------------|-----------------|------------------------|
| `ADDR`           | `:8080`         | 监听地址               |
| `DATA_PATH`      | `./data/app.db` | SQLite 数据文件        |
| `SESSION_SECRET` | *(自动生成)*    | session cookie HMAC 签名 key;未设则首启在 settings 表生成 32 字节 hex,轮换即全部 session 失效。见 ADR-0011 |

> `ADMIN_PASSWORD` 已废弃(ADR-0011):不再 gate 启动,过渡期仅记 deprecation warning;首个 super_admin 经 CLI `hubscope admin create` 创建(CLI 由 ticket 69 交付,落地后硬删 env 读取路径)。
