# HubScope

对内的 AI Hub(模型网关)可用性监控与质量评估网站:Go 单二进制,内嵌 Vue 前端与 SQLite。

- **可用性监控**:以 Endpoint(模型 × 协议)为单位,每 5 分钟一轮 Probe(非流式 + 流式),记录成败、延迟、TTFT、token 用量,产出红黄绿状态与趋势曲线
- **模型评估**:5 个内置能力 Suite(指令遵循/推理/代码/知识问答/语言理解与生成),规则判定 + LLM 裁判混合打分,每周定时 + 手动触发
- **告警**:Endpoint 连续失败/恢复时推送飞书群机器人
- **管理后台**:Hub 实例、模型启停、Webhook、裁判模型全部在线配置

领域术语见 [CONTEXT.md](./CONTEXT.md),需求见 [docs/specs/](./docs/specs/),架构决策见 [docs/adr/](./docs/adr/),部署见 [docs/deployment.md](./docs/deployment.md)。协作规则见 [CLAUDE.md](./CLAUDE.md)。

## 下载与部署

当前未发布预编译二进制,两条路径都从仓库出发:

- **Docker(推荐)**:clone 后 `docker compose up`,不依赖本机 Go/pnpm 工具链
- **一键部署脚本(Linux,需要 Go + pnpm)**:clone 后 `sudo scripts/install.sh`,构建并安装为 systemd 服务

生产部署细节(systemd/nginx)见 [docs/deployment.md](./docs/deployment.md)。

## Quick Start

```sh
make build                                    # 前端构建 + 单二进制 → bin/hubscope
./bin/hubscope                                # 启动服务(默认监听 :8080,数据存 ./data/app.db)
./bin/hubscope admin create --username admin --password 'a-strong-password'
# 打开 http://localhost:8080 → 登录 → 添加 Hub → 自动发现模型
```

首个 `super_admin` 必须用 `hubscope admin create` CLI 引导(数据库里没有用户时无法走 HTTP 鉴权)。口令经 bcrypt 哈希入库,不读环境变量、不进 git。

## Development

```sh
make hooks        # clone 后跑一次:启用 pre-commit 门禁
make dev          # 本地跑后端(前端未构建时用 stub 页面)
make test         # 全量门禁:go vet + go test + 前端 typecheck + 前端构建
cd web && pnpm dev  # 前端热更新开发(代理 /api → :8080)
```

## Configuration

全部经环境变量注入(Hub 凭证在管理后台维护、存数据库,见 ADR-0001):

| Variable         | Default         | Purpose                |
|------------------|-----------------|------------------------|
| `ADDR`           | `:8080`         | 监听地址               |
| `DATA_PATH`      | `./data/app.db` | SQLite 数据文件        |

完整环境变量表见 [docs/deployment.md](./docs/deployment.md)。

管理员口令不经环境变量,用 `hubscope admin create` CLI 创建(见上)。
